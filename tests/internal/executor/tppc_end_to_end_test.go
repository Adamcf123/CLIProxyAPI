package executor

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// TestTppcCredentials tests that CodexExecutor can retrieve credentials from tppc configuration
func TestTppcCredentials(t *testing.T) {
	// Create a config with tppc providers
	cfg := &config.Config{
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "test-provider",
					Enabled: true,
					BaseURL: "https://test.example.com/v1",
					APIKey:  "sk-test123",
				},
				{
					Name:    "disabled-provider",
					Enabled: false,
					BaseURL: "https://disabled.example.com/v1",
					APIKey:  "sk-disabled456",
				},
			},
		},
	}

	// Create CodexExecutor with tppc config
	codexExec := executor.NewCodexExecutor(cfg)

	// Test getting credentials without auth (should fall back to tppc)
	apiKey, baseURL := codexExec.GetTestCredentials()

	if apiKey != "sk-test123" {
		t.Errorf("Expected API key from enabled tppc provider, got: %s", apiKey)
	}

	if baseURL != "https://test.example.com/v1" {
		t.Errorf("Expected base URL from enabled tppc provider, got: %s", baseURL)
	}
}

// TestTppcCredentialsPriority tests that explicit auth takes priority over tppc config
func TestTppcCredentialsPriority(t *testing.T) {
	// Create a config with tppc providers
	cfg := &config.Config{
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "tppc-provider",
					Enabled: true,
					BaseURL: "https://tppc.example.com/v1",
					APIKey:  "sk-tppc123",
				},
			},
		},
	}

	// Create CodexExecutor with tppc config
	codexExec := executor.NewCodexExecutor(cfg)

	// Create auth with explicit credentials (should take priority)
	auth := &cliproxyauth.Auth{
		Attributes: map[string]string{
			"api_key":  "sk-explicit123",
			"base_url": "https://explicit.example.com/v1",
		},
	}

	// Test getting credentials with auth (should use explicit auth, not tppc)
	apiKey, baseURL := codexExec.GetTestCredentialsWithAuth(auth)

	if apiKey != "sk-explicit123" {
		t.Errorf("Expected API key from explicit auth, got: %s", apiKey)
	}

	if baseURL != "https://explicit.example.com/v1" {
		t.Errorf("Expected base URL from explicit auth, got: %s", baseURL)
	}
}

// TestTppcNoEnabledProviders tests behavior when no tppc providers are enabled
func TestTppcNoEnabledProviders(t *testing.T) {
	// Create a config with no enabled tppc providers
	cfg := &config.Config{
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "disabled-provider",
					Enabled: false,
					BaseURL: "https://disabled.example.com/v1",
					APIKey:  "sk-disabled456",
				},
			},
		},
	}

	// Create CodexExecutor with tppc config
	codexExec := executor.NewCodexExecutor(cfg)

	// Test getting credentials without auth and no enabled providers
	apiKey, baseURL := codexExec.GetTestCredentials()

	if apiKey != "" {
		t.Errorf("Expected empty API key when no providers enabled, got: %s", apiKey)
	}

	if baseURL != "" {
		t.Errorf("Expected empty base URL when no providers enabled, got: %s", baseURL)
	}
}

// TestTppcMultipleProviders tests that first enabled provider is used
func TestTppcMultipleProviders(t *testing.T) {
	// Create a config with multiple enabled tppc providers
	cfg := &config.Config{
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "first-provider",
					Enabled: true,
					BaseURL: "https://first.example.com/v1",
					APIKey:  "sk-first123",
				},
				{
					Name:    "second-provider",
					Enabled: true,
					BaseURL: "https://second.example.com/v1",
					APIKey:  "sk-second456",
				},
			},
		},
	}

	// Create CodexExecutor with tppc config
	codexExec := executor.NewCodexExecutor(cfg)

	// Test getting credentials without auth (should use first enabled provider)
	apiKey, baseURL := codexExec.GetTestCredentials()

	if apiKey != "sk-first123" {
		t.Errorf("Expected API key from first enabled provider, got: %s", apiKey)
	}

	if baseURL != "https://first.example.com/v1" {
		t.Errorf("Expected base URL from first enabled provider, got: %s", baseURL)
	}
}
