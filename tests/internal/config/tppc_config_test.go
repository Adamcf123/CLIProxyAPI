package config_test

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestLoadConfigWithTppc(t *testing.T) {
	// Test basic tppc configuration loading
	configContent := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: true
      base-url: "https://codex-api.packycode.com/v1"
      api-key: "sk-test123"
    - name: "custom-provider"
      enabled: false
      base-url: "https://custom.example.com/v1"
      api-key: "sk-custom456"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify tppc configuration
	if len(cfg.Tppc.Providers) != 2 {
		t.Fatalf("Expected 2 providers, got %d", len(cfg.Tppc.Providers))
	}

	// Check first provider (packycode)
	provider1 := cfg.Tppc.Providers[0]
	if provider1.Name != "packycode" {
		t.Errorf("Expected provider1 name 'packycode', got '%s'", provider1.Name)
	}
	if !provider1.Enabled {
		t.Errorf("Expected provider1 enabled=true")
	}
	if provider1.BaseURL != "https://codex-api.packycode.com/v1" {
		t.Errorf("Expected provider1 base-url 'https://codex-api.packycode.com/v1', got '%s'", provider1.BaseURL)
	}
	if provider1.APIKey != "sk-test123" {
		t.Errorf("Expected provider1 api-key 'sk-test123', got '%s'", provider1.APIKey)
	}

	// Check second provider (custom-provider)
	provider2 := cfg.Tppc.Providers[1]
	if provider2.Name != "custom-provider" {
		t.Errorf("Expected provider2 name 'custom-provider', got '%s'", provider2.Name)
	}
	if provider2.Enabled {
		t.Errorf("Expected provider2 enabled=false")
	}
	if provider2.BaseURL != "https://custom.example.com/v1" {
		t.Errorf("Expected provider2 base-url 'https://custom.example.com/v1', got '%s'", provider2.BaseURL)
	}
	if provider2.APIKey != "sk-custom456" {
		t.Errorf("Expected provider2 api-key 'sk-custom456', got '%s'", provider2.APIKey)
	}
}

func TestLoadConfigWithoutTppc(t *testing.T) {
	// Test config without tppc section
	configContent := `
port: 8317
debug: false
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify tppc is empty by default
	if len(cfg.Tppc.Providers) != 0 {
		t.Fatalf("Expected 0 providers, got %d", len(cfg.Tppc.Providers))
	}
}

func TestLoadConfigWithEmptyTppc(t *testing.T) {
	// Test config with empty tppc section
	configContent := `
port: 8317
tppc:
  providers: []
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify tppc is empty
	if len(cfg.Tppc.Providers) != 0 {
		t.Fatalf("Expected 0 providers, got %d", len(cfg.Tppc.Providers))
	}
}

func TestValidateTppcEnabledProvider(t *testing.T) {
	// Test validation of enabled provider with required fields
	configContent := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: true
      base-url: "https://codex-api.packycode.com/v1"
      api-key: "sk-test123"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validation should pass for enabled provider with required fields
	err = appconfig.ValidateTppc(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for enabled provider with required fields, got error: %v", err)
	}
}

func TestValidateTppcMissingBaseUrl(t *testing.T) {
	// Test validation failure when enabled provider missing base-url
	configContent := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: true
      api-key: "sk-test123"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfigOptional(configPath, true)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validation should fail for enabled provider without base-url
	err = appconfig.ValidateTppc(cfg)
	if err == nil {
		t.Error("Expected validation to fail for enabled provider missing base-url")
	}
}

func TestValidateTppcMissingApiKey(t *testing.T) {
	// Test validation failure when enabled provider missing api-key
	configContent := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: true
      base-url: "https://codex-api.packycode.com/v1"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfigOptional(configPath, true)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validation should fail for enabled provider without api-key
	err = appconfig.ValidateTppc(cfg)
	if err == nil {
		t.Error("Expected validation to fail for enabled provider missing api-key")
	}
}

func TestValidateTppcDisabledProvider(t *testing.T) {
	// Test validation passes for disabled provider (even without required fields)
	configContent := `
port: 8317
tppc:
  providers:
    - name: "incomplete-provider"
      enabled: false
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validation should pass for disabled provider (no required fields)
	err = appconfig.ValidateTppc(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for disabled provider, got error: %v", err)
	}
}

func TestValidateTppcInvalidBaseUrl(t *testing.T) {
	// Test validation failure for invalid base-url
	configContent := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: true
      base-url: "invalid-url"
      api-key: "sk-test123"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfigOptional(configPath, true)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validation should fail for invalid base-url
	err = appconfig.ValidateTppc(cfg)
	if err == nil {
		t.Error("Expected validation to fail for invalid base-url")
	}
}
