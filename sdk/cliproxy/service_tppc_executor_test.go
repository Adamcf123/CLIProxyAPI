package cliproxy

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v6/sdk/auth"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestEnsureExecutorsForAuth_TppcProvider(t *testing.T) {
	cfg := &config.Config{
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "fox",
					Enabled: true,
					BaseURL: "https://fox.example.com/v1",
					APIKey:  "sk-fox-123",
				},
			},
		},
	}

	s := &Service{cfg: cfg}
	s.coreManager = coreauth.NewManager(sdkAuth.GetTokenStore(), nil, nil)

	auth := &coreauth.Auth{
		ID:       "auth-fox",
		Provider: "fox",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"source":   "config:tppc[fox]",
			"base_url": "https://fox.example.com/v1",
			"api_key":  "sk-fox-123",
		},
	}

	// ensureExecutorsForAuth should register a Codex executor keyed by provider name.
	s.ensureExecutorsForAuth(auth)

	keys := registeredExecutorKeys(t, s.coreManager)
	if len(keys) != 1 || keys[0] != "fox" {
		t.Fatalf("expected executor keyed by provider 'fox', got %v", keys)
	}

	// Register auth and ensure core manager accepts it without errors.
	if _, err := s.coreManager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth failed: %v", err)
	}
}

func TestRegisterModelsForAuth_TppcProvider(t *testing.T) {
	cfg := &config.Config{
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "fox",
					Enabled: true,
					BaseURL: "https://fox.example.com/v1",
					APIKey:  "sk-fox-123",
				},
			},
		},
	}

	s := &Service{cfg: cfg}
	s.coreManager = coreauth.NewManager(sdkAuth.GetTokenStore(), nil, nil)

	auth := &coreauth.Auth{
		ID:       "auth-fox",
		Provider: "fox",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"source":   "config:tppc[fox]",
			"base_url": "https://fox.example.com/v1",
			"api_key":  "sk-fox-123",
		},
	}

	reg := registry.GetGlobalRegistry()
	reg.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		reg.UnregisterClient(auth.ID)
	})

	s.registerModelsForAuth(auth)

	providers := reg.GetModelProviders("gpt-5-minimal")
	if !containsIgnoreCase(providers, "fox") {
		t.Fatalf("expected provider list to contain fox, got %v", providers)
	}
}

func containsIgnoreCase(values []string, target string) bool {
	targetLower := strings.ToLower(target)
	for _, v := range values {
		if strings.EqualFold(v, targetLower) {
			return true
		}
	}
	return false
}

func registeredExecutorKeys(t *testing.T, m *coreauth.Manager) []string {
	t.Helper()
	if m == nil {
		return nil
	}
	val := reflect.ValueOf(m).Elem().FieldByName("executors")
	if !val.IsValid() || val.IsNil() {
		return nil
	}
	keys := val.MapKeys()
	result := make([]string, len(keys))
	for i, k := range keys {
		result[i] = k.String()
	}
	return result
}
