package executor

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func resetClaudeDeviceProfileCache() {
	claudeDeviceProfileCacheMu.Lock()
	claudeDeviceProfileCache = make(map[string]claudeDeviceProfileCacheEntry)
	claudeDeviceProfileCacheMu.Unlock()
}

func newClaudeHeaderTestRequest(t *testing.T, incoming http.Header) *http.Request {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginReq := httptest.NewRequest(http.MethodPost, "http://localhost/v1/messages", nil)
	ginReq.Header = incoming.Clone()
	ginCtx.Request = ginReq

	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	return req.WithContext(context.WithValue(req.Context(), "gin", ginCtx))
}

func assertClaudeFingerprint(t *testing.T, headers http.Header, userAgent, pkgVersion, runtimeVersion, osName, arch string) {
	t.Helper()

	if got := headers.Get("User-Agent"); got != userAgent {
		t.Fatalf("User-Agent = %q, want %q", got, userAgent)
	}
	if got := headers.Get("X-Stainless-Package-Version"); got != pkgVersion {
		t.Fatalf("X-Stainless-Package-Version = %q, want %q", got, pkgVersion)
	}
	if got := headers.Get("X-Stainless-Runtime-Version"); got != runtimeVersion {
		t.Fatalf("X-Stainless-Runtime-Version = %q, want %q", got, runtimeVersion)
	}
	if got := headers.Get("X-Stainless-Os"); got != osName {
		t.Fatalf("X-Stainless-Os = %q, want %q", got, osName)
	}
	if got := headers.Get("X-Stainless-Arch"); got != arch {
		t.Fatalf("X-Stainless-Arch = %q, want %q", got, arch)
	}
}

func TestApplyClaudeHeaders_UsesConfiguredBaselineFingerprint(t *testing.T) {
	resetClaudeDeviceProfileCache()
	stabilize := true

	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.70 (external, cli)",
			PackageVersion:         "0.80.0",
			RuntimeVersion:         "v24.5.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			Timeout:                "900",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-baseline",
		Attributes: map[string]string{
			"api_key":                            "key-baseline",
			"header:User-Agent":                  "evil-client/9.9",
			"header:X-Stainless-Os":              "Linux",
			"header:X-Stainless-Arch":            "x64",
			"header:X-Stainless-Package-Version": "9.9.9",
		},
	}
	incoming := http.Header{
		"User-Agent":                  []string{"curl/8.7.1"},
		"X-Stainless-Package-Version": []string{"0.10.0"},
		"X-Stainless-Runtime-Version": []string{"v18.0.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	}

	req := newClaudeHeaderTestRequest(t, incoming)
	applyClaudeHeaders(req, auth, "key-baseline", false, nil, cfg)

	assertClaudeFingerprint(t, req.Header, "claude-cli/2.1.70 (external, cli)", "0.80.0", "v24.5.0", "MacOS", "arm64")
	if got := req.Header.Get("X-Stainless-Timeout"); got != "900" {
		t.Fatalf("X-Stainless-Timeout = %q, want %q", got, "900")
	}
}

func TestApplyClaudeHeaders_TracksHighestClaudeCLIFingerprint(t *testing.T) {
	resetClaudeDeviceProfileCache()
	stabilize := true

	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.60 (external, cli)",
			PackageVersion:         "0.70.0",
			RuntimeVersion:         "v22.0.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-upgrade",
		Attributes: map[string]string{
			"api_key": "key-upgrade",
		},
	}

	firstReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.62 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.74.0"},
		"X-Stainless-Runtime-Version": []string{"v24.3.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(firstReq, auth, "key-upgrade", false, nil, cfg)
	assertClaudeFingerprint(t, firstReq.Header, "claude-cli/2.1.62 (external, cli)", "0.74.0", "v24.3.0", "MacOS", "arm64")

	thirdPartyReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"lobe-chat/1.0"},
		"X-Stainless-Package-Version": []string{"0.10.0"},
		"X-Stainless-Runtime-Version": []string{"v18.0.0"},
		"X-Stainless-Os":              []string{"Windows"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(thirdPartyReq, auth, "key-upgrade", false, nil, cfg)
	assertClaudeFingerprint(t, thirdPartyReq.Header, "claude-cli/2.1.62 (external, cli)", "0.74.0", "v24.3.0", "MacOS", "arm64")

	higherReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.63 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.75.0"},
		"X-Stainless-Runtime-Version": []string{"v24.4.0"},
		"X-Stainless-Os":              []string{"MacOS"},
		"X-Stainless-Arch":            []string{"arm64"},
	})
	applyClaudeHeaders(higherReq, auth, "key-upgrade", false, nil, cfg)
	assertClaudeFingerprint(t, higherReq.Header, "claude-cli/2.1.63 (external, cli)", "0.75.0", "v24.4.0", "MacOS", "arm64")

	lowerReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.61 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.73.0"},
		"X-Stainless-Runtime-Version": []string{"v24.2.0"},
		"X-Stainless-Os":              []string{"Windows"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(lowerReq, auth, "key-upgrade", false, nil, cfg)
	assertClaudeFingerprint(t, lowerReq.Header, "claude-cli/2.1.63 (external, cli)", "0.75.0", "v24.4.0", "MacOS", "arm64")
}

func TestApplyClaudeHeaders_DoesNotDowngradeConfiguredBaselineOnFirstClaudeClient(t *testing.T) {
	resetClaudeDeviceProfileCache()
	stabilize := true

	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.70 (external, cli)",
			PackageVersion:         "0.80.0",
			RuntimeVersion:         "v24.5.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-baseline-floor",
		Attributes: map[string]string{
			"api_key": "key-baseline-floor",
		},
	}

	olderClaudeReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.62 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.74.0"},
		"X-Stainless-Runtime-Version": []string{"v24.3.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(olderClaudeReq, auth, "key-baseline-floor", false, nil, cfg)
	assertClaudeFingerprint(t, olderClaudeReq.Header, "claude-cli/2.1.70 (external, cli)", "0.80.0", "v24.5.0", "MacOS", "arm64")

	newerClaudeReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.71 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.81.0"},
		"X-Stainless-Runtime-Version": []string{"v24.6.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(newerClaudeReq, auth, "key-baseline-floor", false, nil, cfg)
	assertClaudeFingerprint(t, newerClaudeReq.Header, "claude-cli/2.1.71 (external, cli)", "0.81.0", "v24.6.0", "MacOS", "arm64")
}

func TestApplyClaudeHeaders_UpgradesCachedSoftwareFingerprintWhenBaselineAdvances(t *testing.T) {
	resetClaudeDeviceProfileCache()
	stabilize := true

	oldCfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.70 (external, cli)",
			PackageVersion:         "0.80.0",
			RuntimeVersion:         "v24.5.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	newCfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.77 (external, cli)",
			PackageVersion:         "0.87.0",
			RuntimeVersion:         "v24.8.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-baseline-reload",
		Attributes: map[string]string{
			"api_key": "key-baseline-reload",
		},
	}

	officialReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.71 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.81.0"},
		"X-Stainless-Runtime-Version": []string{"v24.6.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(officialReq, auth, "key-baseline-reload", false, nil, oldCfg)
	assertClaudeFingerprint(t, officialReq.Header, "claude-cli/2.1.71 (external, cli)", "0.81.0", "v24.6.0", "MacOS", "arm64")

	thirdPartyReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"curl/8.7.1"},
		"X-Stainless-Package-Version": []string{"0.10.0"},
		"X-Stainless-Runtime-Version": []string{"v18.0.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(thirdPartyReq, auth, "key-baseline-reload", false, nil, newCfg)
	assertClaudeFingerprint(t, thirdPartyReq.Header, "claude-cli/2.1.77 (external, cli)", "0.87.0", "v24.8.0", "MacOS", "arm64")
}

func TestApplyClaudeHeaders_LearnsOfficialFingerprintAfterCustomBaselineFallback(t *testing.T) {
	resetClaudeDeviceProfileCache()
	stabilize := true

	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "my-gateway/1.0",
			PackageVersion:         "custom-pkg",
			RuntimeVersion:         "custom-runtime",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-custom-baseline-learning",
		Attributes: map[string]string{
			"api_key": "key-custom-baseline-learning",
		},
	}

	thirdPartyReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"curl/8.7.1"},
		"X-Stainless-Package-Version": []string{"0.10.0"},
		"X-Stainless-Runtime-Version": []string{"v18.0.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(thirdPartyReq, auth, "key-custom-baseline-learning", false, nil, cfg)
	assertClaudeFingerprint(t, thirdPartyReq.Header, "my-gateway/1.0", "custom-pkg", "custom-runtime", "MacOS", "arm64")

	officialReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.77 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.87.0"},
		"X-Stainless-Runtime-Version": []string{"v24.8.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(officialReq, auth, "key-custom-baseline-learning", false, nil, cfg)
	assertClaudeFingerprint(t, officialReq.Header, "claude-cli/2.1.77 (external, cli)", "0.87.0", "v24.8.0", "MacOS", "arm64")

	postLearningThirdPartyReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"curl/8.7.1"},
		"X-Stainless-Package-Version": []string{"0.10.0"},
		"X-Stainless-Runtime-Version": []string{"v18.0.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(postLearningThirdPartyReq, auth, "key-custom-baseline-learning", false, nil, cfg)
	assertClaudeFingerprint(t, postLearningThirdPartyReq.Header, "claude-cli/2.1.77 (external, cli)", "0.87.0", "v24.8.0", "MacOS", "arm64")
}

func TestResolveClaudeDeviceProfile_RechecksCacheBeforeStoringCandidate(t *testing.T) {
	resetClaudeDeviceProfileCache()
	stabilize := true

	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.60 (external, cli)",
			PackageVersion:         "0.70.0",
			RuntimeVersion:         "v22.0.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-racy-upgrade",
		Attributes: map[string]string{
			"api_key": "key-racy-upgrade",
		},
	}

	lowPaused := make(chan struct{})
	releaseLow := make(chan struct{})
	var pauseOnce sync.Once
	var releaseOnce sync.Once

	claudeDeviceProfileBeforeCandidateStore = func(candidate claudeDeviceProfile) {
		if candidate.UserAgent != "claude-cli/2.1.62 (external, cli)" {
			return
		}
		pauseOnce.Do(func() { close(lowPaused) })
		<-releaseLow
	}
	t.Cleanup(func() {
		claudeDeviceProfileBeforeCandidateStore = nil
		releaseOnce.Do(func() { close(releaseLow) })
	})

	lowResultCh := make(chan claudeDeviceProfile, 1)
	go func() {
		lowResultCh <- resolveClaudeDeviceProfile(auth, "key-racy-upgrade", http.Header{
			"User-Agent":                  []string{"claude-cli/2.1.62 (external, cli)"},
			"X-Stainless-Package-Version": []string{"0.74.0"},
			"X-Stainless-Runtime-Version": []string{"v24.3.0"},
			"X-Stainless-Os":              []string{"Linux"},
			"X-Stainless-Arch":            []string{"x64"},
		}, cfg)
	}()

	select {
	case <-lowPaused:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lower candidate to pause before storing")
	}

	highResult := resolveClaudeDeviceProfile(auth, "key-racy-upgrade", http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.63 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.75.0"},
		"X-Stainless-Runtime-Version": []string{"v24.4.0"},
		"X-Stainless-Os":              []string{"MacOS"},
		"X-Stainless-Arch":            []string{"arm64"},
	}, cfg)
	releaseOnce.Do(func() { close(releaseLow) })

	select {
	case lowResult := <-lowResultCh:
		if lowResult.UserAgent != "claude-cli/2.1.63 (external, cli)" {
			t.Fatalf("lowResult.UserAgent = %q, want %q", lowResult.UserAgent, "claude-cli/2.1.63 (external, cli)")
		}
		if lowResult.PackageVersion != "0.75.0" {
			t.Fatalf("lowResult.PackageVersion = %q, want %q", lowResult.PackageVersion, "0.75.0")
		}
		if lowResult.OS != "MacOS" || lowResult.Arch != "arm64" {
			t.Fatalf("lowResult platform = %s/%s, want %s/%s", lowResult.OS, lowResult.Arch, "MacOS", "arm64")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lower candidate result")
	}

	if highResult.UserAgent != "claude-cli/2.1.63 (external, cli)" {
		t.Fatalf("highResult.UserAgent = %q, want %q", highResult.UserAgent, "claude-cli/2.1.63 (external, cli)")
	}
	if highResult.OS != "MacOS" || highResult.Arch != "arm64" {
		t.Fatalf("highResult platform = %s/%s, want %s/%s", highResult.OS, highResult.Arch, "MacOS", "arm64")
	}

	cached := resolveClaudeDeviceProfile(auth, "key-racy-upgrade", http.Header{
		"User-Agent": []string{"curl/8.7.1"},
	}, cfg)
	if cached.UserAgent != "claude-cli/2.1.63 (external, cli)" {
		t.Fatalf("cached.UserAgent = %q, want %q", cached.UserAgent, "claude-cli/2.1.63 (external, cli)")
	}
	if cached.PackageVersion != "0.75.0" {
		t.Fatalf("cached.PackageVersion = %q, want %q", cached.PackageVersion, "0.75.0")
	}
	if cached.OS != "MacOS" || cached.Arch != "arm64" {
		t.Fatalf("cached platform = %s/%s, want %s/%s", cached.OS, cached.Arch, "MacOS", "arm64")
	}
}

func TestApplyClaudeHeaders_ThirdPartyBaselineThenOfficialUpgradeKeepsPinnedPlatform(t *testing.T) {
	resetClaudeDeviceProfileCache()
	stabilize := true

	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.70 (external, cli)",
			PackageVersion:         "0.80.0",
			RuntimeVersion:         "v24.5.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-third-party-then-official",
		Attributes: map[string]string{
			"api_key": "key-third-party-then-official",
		},
	}

	thirdPartyReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"curl/8.7.1"},
		"X-Stainless-Package-Version": []string{"0.10.0"},
		"X-Stainless-Runtime-Version": []string{"v18.0.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(thirdPartyReq, auth, "key-third-party-then-official", false, nil, cfg)
	assertClaudeFingerprint(t, thirdPartyReq.Header, "claude-cli/2.1.70 (external, cli)", "0.80.0", "v24.5.0", "MacOS", "arm64")

	officialReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.77 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.87.0"},
		"X-Stainless-Runtime-Version": []string{"v24.8.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(officialReq, auth, "key-third-party-then-official", false, nil, cfg)
	assertClaudeFingerprint(t, officialReq.Header, "claude-cli/2.1.77 (external, cli)", "0.87.0", "v24.8.0", "MacOS", "arm64")
}

func TestApplyClaudeHeaders_DisableDeviceProfileStabilization(t *testing.T) {
	resetClaudeDeviceProfileCache()

	stabilize := false
	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.60 (external, cli)",
			PackageVersion:         "0.70.0",
			RuntimeVersion:         "v22.0.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-disable-stability",
		Attributes: map[string]string{
			"api_key": "key-disable-stability",
		},
	}

	firstReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.62 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.74.0"},
		"X-Stainless-Runtime-Version": []string{"v24.3.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(firstReq, auth, "key-disable-stability", false, nil, cfg)
	assertClaudeFingerprint(t, firstReq.Header, "claude-cli/2.1.62 (external, cli)", "0.74.0", "v24.3.0", "Linux", "x64")

	thirdPartyReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"lobe-chat/1.0"},
		"X-Stainless-Package-Version": []string{"0.10.0"},
		"X-Stainless-Runtime-Version": []string{"v18.0.0"},
		"X-Stainless-Os":              []string{"Windows"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(thirdPartyReq, auth, "key-disable-stability", false, nil, cfg)
	assertClaudeFingerprint(t, thirdPartyReq.Header, "claude-cli/2.1.60 (external, cli)", "0.10.0", "v18.0.0", "Windows", "x64")

	lowerReq := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.61 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.73.0"},
		"X-Stainless-Runtime-Version": []string{"v24.2.0"},
		"X-Stainless-Os":              []string{"Windows"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(lowerReq, auth, "key-disable-stability", false, nil, cfg)
	assertClaudeFingerprint(t, lowerReq.Header, "claude-cli/2.1.61 (external, cli)", "0.73.0", "v24.2.0", "Windows", "x64")
}

func TestApplyClaudeHeaders_LegacyModePreservesConfiguredUserAgentOverrideForClaudeClients(t *testing.T) {
	resetClaudeDeviceProfileCache()

	stabilize := false
	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.60 (external, cli)",
			PackageVersion:         "0.70.0",
			RuntimeVersion:         "v22.0.0",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-legacy-ua-override",
		Attributes: map[string]string{
			"api_key":           "key-legacy-ua-override",
			"header:User-Agent": "config-ua/1.0",
		},
	}

	req := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent":                  []string{"claude-cli/2.1.62 (external, cli)"},
		"X-Stainless-Package-Version": []string{"0.74.0"},
		"X-Stainless-Runtime-Version": []string{"v24.3.0"},
		"X-Stainless-Os":              []string{"Linux"},
		"X-Stainless-Arch":            []string{"x64"},
	})
	applyClaudeHeaders(req, auth, "key-legacy-ua-override", false, nil, cfg)

	assertClaudeFingerprint(t, req.Header, "config-ua/1.0", "0.74.0", "v24.3.0", "Linux", "x64")
}

func TestApplyClaudeHeaders_LegacyModeFallsBackToRuntimeOSArchWhenMissing(t *testing.T) {
	resetClaudeDeviceProfileCache()

	stabilize := false
	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:              "claude-cli/2.1.60 (external, cli)",
			PackageVersion:         "0.70.0",
			RuntimeVersion:         "v22.0.0",
			OS:                     "MacOS",
			Arch:                   "arm64",
			StabilizeDeviceProfile: &stabilize,
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-legacy-runtime-os-arch",
		Attributes: map[string]string{
			"api_key": "key-legacy-runtime-os-arch",
		},
	}

	req := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent": []string{"curl/8.7.1"},
	})
	applyClaudeHeaders(req, auth, "key-legacy-runtime-os-arch", false, nil, cfg)

	assertClaudeFingerprint(t, req.Header, "claude-cli/2.1.60 (external, cli)", "0.70.0", "v22.0.0", mapStainlessOS(), mapStainlessArch())
}

func TestApplyClaudeHeaders_UnsetStabilizationAlsoUsesLegacyRuntimeOSArchFallback(t *testing.T) {
	resetClaudeDeviceProfileCache()

	cfg := &config.Config{
		ClaudeHeaderDefaults: config.ClaudeHeaderDefaults{
			UserAgent:      "claude-cli/2.1.60 (external, cli)",
			PackageVersion: "0.70.0",
			RuntimeVersion: "v22.0.0",
			OS:             "MacOS",
			Arch:           "arm64",
		},
	}
	auth := &cliproxyauth.Auth{
		ID: "auth-unset-runtime-os-arch",
		Attributes: map[string]string{
			"api_key": "key-unset-runtime-os-arch",
		},
	}

	req := newClaudeHeaderTestRequest(t, http.Header{
		"User-Agent": []string{"curl/8.7.1"},
	})
	applyClaudeHeaders(req, auth, "key-unset-runtime-os-arch", false, nil, cfg)

	assertClaudeFingerprint(t, req.Header, "claude-cli/2.1.60 (external, cli)", "0.70.0", "v22.0.0", mapStainlessOS(), mapStainlessArch())
}

func TestClaudeDeviceProfileStabilizationEnabled_DefaultFalse(t *testing.T) {
	if claudeDeviceProfileStabilizationEnabled(nil) {
		t.Fatal("expected nil config to default to disabled stabilization")
	}
	if claudeDeviceProfileStabilizationEnabled(&config.Config{}) {
		t.Fatal("expected unset stabilize-device-profile to default to disabled stabilization")
	}
}

func TestApplyClaudeToolPrefix(t *testing.T) {
	input := []byte(`{"tools":[{"name":"alpha"},{"name":"proxy_bravo"}],"tool_choice":{"type":"tool","name":"charlie"},"messages":[{"role":"assistant","content":[{"type":"tool_use","name":"delta","id":"t1","input":{}}]}]}`)
	out := applyClaudeToolPrefix(input, "proxy_")

	if got := gjson.GetBytes(out, "tools.0.name").String(); got != "proxy_alpha" {
		t.Fatalf("tools.0.name = %q, want %q", got, "proxy_alpha")
	}
	if got := gjson.GetBytes(out, "tools.1.name").String(); got != "proxy_bravo" {
		t.Fatalf("tools.1.name = %q, want %q", got, "proxy_bravo")
	}
	if got := gjson.GetBytes(out, "tool_choice.name").String(); got != "proxy_charlie" {
		t.Fatalf("tool_choice.name = %q, want %q", got, "proxy_charlie")
	}
	if got := gjson.GetBytes(out, "messages.0.content.0.name").String(); got != "proxy_delta" {
		t.Fatalf("messages.0.content.0.name = %q, want %q", got, "proxy_delta")
	}
}

func TestApplyClaudeToolPrefix_WithToolReference(t *testing.T) {
	input := []byte(`{"tools":[{"name":"alpha"}],"messages":[{"role":"user","content":[{"type":"tool_reference","tool_name":"beta"},{"type":"tool_reference","tool_name":"proxy_gamma"}]}]}`)
	out := applyClaudeToolPrefix(input, "proxy_")

	if got := gjson.GetBytes(out, "messages.0.content.0.tool_name").String(); got != "proxy_beta" {
		t.Fatalf("messages.0.content.0.tool_name = %q, want %q", got, "proxy_beta")
	}
	if got := gjson.GetBytes(out, "messages.0.content.1.tool_name").String(); got != "proxy_gamma" {
		t.Fatalf("messages.0.content.1.tool_name = %q, want %q", got, "proxy_gamma")
	}
}

func TestApplyClaudeToolPrefix_SkipsBuiltinTools(t *testing.T) {
	input := []byte(`{"tools":[{"type":"web_search_20250305","name":"web_search"},{"name":"my_custom_tool","input_schema":{"type":"object"}}]}`)
	out := applyClaudeToolPrefix(input, "proxy_")

	if got := gjson.GetBytes(out, "tools.0.name").String(); got != "web_search" {
		t.Fatalf("built-in tool name should not be prefixed: tools.0.name = %q, want %q", got, "web_search")
	}
	if got := gjson.GetBytes(out, "tools.1.name").String(); got != "proxy_my_custom_tool" {
		t.Fatalf("custom tool should be prefixed: tools.1.name = %q, want %q", got, "proxy_my_custom_tool")
	}
}

func TestApplyClaudeToolPrefix_BuiltinToolSkipped(t *testing.T) {
	body := []byte(`{
		"tools": [
			{"type": "web_search_20250305", "name": "web_search", "max_uses": 5},
			{"name": "Read"}
		],
		"messages": [
			{"role": "user", "content": [
				{"type": "tool_use", "name": "web_search", "id": "ws1", "input": {}},
				{"type": "tool_use", "name": "Read", "id": "r1", "input": {}}
			]}
		]
	}`)
	out := applyClaudeToolPrefix(body, "proxy_")

	if got := gjson.GetBytes(out, "tools.0.name").String(); got != "web_search" {
		t.Fatalf("tools.0.name = %q, want %q", got, "web_search")
	}
	if got := gjson.GetBytes(out, "messages.0.content.0.name").String(); got != "web_search" {
		t.Fatalf("messages.0.content.0.name = %q, want %q", got, "web_search")
	}
	if got := gjson.GetBytes(out, "tools.1.name").String(); got != "proxy_Read" {
		t.Fatalf("tools.1.name = %q, want %q", got, "proxy_Read")
	}
	if got := gjson.GetBytes(out, "messages.0.content.1.name").String(); got != "proxy_Read" {
		t.Fatalf("messages.0.content.1.name = %q, want %q", got, "proxy_Read")
	}
}

func TestApplyClaudeToolPrefix_KnownBuiltinInHistoryOnly(t *testing.T) {
	body := []byte(`{
		"tools": [
			{"name": "Read"}
		],
		"messages": [
			{"role": "user", "content": [
				{"type": "tool_use", "name": "web_search", "id": "ws1", "input": {}}
			]}
		]
	}`)
	out := applyClaudeToolPrefix(body, "proxy_")

	if got := gjson.GetBytes(out, "messages.0.content.0.name").String(); got != "web_search" {
		t.Fatalf("messages.0.content.0.name = %q, want %q", got, "web_search")
	}
	if got := gjson.GetBytes(out, "tools.0.name").String(); got != "proxy_Read" {
		t.Fatalf("tools.0.name = %q, want %q", got, "proxy_Read")
	}
}

func TestApplyClaudeToolPrefix_CustomToolsPrefixed(t *testing.T) {
	body := []byte(`{
		"tools": [{"name": "Read"}, {"name": "Write"}],
		"messages": [
			{"role": "user", "content": [
				{"type": "tool_use", "name": "Read", "id": "r1", "input": {}},
				{"type": "tool_use", "name": "Write", "id": "w1", "input": {}}
			]}
		]
	}`)
	out := applyClaudeToolPrefix(body, "proxy_")

	if got := gjson.GetBytes(out, "tools.0.name").String(); got != "proxy_Read" {
		t.Fatalf("tools.0.name = %q, want %q", got, "proxy_Read")
	}
	if got := gjson.GetBytes(out, "tools.1.name").String(); got != "proxy_Write" {
		t.Fatalf("tools.1.name = %q, want %q", got, "proxy_Write")
	}
	if got := gjson.GetBytes(out, "messages.0.content.0.name").String(); got != "proxy_Read" {
		t.Fatalf("messages.0.content.0.name = %q, want %q", got, "proxy_Read")
	}
	if got := gjson.GetBytes(out, "messages.0.content.1.name").String(); got != "proxy_Write" {
		t.Fatalf("messages.0.content.1.name = %q, want %q", got, "proxy_Write")
	}
}

func TestApplyClaudeToolPrefix_ToolChoiceBuiltin(t *testing.T) {
	body := []byte(`{
		"tools": [
			{"type": "web_search_20250305", "name": "web_search"},
			{"name": "Read"}
		],
		"tool_choice": {"type": "tool", "name": "web_search"}
	}`)
	out := applyClaudeToolPrefix(body, "proxy_")

	if got := gjson.GetBytes(out, "tool_choice.name").String(); got != "web_search" {
		t.Fatalf("tool_choice.name = %q, want %q", got, "web_search")
	}
}

func TestStripClaudeToolPrefixFromResponse(t *testing.T) {
	input := []byte(`{"content":[{"type":"tool_use","name":"proxy_alpha","id":"t1","input":{}},{"type":"tool_use","name":"bravo","id":"t2","input":{}}]}`)
	out := stripClaudeToolPrefixFromResponse(input, "proxy_")

	if got := gjson.GetBytes(out, "content.0.name").String(); got != "alpha" {
		t.Fatalf("content.0.name = %q, want %q", got, "alpha")
	}
	if got := gjson.GetBytes(out, "content.1.name").String(); got != "bravo" {
		t.Fatalf("content.1.name = %q, want %q", got, "bravo")
	}
}

func TestStripClaudeToolPrefixFromResponse_WithToolReference(t *testing.T) {
	input := []byte(`{"content":[{"type":"tool_reference","tool_name":"proxy_alpha"},{"type":"tool_reference","tool_name":"bravo"}]}`)
	out := stripClaudeToolPrefixFromResponse(input, "proxy_")

	if got := gjson.GetBytes(out, "content.0.tool_name").String(); got != "alpha" {
		t.Fatalf("content.0.tool_name = %q, want %q", got, "alpha")
	}
	if got := gjson.GetBytes(out, "content.1.tool_name").String(); got != "bravo" {
		t.Fatalf("content.1.tool_name = %q, want %q", got, "bravo")
	}
}

func TestStripClaudeToolPrefixFromStreamLine(t *testing.T) {
	line := []byte(`data: {"type":"content_block_start","content_block":{"type":"tool_use","name":"proxy_alpha","id":"t1"},"index":0}`)
	out := stripClaudeToolPrefixFromStreamLine(line, "proxy_")

	payload := bytes.TrimSpace(out)
	if bytes.HasPrefix(payload, []byte("data:")) {
		payload = bytes.TrimSpace(payload[len("data:"):])
	}
	if got := gjson.GetBytes(payload, "content_block.name").String(); got != "alpha" {
		t.Fatalf("content_block.name = %q, want %q", got, "alpha")
	}
}

func TestStripClaudeToolPrefixFromStreamLine_WithToolReference(t *testing.T) {
	line := []byte(`data: {"type":"content_block_start","content_block":{"type":"tool_reference","tool_name":"proxy_beta"},"index":0}`)
	out := stripClaudeToolPrefixFromStreamLine(line, "proxy_")

	payload := bytes.TrimSpace(out)
	if bytes.HasPrefix(payload, []byte("data:")) {
		payload = bytes.TrimSpace(payload[len("data:"):])
	}
	if got := gjson.GetBytes(payload, "content_block.tool_name").String(); got != "beta" {
		t.Fatalf("content_block.tool_name = %q, want %q", got, "beta")
	}
}

func TestApplyClaudeToolPrefix_NestedToolReference(t *testing.T) {
	input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":[{"type":"tool_reference","tool_name":"mcp__nia__manage_resource"}]}]}]}`)
	out := applyClaudeToolPrefix(input, "proxy_")
	got := gjson.GetBytes(out, "messages.0.content.0.content.0.tool_name").String()
	if got != "proxy_mcp__nia__manage_resource" {
		t.Fatalf("nested tool_reference tool_name = %q, want %q", got, "proxy_mcp__nia__manage_resource")
	}
}

func TestClaudeExecutor_ReusesUserIDAcrossModelsWhenCacheEnabled(t *testing.T) {
	resetUserIDCache()

	var userIDs []string
	var requestModels []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		userID := gjson.GetBytes(body, "metadata.user_id").String()
		model := gjson.GetBytes(body, "model").String()
		userIDs = append(userIDs, userID)
		requestModels = append(requestModels, model)
		t.Logf("HTTP Server received request: model=%s, user_id=%s, url=%s", model, userID, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","model":"claude-3-5-sonnet","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	t.Logf("End-to-end test: Fake HTTP server started at %s", server.URL)

	cacheEnabled := true
	executor := NewClaudeExecutor(&config.Config{
		ClaudeKey: []config.ClaudeKey{
			{
				APIKey:  "key-123",
				BaseURL: server.URL,
				Cloak: &config.CloakConfig{
					CacheUserID: &cacheEnabled,
				},
			},
		},
	})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}

	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	models := []string{"claude-3-5-sonnet", "claude-3-5-haiku"}
	for _, model := range models {
		t.Logf("Sending request for model: %s", model)
		modelPayload, _ := sjson.SetBytes(payload, "model", model)
		if _, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
			Model:   model,
			Payload: modelPayload,
		}, cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString("claude"),
		}); err != nil {
			t.Fatalf("Execute(%s) error: %v", model, err)
		}
	}

	if len(userIDs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(userIDs))
	}
	if userIDs[0] == "" || userIDs[1] == "" {
		t.Fatal("expected user_id to be populated")
	}
	t.Logf("user_id[0] (model=%s): %s", requestModels[0], userIDs[0])
	t.Logf("user_id[1] (model=%s): %s", requestModels[1], userIDs[1])
	if userIDs[0] != userIDs[1] {
		t.Fatalf("expected user_id to be reused across models, got %q and %q", userIDs[0], userIDs[1])
	}
	if !isValidUserID(userIDs[0]) {
		t.Fatalf("user_id %q is not valid", userIDs[0])
	}
	t.Logf("✓ End-to-end test passed: Same user_id (%s) was used for both models", userIDs[0])
}

func TestClaudeExecutor_GeneratesNewUserIDByDefault(t *testing.T) {
	resetUserIDCache()

	var userIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		userIDs = append(userIDs, gjson.GetBytes(body, "metadata.user_id").String())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","model":"claude-3-5-sonnet","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}

	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	for i := 0; i < 2; i++ {
		if _, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
			Model:   "claude-3-5-sonnet",
			Payload: payload,
		}, cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString("claude"),
		}); err != nil {
			t.Fatalf("Execute call %d error: %v", i, err)
		}
	}

	if len(userIDs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(userIDs))
	}
	if userIDs[0] == "" || userIDs[1] == "" {
		t.Fatal("expected user_id to be populated")
	}
	if userIDs[0] == userIDs[1] {
		t.Fatalf("expected user_id to change when caching is not enabled, got identical values %q", userIDs[0])
	}
	if !isValidUserID(userIDs[0]) || !isValidUserID(userIDs[1]) {
		t.Fatalf("user_ids should be valid, got %q and %q", userIDs[0], userIDs[1])
	}
}

func TestStripClaudeToolPrefixFromResponse_NestedToolReference(t *testing.T) {
	input := []byte(`{"content":[{"type":"tool_result","tool_use_id":"toolu_123","content":[{"type":"tool_reference","tool_name":"proxy_mcp__nia__manage_resource"}]}]}`)
	out := stripClaudeToolPrefixFromResponse(input, "proxy_")
	got := gjson.GetBytes(out, "content.0.content.0.tool_name").String()
	if got != "mcp__nia__manage_resource" {
		t.Fatalf("nested tool_reference tool_name = %q, want %q", got, "mcp__nia__manage_resource")
	}
}

func TestApplyClaudeToolPrefix_NestedToolReferenceWithStringContent(t *testing.T) {
	// tool_result.content can be a string - should not be processed
	input := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":"plain string result"}]}]}`)
	out := applyClaudeToolPrefix(input, "proxy_")
	got := gjson.GetBytes(out, "messages.0.content.0.content").String()
	if got != "plain string result" {
		t.Fatalf("string content should remain unchanged = %q", got)
	}
}

func TestApplyClaudeToolPrefix_SkipsBuiltinToolReference(t *testing.T) {
	input := []byte(`{"tools":[{"type":"web_search_20250305","name":"web_search"}],"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"tool_reference","tool_name":"web_search"}]}]}]}`)
	out := applyClaudeToolPrefix(input, "proxy_")
	got := gjson.GetBytes(out, "messages.0.content.0.content.0.tool_name").String()
	if got != "web_search" {
		t.Fatalf("built-in tool_reference should not be prefixed, got %q", got)
	}
}

func TestNormalizeCacheControlTTL_DowngradesLaterOneHourBlocks(t *testing.T) {
	payload := []byte(`{
		"tools": [{"name":"t1","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"system": [{"type":"text","text":"s1","cache_control":{"type":"ephemeral"}}],
		"messages": [{"role":"user","content":[{"type":"text","text":"u1","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]
	}`)

	out := normalizeCacheControlTTL(payload)

	if got := gjson.GetBytes(out, "tools.0.cache_control.ttl").String(); got != "1h" {
		t.Fatalf("tools.0.cache_control.ttl = %q, want %q", got, "1h")
	}
	if gjson.GetBytes(out, "messages.0.content.0.cache_control.ttl").Exists() {
		t.Fatalf("messages.0.content.0.cache_control.ttl should be removed after a default-5m block")
	}
}

func TestNormalizeCacheControlTTL_PreservesOriginalBytesWhenNoChange(t *testing.T) {
	// Payload where no TTL normalization is needed (all blocks use 1h with no
	// preceding 5m block). The text intentionally contains HTML chars (<, >, &)
	// that json.Marshal would escape to \u003c etc., altering byte identity.
	payload := []byte(`{"tools":[{"name":"t1","cache_control":{"type":"ephemeral","ttl":"1h"}}],"system":[{"type":"text","text":"<system-reminder>foo & bar</system-reminder>","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)

	out := normalizeCacheControlTTL(payload)

	if !bytes.Equal(out, payload) {
		t.Fatalf("normalizeCacheControlTTL altered bytes when no change was needed.\noriginal: %s\ngot:      %s", payload, out)
	}
}

func TestEnforceCacheControlLimit_StripsNonLastToolBeforeMessages(t *testing.T) {
	payload := []byte(`{
		"tools": [
			{"name":"t1","cache_control":{"type":"ephemeral"}},
			{"name":"t2","cache_control":{"type":"ephemeral"}}
		],
		"system": [{"type":"text","text":"s1","cache_control":{"type":"ephemeral"}}],
		"messages": [
			{"role":"user","content":[{"type":"text","text":"u1","cache_control":{"type":"ephemeral"}}]},
			{"role":"user","content":[{"type":"text","text":"u2","cache_control":{"type":"ephemeral"}}]}
		]
	}`)

	out := enforceCacheControlLimit(payload, 4)

	if got := countCacheControls(out); got != 4 {
		t.Fatalf("cache_control count = %d, want 4", got)
	}
	if gjson.GetBytes(out, "tools.0.cache_control").Exists() {
		t.Fatalf("tools.0.cache_control should be removed first (non-last tool)")
	}
	if !gjson.GetBytes(out, "tools.1.cache_control").Exists() {
		t.Fatalf("tools.1.cache_control (last tool) should be preserved")
	}
	if !gjson.GetBytes(out, "messages.0.content.0.cache_control").Exists() || !gjson.GetBytes(out, "messages.1.content.0.cache_control").Exists() {
		t.Fatalf("message cache_control blocks should be preserved when non-last tool removal is enough")
	}
}

func TestEnforceCacheControlLimit_ToolOnlyPayloadStillRespectsLimit(t *testing.T) {
	payload := []byte(`{
		"tools": [
			{"name":"t1","cache_control":{"type":"ephemeral"}},
			{"name":"t2","cache_control":{"type":"ephemeral"}},
			{"name":"t3","cache_control":{"type":"ephemeral"}},
			{"name":"t4","cache_control":{"type":"ephemeral"}},
			{"name":"t5","cache_control":{"type":"ephemeral"}}
		]
	}`)

	out := enforceCacheControlLimit(payload, 4)

	if got := countCacheControls(out); got != 4 {
		t.Fatalf("cache_control count = %d, want 4", got)
	}
	if gjson.GetBytes(out, "tools.0.cache_control").Exists() {
		t.Fatalf("tools.0.cache_control should be removed to satisfy max=4")
	}
	if !gjson.GetBytes(out, "tools.4.cache_control").Exists() {
		t.Fatalf("last tool cache_control should be preserved when possible")
	}
}

func TestClaudeExecutor_CountTokens_AppliesCacheControlGuards(t *testing.T) {
	var seenBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seenBody = bytes.Clone(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":42}`))
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}

	payload := []byte(`{
		"tools": [
			{"name":"t1","cache_control":{"type":"ephemeral","ttl":"1h"}},
			{"name":"t2","cache_control":{"type":"ephemeral"}}
		],
		"system": [
			{"type":"text","text":"s1","cache_control":{"type":"ephemeral","ttl":"1h"}},
			{"type":"text","text":"s2","cache_control":{"type":"ephemeral","ttl":"1h"}}
		],
		"messages": [
			{"role":"user","content":[{"type":"text","text":"u1","cache_control":{"type":"ephemeral","ttl":"1h"}}]},
			{"role":"user","content":[{"type":"text","text":"u2","cache_control":{"type":"ephemeral","ttl":"1h"}}]}
		]
	}`)

	_, err := executor.CountTokens(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-haiku-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")})
	if err != nil {
		t.Fatalf("CountTokens error: %v", err)
	}

	if len(seenBody) == 0 {
		t.Fatal("expected count_tokens request body to be captured")
	}
	if got := countCacheControls(seenBody); got > 4 {
		t.Fatalf("count_tokens body has %d cache_control blocks, want <= 4", got)
	}
	if hasTTLOrderingViolation(seenBody) {
		t.Fatalf("count_tokens body still has ttl ordering violations: %s", string(seenBody))
	}
}

func hasTTLOrderingViolation(payload []byte) bool {
	seen5m := false
	violates := false

	checkCC := func(cc gjson.Result) {
		if !cc.Exists() || violates {
			return
		}
		ttl := cc.Get("ttl").String()
		if ttl != "1h" {
			seen5m = true
			return
		}
		if seen5m {
			violates = true
		}
	}

	tools := gjson.GetBytes(payload, "tools")
	if tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			checkCC(tool.Get("cache_control"))
			return !violates
		})
	}

	system := gjson.GetBytes(payload, "system")
	if system.IsArray() {
		system.ForEach(func(_, item gjson.Result) bool {
			checkCC(item.Get("cache_control"))
			return !violates
		})
	}

	messages := gjson.GetBytes(payload, "messages")
	if messages.IsArray() {
		messages.ForEach(func(_, msg gjson.Result) bool {
			content := msg.Get("content")
			if content.IsArray() {
				content.ForEach(func(_, item gjson.Result) bool {
					checkCC(item.Get("cache_control"))
					return !violates
				})
			}
			return !violates
		})
	}

	return violates
}

func TestClaudeExecutor_Execute_InvalidGzipErrorBodyReturnsDecodeMessage(t *testing.T) {
	testClaudeExecutorInvalidCompressedErrorBody(t, func(executor *ClaudeExecutor, auth *cliproxyauth.Auth, payload []byte) error {
		_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
			Model:   "claude-3-5-sonnet-20241022",
			Payload: payload,
		}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")})
		return err
	})
}

func TestClaudeExecutor_ExecuteStream_InvalidGzipErrorBodyReturnsDecodeMessage(t *testing.T) {
	testClaudeExecutorInvalidCompressedErrorBody(t, func(executor *ClaudeExecutor, auth *cliproxyauth.Auth, payload []byte) error {
		_, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
			Model:   "claude-3-5-sonnet-20241022",
			Payload: payload,
		}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")})
		return err
	})
}

func TestClaudeExecutor_CountTokens_InvalidGzipErrorBodyReturnsDecodeMessage(t *testing.T) {
	testClaudeExecutorInvalidCompressedErrorBody(t, func(executor *ClaudeExecutor, auth *cliproxyauth.Auth, payload []byte) error {
		_, err := executor.CountTokens(context.Background(), auth, cliproxyexecutor.Request{
			Model:   "claude-3-5-sonnet-20241022",
			Payload: payload,
		}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")})
		return err
	})
}

func testClaudeExecutorInvalidCompressedErrorBody(
	t *testing.T,
	invoke func(executor *ClaudeExecutor, auth *cliproxyauth.Auth, payload []byte) error,
) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("not-a-valid-gzip-stream"))
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	err := invoke(executor, auth, payload)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to decode error response body") {
		t.Fatalf("expected decode failure message, got: %v", err)
	}
	if statusProvider, ok := err.(interface{ StatusCode() int }); !ok || statusProvider.StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected status code 400, got: %v", err)
	}
}

// TestClaudeExecutor_ExecuteStream_SetsIdentityAcceptEncoding verifies that streaming
// requests use Accept-Encoding: identity so the upstream cannot respond with a
// compressed SSE body that would silently break the line scanner.
func TestClaudeExecutor_ExecuteStream_SetsIdentityAcceptEncoding(t *testing.T) {
	var gotEncoding, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Accept-Encoding")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-sonnet-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected chunk error: %v", chunk.Err)
		}
	}

	if gotEncoding != "identity" {
		t.Errorf("Accept-Encoding = %q, want %q", gotEncoding, "identity")
	}
	if gotAccept != "text/event-stream" {
		t.Errorf("Accept = %q, want %q", gotAccept, "text/event-stream")
	}
}

// TestClaudeExecutor_Execute_SetsCompressedAcceptEncoding verifies that non-streaming
// requests keep the full accept-encoding to allow response compression (which
// decodeResponseBody handles correctly).
func TestClaudeExecutor_Execute_SetsCompressedAcceptEncoding(t *testing.T) {
	var gotEncoding, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Accept-Encoding")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","model":"claude-3-5-sonnet-20241022","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-sonnet-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if gotEncoding != "gzip, deflate, br, zstd" {
		t.Errorf("Accept-Encoding = %q, want %q", gotEncoding, "gzip, deflate, br, zstd")
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want %q", gotAccept, "application/json")
	}
}

// TestClaudeExecutor_ExecuteStream_GzipSuccessBodyDecoded verifies that a streaming
// HTTP 200 response with Content-Encoding: gzip is correctly decompressed before
// the line scanner runs, so SSE chunks are not silently dropped.
func TestClaudeExecutor_ExecuteStream_GzipSuccessBodyDecoded(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("data: {\"type\":\"message_stop\"}\n"))
	_ = gz.Close()
	compressedBody := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressedBody)
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-sonnet-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var combined strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		combined.Write(chunk.Payload)
	}

	if combined.Len() == 0 {
		t.Fatal("expected at least one chunk from gzip-encoded SSE body, got none (body was not decompressed)")
	}
	if !strings.Contains(combined.String(), "message_stop") {
		t.Errorf("expected SSE content in chunks, got: %q", combined.String())
	}
}

// TestDecodeResponseBody_MagicByteGzipNoHeader verifies that decodeResponseBody
// detects gzip-compressed content via magic bytes even when Content-Encoding is absent.
func TestDecodeResponseBody_MagicByteGzipNoHeader(t *testing.T) {
	const plaintext = "data: {\"type\":\"message_stop\"}\n"

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(plaintext))
	_ = gz.Close()

	rc := io.NopCloser(&buf)
	decoded, err := decodeResponseBody(rc, "")
	if err != nil {
		t.Fatalf("decodeResponseBody error: %v", err)
	}
	defer decoded.Close()

	got, err := io.ReadAll(decoded)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(got) != plaintext {
		t.Errorf("decoded = %q, want %q", got, plaintext)
	}
}

// TestDecodeResponseBody_PlainTextNoHeader verifies that decodeResponseBody returns
// plain text untouched when Content-Encoding is absent and no magic bytes match.
func TestDecodeResponseBody_PlainTextNoHeader(t *testing.T) {
	const plaintext = "data: {\"type\":\"message_stop\"}\n"
	rc := io.NopCloser(strings.NewReader(plaintext))
	decoded, err := decodeResponseBody(rc, "")
	if err != nil {
		t.Fatalf("decodeResponseBody error: %v", err)
	}
	defer decoded.Close()

	got, err := io.ReadAll(decoded)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(got) != plaintext {
		t.Errorf("decoded = %q, want %q", got, plaintext)
	}
}

// TestClaudeExecutor_ExecuteStream_GzipNoContentEncodingHeader verifies the full
// pipeline: when the upstream returns a gzip-compressed SSE body WITHOUT setting
// Content-Encoding (a misbehaving upstream), the magic-byte sniff in
// decodeResponseBody still decompresses it, so chunks reach the caller.
func TestClaudeExecutor_ExecuteStream_GzipNoContentEncodingHeader(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("data: {\"type\":\"message_stop\"}\n"))
	_ = gz.Close()
	compressedBody := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Intentionally omit Content-Encoding to simulate misbehaving upstream.
		_, _ = w.Write(compressedBody)
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-sonnet-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var combined strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		combined.Write(chunk.Payload)
	}

	if combined.Len() == 0 {
		t.Fatal("expected chunks from gzip body without Content-Encoding header, got none (magic-byte sniff failed)")
	}
	if !strings.Contains(combined.String(), "message_stop") {
		t.Errorf("unexpected chunk content: %q", combined.String())
	}
}

// TestClaudeExecutor_ExecuteStream_AcceptEncodingOverrideCannotBypassIdentity verifies
// that injecting Accept-Encoding via auth.Attributes cannot override the stream
// path's enforced identity encoding.
func TestClaudeExecutor_ExecuteStream_AcceptEncodingOverrideCannotBypassIdentity(t *testing.T) {
	var gotEncoding string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	// Inject Accept-Encoding via the custom header attribute mechanism.
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":                "key-123",
		"base_url":               server.URL,
		"header:Accept-Encoding": "gzip, deflate, br, zstd",
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-sonnet-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected chunk error: %v", chunk.Err)
		}
	}

	if gotEncoding != "identity" {
		t.Errorf("Accept-Encoding = %q; stream path must enforce identity regardless of auth.Attributes override", gotEncoding)
	}
}

// TestDecodeResponseBody_MagicByteZstdNoHeader verifies that decodeResponseBody
// detects zstd-compressed content via magic bytes (28 b5 2f fd) even when
// Content-Encoding is absent.
func TestDecodeResponseBody_MagicByteZstdNoHeader(t *testing.T) {
	const plaintext = "data: {\"type\":\"message_stop\"}\n"

	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	_, _ = enc.Write([]byte(plaintext))
	_ = enc.Close()

	rc := io.NopCloser(&buf)
	decoded, err := decodeResponseBody(rc, "")
	if err != nil {
		t.Fatalf("decodeResponseBody error: %v", err)
	}
	defer decoded.Close()

	got, err := io.ReadAll(decoded)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(got) != plaintext {
		t.Errorf("decoded = %q, want %q", got, plaintext)
	}
}

// TestClaudeExecutor_Execute_GzipErrorBodyNoContentEncodingHeader verifies that the
// error path (4xx) correctly decompresses a gzip body even when the upstream omits
// the Content-Encoding header.  This closes the gap left by PR #1771, which only
// fixed header-declared compression on the error path.
func TestClaudeExecutor_Execute_GzipErrorBodyNoContentEncodingHeader(t *testing.T) {
	const errJSON = `{"type":"error","error":{"type":"invalid_request_error","message":"test error"}}`

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(errJSON))
	_ = gz.Close()
	compressedBody := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Intentionally omit Content-Encoding to simulate misbehaving upstream.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(compressedBody)
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-sonnet-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
	})
	if err == nil {
		t.Fatal("expected an error for 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "test error") {
		t.Errorf("error message should contain decompressed JSON, got: %q", err.Error())
	}
}

// TestClaudeExecutor_ExecuteStream_GzipErrorBodyNoContentEncodingHeader verifies
// the same for the streaming executor: 4xx gzip body without Content-Encoding is
// decoded and the error message is readable.
func TestClaudeExecutor_ExecuteStream_GzipErrorBodyNoContentEncodingHeader(t *testing.T) {
	const errJSON = `{"type":"error","error":{"type":"invalid_request_error","message":"stream test error"}}`

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(errJSON))
	_ = gz.Close()
	compressedBody := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Intentionally omit Content-Encoding to simulate misbehaving upstream.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(compressedBody)
	}))
	defer server.Close()

	executor := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "key-123",
		"base_url": server.URL,
	}}
	payload := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)

	_, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "claude-3-5-sonnet-20241022",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("claude"),
	})
	if err == nil {
		t.Fatal("expected an error for 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "stream test error") {
		t.Errorf("error message should contain decompressed JSON, got: %q", err.Error())
	}
}

// Test case 1: String system prompt is preserved and converted to a content block
func TestCheckSystemInstructionsWithMode_StringSystemPreserved(t *testing.T) {
	payload := []byte(`{"system":"You are a helpful assistant.","messages":[{"role":"user","content":"hi"}]}`)

	out := checkSystemInstructionsWithMode(payload, false)

	system := gjson.GetBytes(out, "system")
	if !system.IsArray() {
		t.Fatalf("system should be an array, got %s", system.Type)
	}

	blocks := system.Array()
	if len(blocks) != 3 {
		t.Fatalf("expected 3 system blocks, got %d", len(blocks))
	}

	if !strings.HasPrefix(blocks[0].Get("text").String(), "x-anthropic-billing-header:") {
		t.Fatalf("blocks[0] should be billing header, got %q", blocks[0].Get("text").String())
	}
	if blocks[1].Get("text").String() != "You are a Claude agent, built on Anthropic's Claude Agent SDK." {
		t.Fatalf("blocks[1] should be agent block, got %q", blocks[1].Get("text").String())
	}
	if blocks[2].Get("text").String() != "You are a helpful assistant." {
		t.Fatalf("blocks[2] should be user system prompt, got %q", blocks[2].Get("text").String())
	}
	if blocks[2].Get("cache_control.type").String() != "ephemeral" {
		t.Fatalf("blocks[2] should have cache_control.type=ephemeral")
	}
}

// Test case 2: Strict mode drops the string system prompt
func TestCheckSystemInstructionsWithMode_StringSystemStrict(t *testing.T) {
	payload := []byte(`{"system":"You are a helpful assistant.","messages":[{"role":"user","content":"hi"}]}`)

	out := checkSystemInstructionsWithMode(payload, true)

	blocks := gjson.GetBytes(out, "system").Array()
	if len(blocks) != 2 {
		t.Fatalf("strict mode should produce 2 blocks, got %d", len(blocks))
	}
}

// Test case 3: Empty string system prompt does not produce a spurious block
func TestCheckSystemInstructionsWithMode_EmptyStringSystemIgnored(t *testing.T) {
	payload := []byte(`{"system":"","messages":[{"role":"user","content":"hi"}]}`)

	out := checkSystemInstructionsWithMode(payload, false)

	blocks := gjson.GetBytes(out, "system").Array()
	if len(blocks) != 2 {
		t.Fatalf("empty string system should produce 2 blocks, got %d", len(blocks))
	}
}

// Test case 4: Array system prompt is unaffected by the string handling
func TestCheckSystemInstructionsWithMode_ArraySystemStillWorks(t *testing.T) {
	payload := []byte(`{"system":[{"type":"text","text":"Be concise."}],"messages":[{"role":"user","content":"hi"}]}`)

	out := checkSystemInstructionsWithMode(payload, false)

	blocks := gjson.GetBytes(out, "system").Array()
	if len(blocks) != 3 {
		t.Fatalf("expected 3 system blocks, got %d", len(blocks))
	}
	if blocks[2].Get("text").String() != "Be concise." {
		t.Fatalf("blocks[2] should be user system prompt, got %q", blocks[2].Get("text").String())
	}
}

// Test case 5: Special characters in string system prompt survive conversion
func TestCheckSystemInstructionsWithMode_StringWithSpecialChars(t *testing.T) {
	payload := []byte(`{"system":"Use <xml> tags & \"quotes\" in output.","messages":[{"role":"user","content":"hi"}]}`)

	out := checkSystemInstructionsWithMode(payload, false)

	blocks := gjson.GetBytes(out, "system").Array()
	if len(blocks) != 3 {
		t.Fatalf("expected 3 system blocks, got %d", len(blocks))
	}
	if blocks[2].Get("text").String() != `Use <xml> tags & "quotes" in output.` {
		t.Fatalf("blocks[2] text mangled, got %q", blocks[2].Get("text").String())
	}
}

func TestEnsureKimiToolCallThinkingBlock(t *testing.T) {
	t.Run("kimi-for-coding prepends thinking block when missing", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallThinkingBlock("kimi-for-coding", input)
		msg := gjson.GetBytes(out, "messages.0")
		if got := msg.Get("content.0.type").String(); got != "thinking" {
			t.Fatalf("content.0.type=%q, want %q", got, "thinking")
		}
		if got := msg.Get("content.0.thinking").String(); strings.TrimSpace(got) == "" {
			t.Fatalf("expected non-empty content.0.thinking")
		}
		if got := msg.Get("content.1.type").String(); got != "tool_use" {
			t.Fatalf("content.1.type=%q, want %q", got, "tool_use")
		}
	})

	t.Run("kimi-for-coding patches empty thinking block", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"   "},{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallThinkingBlock("kimi-for-coding", input)
		if got := strings.TrimSpace(gjson.GetBytes(out, "messages.0.content.0.thinking").String()); got == "" {
			t.Fatalf("expected patched non-empty thinking")
		}
	})

	t.Run("kimi-for-coding does not override existing non-empty thinking", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"keep"},{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallThinkingBlock("kimi-for-coding", input)
		if got := gjson.GetBytes(out, "messages.0.content.0.thinking").String(); got != "keep" {
			t.Fatalf("thinking=%q, want %q", got, "keep")
		}
	})

	t.Run("non-kimi model is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"claude-3-5-sonnet-20241022","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallThinkingBlock("claude-3-5-sonnet-20241022", input)
		if got := gjson.GetBytes(out, "messages.0.content.0.type").String(); got != "tool_use" {
			t.Fatalf("expected content.0.type to remain %q, got %q", "tool_use", got)
		}
	})

	t.Run("kimi-for-coding without thinking enabled is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallThinkingBlock("kimi-for-coding", input)
		if got := gjson.GetBytes(out, "messages.0.content.0.type").String(); got != "tool_use" {
			t.Fatalf("expected content.0.type to remain %q, got %q", "tool_use", got)
		}
	})

	t.Run("kimi-for-coding assistant text message is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"text","text":"hi"}]}]}`)
		out := ensureKimiToolCallThinkingBlock("kimi-for-coding", input)
		if got := gjson.GetBytes(out, "messages.0.content.0.type").String(); got != "text" {
			t.Fatalf("expected content.0.type to remain %q, got %q", "text", got)
		}
	})
}

func TestEnsureKimiToolCallReasoningContent(t *testing.T) {
	t.Run("kimi-for-coding patches missing reasoning_content for assistant tool_use", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if got := strings.TrimSpace(gjson.GetBytes(out, "messages.0.reasoning_content").String()); got == "" {
			t.Fatalf("expected non-empty messages.0.reasoning_content")
		}
	})

	t.Run("kimi-for-coding patches blank reasoning_content for assistant tool_use", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","reasoning_content":"   ","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if got := strings.TrimSpace(gjson.GetBytes(out, "messages.0.reasoning_content").String()); got == "" {
			t.Fatalf("expected patched non-empty messages.0.reasoning_content")
		}
	})

	t.Run("non-kimi model is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"claude-3-5-sonnet-20241022","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("claude-3-5-sonnet-20241022", input)
		if gjson.GetBytes(out, "messages.0.reasoning_content").Exists() {
			t.Fatalf("expected non-kimi model to remain unchanged")
		}
	})

	t.Run("kimi-for-coding without thinking enabled is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if gjson.GetBytes(out, "messages.0.reasoning_content").Exists() {
			t.Fatalf("expected when thinking disabled we do not inject reasoning_content")
		}
	})
}

func TestClaudeExecutor_ExecuteStream_KimiToolUseThinkingBlockPatched(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		captured = b

		content := gjson.GetBytes(b, "messages.1.content")
		hasValidThinking := false
		content.ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "thinking" && strings.TrimSpace(part.Get("thinking").String()) != "" {
				hasValidThinking = true
				return false
			}
			return true
		})
		rc := strings.TrimSpace(gjson.GetBytes(b, "messages.1.reasoning_content").String())
		if !hasValidThinking || rc == "" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("thinking is enabled but reasoning_content is missing in assistant tool call message at index 2"))
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{"api_key": "test", "base_url": server.URL}}
	payload := []byte(`{"model":"kimi-for-coding","max_tokens":1,"stream":true,"thinking":{"type":"enabled","budget_tokens":10},"tools":[{"name":"my_tool","input_schema":{"type":"object"}}],"tool_choice":{"type":"auto"},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`)
	req := cliproxyexecutor.Request{Model: "kimi-for-coding", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude"), OriginalRequest: payload}

	result, err := exec.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected chunk error: %v", chunk.Err)
		}
	}
	if len(captured) == 0 {
		t.Fatalf("expected upstream request to be captured")
	}
	content := gjson.GetBytes(captured, "messages.1.content")
	found := false
	content.ForEach(func(_, part gjson.Result) bool {
		if part.Get("type").String() == "thinking" && strings.TrimSpace(part.Get("thinking").String()) != "" {
			found = true
			return false
		}
		return true
	})
	if !found {
		t.Fatalf("expected upstream assistant tool_use message to include a non-empty thinking block")
	}
	if got := strings.TrimSpace(gjson.GetBytes(captured, "messages.1.reasoning_content").String()); got == "" {
		t.Fatalf("expected upstream assistant tool_use message to include non-empty reasoning_content")
	}
}

func TestClaudeExecutor_Execute_KimiToolUseThinkingBlockPatched(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		captured = b

		content := gjson.GetBytes(b, "messages.1.content")
		hasValidThinking := false
		content.ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "thinking" && strings.TrimSpace(part.Get("thinking").String()) != "" {
				hasValidThinking = true
				return false
			}
			return true
		})
		rc := strings.TrimSpace(gjson.GetBytes(b, "messages.1.reasoning_content").String())
		if !hasValidThinking || rc == "" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("thinking is enabled but reasoning_content is missing in assistant tool call message at index 2"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"kimi-for-coding","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{"api_key": "test", "base_url": server.URL}}
	payload := []byte(`{"model":"kimi-for-coding","max_tokens":1,"stream":false,"thinking":{"type":"enabled","budget_tokens":10},"tools":[{"name":"my_tool","input_schema":{"type":"object"}}],"tool_choice":{"type":"auto"},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`)
	req := cliproxyexecutor.Request{Model: "kimi-for-coding", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude"), OriginalRequest: payload}

	_, err := exec.Execute(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(captured) == 0 {
		t.Fatalf("expected upstream request to be captured")
	}
	content := gjson.GetBytes(captured, "messages.1.content")
	found := false
	content.ForEach(func(_, part gjson.Result) bool {
		if part.Get("type").String() == "thinking" && strings.TrimSpace(part.Get("thinking").String()) != "" {
			found = true
			return false
		}
		return true
	})
	if !found {
		t.Fatalf("expected upstream assistant tool_use message to include a non-empty thinking block")
	}
	if got := strings.TrimSpace(gjson.GetBytes(captured, "messages.1.reasoning_content").String()); got == "" {
		t.Fatalf("expected upstream assistant tool_use message to include non-empty reasoning_content")
	}
}

func TestClaudeExecutor_Execute_KimiToolUsePatched_WhenRequestModelIsAliasedUpstream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		captured = b

		content := gjson.GetBytes(b, "messages.1.content")
		hasValidThinking := false
		content.ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "thinking" && strings.TrimSpace(part.Get("thinking").String()) != "" {
				hasValidThinking = true
				return false
			}
			return true
		})
		rc := strings.TrimSpace(gjson.GetBytes(b, "messages.1.reasoning_content").String())
		if !hasValidThinking || rc == "" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("thinking is enabled but reasoning_content is missing in assistant tool call message at index 2"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"moonshotai/kimi-k2.5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{"api_key": "test", "base_url": server.URL}}
	payload := []byte(`{"model":"kimi-for-coding","max_tokens":1,"stream":false,"thinking":{"type":"enabled","budget_tokens":10},"tools":[{"name":"my_tool","input_schema":{"type":"object"}}],"tool_choice":{"type":"auto"},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`)
	req := cliproxyexecutor.Request{Model: "moonshotai/kimi-k2.5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude"), OriginalRequest: payload}

	_, err := exec.Execute(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(captured) == 0 {
		t.Fatalf("expected upstream request to be captured")
	}
	if got := strings.TrimSpace(gjson.GetBytes(captured, "messages.1.reasoning_content").String()); got == "" {
		t.Fatalf("expected upstream assistant tool_use message to include non-empty reasoning_content")
	}
	content := gjson.GetBytes(captured, "messages.1.content")
	foundThinking := false
	content.ForEach(func(_, part gjson.Result) bool {
		if part.Get("type").String() == "thinking" && strings.TrimSpace(part.Get("thinking").String()) != "" {
			foundThinking = true
			return false
		}
		return true
	})
	if !foundThinking {
		t.Fatalf("expected upstream assistant tool_use message to include a non-empty thinking block")
	}
}

// makeGinCtxWithUA creates a gin test context with the given User-Agent and optional Anthropic-Beta
// header, and returns a context.Context with the gin context embedded under key "gin".
func makeGinCtxWithUA(ua, betaHeader string) context.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)
	ginCtx.Request, _ = http.NewRequest(http.MethodPost, "/v1/messages", nil)
	ginCtx.Request.Header.Set("User-Agent", ua)
	if betaHeader != "" {
		ginCtx.Request.Header.Set("Anthropic-Beta", betaHeader)
	}
	return context.WithValue(context.Background(), "gin", ginCtx)
}

// Scenario A: claude-cli UA + OAuth token → upstream receives original tool names (no proxy_ prefix).
func TestClaudeExecutor_NativePassthrough_BodyUnchanged(t *testing.T) {
	var toolNames []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gjson.GetBytes(b, "tools").ForEach(func(_, v gjson.Result) bool {
			toolNames = append(toolNames, v.Get("name").String())
			return true
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	ctx := makeGinCtxWithUA("claude-cli/2.1.62 (external, sdk-cli)", "")
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"tools":[{"name":"Read","input_schema":{"type":"object"}},{"name":"Write","input_schema":{"type":"object"}}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	for _, name := range toolNames {
		if name == "proxy_Read" || name == "proxy_Write" {
			t.Fatalf("expected no proxy_ prefix on tool names for claude-cli UA, got %q in %v", name, toolNames)
		}
	}
	var found bool
	for _, name := range toolNames {
		if name == "Read" || name == "Write" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tool names 'Read'/'Write' in upstream request, got %v", toolNames)
	}
}

// Scenario B: claude-cli UA + OAuth token → Anthropic-Beta header forwarded unchanged.
func TestClaudeExecutor_NativePassthrough_BetaHeaderUnchanged(t *testing.T) {
	clientBeta := "claude-code-20250219,oauth-2025-04-20,prompt-caching-scope-2026-01-05"
	var capturedBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBeta = r.Header.Get("Anthropic-Beta")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	ctx := makeGinCtxWithUA("claude-cli/2.1.62 (external, sdk-cli)", clientBeta)
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if capturedBeta != clientBeta {
		t.Fatalf("expected upstream Anthropic-Beta=%q, got %q", clientBeta, capturedBeta)
	}
}

// Scenario C: claude-cli UA + OAuth token → X-Stainless-Helper-Method not injected.
func TestClaudeExecutor_NativePassthrough_NoStainlessHelperMethod(t *testing.T) {
	var capturedStainless string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStainless = r.Header.Get("X-Stainless-Helper-Method")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	ctx := makeGinCtxWithUA("claude-cli/2.1.62 (external, sdk-cli)", "")
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if capturedStainless != "" {
		t.Fatalf("expected no X-Stainless-Helper-Method header for claude-cli UA, got %q", capturedStainless)
	}
}

// Scenario D: claude-cli UA + OAuth token → SSE stream tool names forwarded unchanged (no prefix strip).
func TestClaudeExecutor_NativePassthrough_StreamResponseUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"proxy_Read","id":"t1"}}` + "\n\n"))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	ctx, cancel := context.WithTimeout(makeGinCtxWithUA("claude-cli/2.1.62 (external, sdk-cli)", ""), 5*time.Second)
	defer cancel()

	payload := []byte(`{"model":"claude-sonnet-4-5","stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	stream, err := exec.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}
	var lines []string
	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			break
		}
		lines = append(lines, string(chunk.Payload))
	}
	combined := strings.Join(lines, "")
	if !strings.Contains(combined, "proxy_Read") {
		t.Fatalf("expected stream to contain 'proxy_Read' unchanged, got: %q", combined)
	}
}

// Scenario E: non-claude-cli UA + OAuth token → proxy_ prefix applied normally (regression guard).
// Security: X-Api-Key and Authorization from client must not reach upstream.
func TestClaudeExecutor_NativePassthrough_ClientCredentialHeadersNotLeaked(t *testing.T) {
	var capturedXApiKey, capturedAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedXApiKey = r.Header.Get("X-Api-Key")
		capturedAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-upstreamtoken",
		"base_url": server.URL,
	}}
	// Client authenticates to the proxy using X-Api-Key (proxy credential, must not leak).
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request, _ = http.NewRequest(http.MethodPost, "/v1/messages", nil)
	ginCtx.Request.Header.Set("User-Agent", "claude-cli/2.1.62 (external, sdk-cli)")
	ginCtx.Request.Header.Set("X-Api-Key", "proxy-client-secret")
	ginCtx.Request.Header.Set("Authorization", "Bearer proxy-client-secret")
	ctx := context.WithValue(context.Background(), "gin", ginCtx)

	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if capturedXApiKey == "proxy-client-secret" {
		t.Fatal("X-Api-Key proxy credential leaked to upstream")
	}
	// Upstream Authorization must be the upstream token, not the proxy credential.
	if capturedAuthorization != "Bearer sk-ant-oat01-upstreamtoken" {
		t.Fatalf("expected upstream Authorization=Bearer sk-ant-oat01-upstreamtoken, got %q", capturedAuthorization)
	}
}

// Security: non-claude source format must not trigger nativePassthrough even with claude-cli UA + OAuth token.
func TestClaudeExecutor_NativePassthrough_NonClaudeSourceFormatTranslated(t *testing.T) {
	var capturedAnthropicVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAnthropicVersion = r.Header.Get("Anthropic-Version")
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	// makeGinCtxWithUA intentionally omits Anthropic-Version from the client request.
	// applyClaudeHeaders (non-passthrough path) always injects "2023-06-01".
	// nativePassthrough merely forwards client headers, so Anthropic-Version would be absent.
	ctx := makeGinCtxWithUA("claude-cli/2.1.62 (external, sdk-cli)", "")
	openAIPayload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: openAIPayload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// applyClaudeHeaders sets Anthropic-Version: 2023-06-01; nativePassthrough would leave it
	// absent (client didn't send it).  Presence of the header proves the non-passthrough path ran.
	if capturedAnthropicVersion != "2023-06-01" {
		t.Fatalf("expected upstream Anthropic-Version=2023-06-01 (applyClaudeHeaders ran), got %q — nativePassthrough may have fired incorrectly", capturedAnthropicVersion)
	}
}

func TestClaudeExecutor_NativePassthrough_ThirdPartyClientUnaffected(t *testing.T) {
	var toolNames []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gjson.GetBytes(b, "tools").ForEach(func(_, v gjson.Result) bool {
			toolNames = append(toolNames, v.Get("name").String())
			return true
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	ctx := makeGinCtxWithUA("cursor/1.0", "")
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	var found bool
	for _, name := range toolNames {
		if name == "Read" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'Read' (no prefix) in upstream request for non-claude-cli UA, got %v", toolNames)
	}
}

// Security: X-Goog-Api-Key used as proxy credential must not be forwarded to upstream in nativePassthrough.
// provider.go accepts X-Goog-Api-Key as a valid proxy auth source; this test ensures the header
// is scrubbed from outgoing upstream requests just like Authorization and X-Api-Key are.
func TestClaudeExecutor_NativePassthrough_XGoogApiKeyNotLeaked(t *testing.T) {
	const proxySecret = "proxy-goog-secret"
	const upstreamToken = "sk-ant-oat01-upstreamtoken"

	t.Run("Execute", func(t *testing.T) {
		var capturedGoog string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedGoog = r.Header.Get("X-Goog-Api-Key")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
		}))
		defer server.Close()

		exec := NewClaudeExecutor(&config.Config{})
		auth := &cliproxyauth.Auth{Attributes: map[string]string{
			"api_key":  upstreamToken,
			"base_url": server.URL,
		}}
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request, _ = http.NewRequest(http.MethodPost, "/v1/messages", nil)
		ginCtx.Request.Header.Set("User-Agent", "claude-cli/2.1.62 (external, sdk-cli)")
		ginCtx.Request.Header.Set("X-Goog-Api-Key", proxySecret)
		ctx := context.WithValue(context.Background(), "gin", ginCtx)

		payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
		req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
		opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

		if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if capturedGoog == proxySecret {
			t.Fatal("X-Goog-Api-Key proxy credential leaked to upstream via Execute")
		}
	})

	t.Run("ExecuteStream", func(t *testing.T) {
		var capturedGoog string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedGoog = r.Header.Get("X-Goog-Api-Key")
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
		}))
		defer server.Close()

		exec := NewClaudeExecutor(&config.Config{})
		auth := &cliproxyauth.Auth{Attributes: map[string]string{
			"api_key":  upstreamToken,
			"base_url": server.URL,
		}}
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request, _ = http.NewRequest(http.MethodPost, "/v1/messages", nil)
		ginCtx.Request.Header.Set("User-Agent", "claude-cli/2.1.62 (external, sdk-cli)")
		ginCtx.Request.Header.Set("X-Goog-Api-Key", proxySecret)
		ctx := context.WithValue(context.Background(), "gin", ginCtx)

		payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"stream":true}`)
		req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
		opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

		result, err := exec.ExecuteStream(ctx, auth, req, opts)
		if err != nil {
			t.Fatalf("ExecuteStream returned error: %v", err)
		}
		// Drain the stream.
		for range result.Chunks {
		}
		if capturedGoog == proxySecret {
			t.Fatal("X-Goog-Api-Key proxy credential leaked to upstream via ExecuteStream")
		}
	})

	t.Run("CountTokens", func(t *testing.T) {
		var capturedGoog string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedGoog = r.Header.Get("X-Goog-Api-Key")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"input_tokens":10}`))
		}))
		defer server.Close()

		exec := NewClaudeExecutor(&config.Config{})
		auth := &cliproxyauth.Auth{Attributes: map[string]string{
			"api_key":  upstreamToken,
			"base_url": server.URL,
		}}
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request, _ = http.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
		ginCtx.Request.Header.Set("User-Agent", "claude-cli/2.1.62 (external, sdk-cli)")
		ginCtx.Request.Header.Set("X-Goog-Api-Key", proxySecret)
		ctx := context.WithValue(context.Background(), "gin", ginCtx)

		payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
		req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
		opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

		if _, err := exec.CountTokens(ctx, auth, req, opts); err != nil {
			t.Fatalf("CountTokens returned error: %v", err)
		}
		if capturedGoog == proxySecret {
			t.Fatal("X-Goog-Api-Key proxy credential leaked to upstream via CountTokens")
		}
	})
}

// Success criterion 5: 指标采集在 nativePassthrough 路径（Execute）继续工作。
// nativePassthrough 不绕过 reporter.publish；parseClaudeUsage 解析上游响应 usage 字段。
func TestClaudeExecutor_NativePassthrough_UsageRecordPublished_Execute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`))
	}))
	defer server.Close()

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()
	var got usage.Record
	publishUsageRecord = func(_ context.Context, record usage.Record) { got = record }

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	ctx := makeGinCtxWithUA("claude-cli/2.1.62 (external, sdk-cli)", "")
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got.Detail.InputTokens != 5 {
		t.Errorf("expected InputTokens=5, got %d — usage record not published or wrong in nativePassthrough Execute path", got.Detail.InputTokens)
	}
	if got.Detail.OutputTokens != 3 {
		t.Errorf("expected OutputTokens=3, got %d — usage record not published or wrong in nativePassthrough Execute path", got.Detail.OutputTokens)
	}
}

// Success criterion 5: 指标采集在 nativePassthrough 路径（ExecuteStream）继续工作。
// parseClaudeStreamUsage 解析 SSE 流中含 usage 字段的行。
func TestClaudeExecutor_NativePassthrough_UsageRecordPublished_ExecuteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// message_delta 事件携带顶层 usage 字段，parseClaudeStreamUsage 从中提取 token 数。
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":7,\"output_tokens\":4}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()
	var got usage.Record
	publishUsageRecord = func(_ context.Context, record usage.Record) { got = record }

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	ctx := makeGinCtxWithUA("claude-cli/2.1.62 (external, sdk-cli)", "")
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"stream":true}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	result, err := exec.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}
	// 耗尽流，确保 goroutine 中的 reporter.publish 和 ensurePublished 执行完毕。
	for range result.Chunks {
	}
	if got.Detail.InputTokens != 7 {
		t.Errorf("expected InputTokens=7, got %d — usage record not published or wrong in nativePassthrough ExecuteStream path", got.Detail.InputTokens)
	}
	if got.Detail.OutputTokens != 4 {
		t.Errorf("expected OutputTokens=4, got %d — usage record not published or wrong in nativePassthrough ExecuteStream path", got.Detail.OutputTokens)
	}
}

// TestClaudeExecutor_NativePassthrough_OAuthBeta verifies that the oauth-2025-04-20 beta flag
// is appended to Anthropic-Beta when absent, and not duplicated when already present.
func TestClaudeExecutor_NativePassthrough_OAuthBeta(t *testing.T) {
	const ua = "claude-cli/2.1.62 (external, sdk-cli)"
	const upstreamToken = "sk-ant-oat01-test"

	// Case 1: client beta does NOT contain oauth-2025-04-20 → upstream should receive it appended.
	t.Run("AppendedWhenAbsent", func(t *testing.T) {
		clientBeta := "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28"
		wantBeta := "claude-code-20250219,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28,oauth-2025-04-20"

		t.Run("Execute", func(t *testing.T) {
			var capturedBeta string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedBeta = r.Header.Get("Anthropic-Beta")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
			}))
			defer server.Close()

			exec := NewClaudeExecutor(&config.Config{})
			auth := &cliproxyauth.Auth{Attributes: map[string]string{
				"api_key":  upstreamToken,
				"base_url": server.URL,
			}}
			ctx := makeGinCtxWithUA(ua, clientBeta)
			payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
			req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

			if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if capturedBeta != wantBeta {
				t.Fatalf("Execute: expected upstream Anthropic-Beta=%q, got %q", wantBeta, capturedBeta)
			}
		})

		t.Run("ExecuteStream", func(t *testing.T) {
			var capturedBeta string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedBeta = r.Header.Get("Anthropic-Beta")
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-5\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
				_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
				_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n"))
				_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n"))
				_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			}))
			defer server.Close()

			exec := NewClaudeExecutor(&config.Config{})
			auth := &cliproxyauth.Auth{Attributes: map[string]string{
				"api_key":  upstreamToken,
				"base_url": server.URL,
			}}
			ctx, cancel := context.WithTimeout(makeGinCtxWithUA(ua, clientBeta), 5*time.Second)
			defer cancel()

			payload := []byte(`{"model":"claude-sonnet-4-5","stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
			req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

			result, err := exec.ExecuteStream(ctx, auth, req, opts)
			if err != nil {
				t.Fatalf("ExecuteStream returned error: %v", err)
			}
			for range result.Chunks {
			}
			if capturedBeta != wantBeta {
				t.Fatalf("ExecuteStream: expected upstream Anthropic-Beta=%q, got %q", wantBeta, capturedBeta)
			}
		})

		t.Run("CountTokens", func(t *testing.T) {
			var capturedBeta string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedBeta = r.Header.Get("Anthropic-Beta")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"input_tokens":10}`))
			}))
			defer server.Close()

			exec := NewClaudeExecutor(&config.Config{})
			auth := &cliproxyauth.Auth{Attributes: map[string]string{
				"api_key":  upstreamToken,
				"base_url": server.URL,
			}}
			ctx := makeGinCtxWithUA(ua, clientBeta)
			payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
			req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

			if _, err := exec.CountTokens(ctx, auth, req, opts); err != nil {
				t.Fatalf("CountTokens returned error: %v", err)
			}
			if capturedBeta != wantBeta {
				t.Fatalf("CountTokens: expected upstream Anthropic-Beta=%q, got %q", wantBeta, capturedBeta)
			}
		})
	})

	// Case 2: client beta already contains oauth-2025-04-20 → upstream receives value unchanged (no duplication).
	t.Run("NotDuplicatedWhenPresent", func(t *testing.T) {
		clientBeta := "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14"

		t.Run("Execute", func(t *testing.T) {
			var capturedBeta string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedBeta = r.Header.Get("Anthropic-Beta")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
			}))
			defer server.Close()

			exec := NewClaudeExecutor(&config.Config{})
			auth := &cliproxyauth.Auth{Attributes: map[string]string{
				"api_key":  upstreamToken,
				"base_url": server.URL,
			}}
			ctx := makeGinCtxWithUA(ua, clientBeta)
			payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
			req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

			if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if capturedBeta != clientBeta {
				t.Fatalf("Execute: expected upstream Anthropic-Beta=%q (unchanged), got %q", clientBeta, capturedBeta)
			}
		})
	})
}

// TestClaudeExecutor_OAuthTokenInConfig_UsesBearerHeader verifies that an OAuth token
// configured as api_key (e.g., in claude-api-key YAML) is sent via Authorization: Bearer
// instead of x-api-key, preventing the "OAuth authentication is currently not supported" error.
func TestClaudeExecutor_OAuthTokenInConfig_UsesBearerHeader(t *testing.T) {
	var capturedAuthHeader, capturedAPIKeyHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthHeader = r.Header.Get("Authorization")
		capturedAPIKeyHeader = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"model":"claude-sonnet-4-5","stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer server.Close()

	oauthToken := "sk-ant-oat01-testtoken"
	exec := NewClaudeExecutor(&config.Config{})
	// OAuth token stored in Attributes["api_key"] as users may do in YAML config
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  oauthToken,
		"base_url": server.URL,
	}}
	ctx := makeGinCtxWithUA("some-other-client/1.0", "")
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	_, err := exec.Execute(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if capturedAPIKeyHeader != "" {
		t.Errorf("OAuth token must not be sent via x-api-key header, got: %q", capturedAPIKeyHeader)
	}
	wantBearer := "Bearer " + oauthToken
	if capturedAuthHeader != wantBearer {
		t.Errorf("OAuth token must be sent via Authorization: Bearer, got: %q, want: %q", capturedAuthHeader, wantBearer)
	}
}

// TestClaudeExecutor_DefaultBeta verifies that when a non-claude-cli client sends no
// Anthropic-Beta header, the executor injects the expected default beta value reflecting
// the current CLI 2.1.62 OAuth direct-connect feature set.
func TestClaudeExecutor_DefaultBeta(t *testing.T) {
	var capturedBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBeta = r.Header.Get("Anthropic-Beta")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"api_key":  "sk-ant-oat01-test",
		"base_url": server.URL,
	}}
	// Non-claude-cli UA ensures nativePassthrough is NOT triggered.
	// Empty beta header means the executor must inject its own default.
	ctx := makeGinCtxWithUA("cursor/1.0", "")
	payload := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude")}

	if _, err := exec.Execute(ctx, auth, req, opts); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	wantBeta := "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05"
	if capturedBeta != wantBeta {
		t.Fatalf("expected upstream Anthropic-Beta=%q, got %q", wantBeta, capturedBeta)
	}
}
