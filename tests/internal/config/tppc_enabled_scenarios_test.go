package config_test

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// TestTppcEnabledScenarios验证tppc启用场景的配置行为
func TestTppcEnabledScenarios(t *testing.T) {
	// 测试场景1: 启用packycode provider
	configContent1 := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: true
      base-url: "https://codex-api-slb.packycode.com/v1"
      api-key: "sk-Kvn0G6NoifRHVgCXYbUfg8psuhK3q7sS"
`

	tmpDir := t.TempDir()
	configPath1 := filepath.Join(tmpDir, "config-enabled.yaml")
	if err := os.WriteFile(configPath1, []byte(configContent1), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg1, err := appconfig.LoadConfig(configPath1)
	if err != nil {
		t.Fatalf("Failed to load enabled config: %v", err)
	}

	// 验证启用状态
	if len(cfg1.Tppc.Providers) != 1 {
		t.Fatalf("Expected 1 provider, got %d", len(cfg1.Tppc.Providers))
	}
	if !cfg1.Tppc.Providers[0].Enabled {
		t.Error("Expected packycode provider to be enabled")
	}

	// 测试场景2: 多提供商配置
	configContent2 := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: true
      base-url: "https://codex-api-slb.packycode.com/v1"
      api-key: "sk-packycode123"
    - name: "custom-provider"
      enabled: false
      base-url: "https://custom.example.com/v1"
      api-key: "sk-custom456"
`

	configPath2 := filepath.Join(tmpDir, "config-multi.yaml")
	if err := os.WriteFile(configPath2, []byte(configContent2), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg2, err := appconfig.LoadConfig(configPath2)
	if err != nil {
		t.Fatalf("Failed to load multi-provider config: %v", err)
	}

	// 验证多提供商
	if len(cfg2.Tppc.Providers) != 2 {
		t.Fatalf("Expected 2 providers, got %d", len(cfg2.Tppc.Providers))
	}
	if !cfg2.Tppc.Providers[0].Enabled {
		t.Error("Expected packycode provider to be enabled")
	}
	if cfg2.Tppc.Providers[1].Enabled {
		t.Error("Expected custom-provider to be disabled")
	}

	// 验证验证函数
	if err := appconfig.ValidateTppc(cfg1); err != nil {
		t.Errorf("Expected validation to pass for enabled provider, got error: %v", err)
	}
	if err := appconfig.ValidateTppc(cfg2); err != nil {
		t.Errorf("Expected validation to pass for multi-provider config, got error: %v", err)
	}
}

// TestTppcDisabledScenarios验证tppc禁用场景
func TestTppcDisabledScenarios(t *testing.T) {
	configContent := `
port: 8317
tppc:
  providers:
    - name: "packycode"
      enabled: false
      base-url: "https://codex-api-slb.packycode.com/v1"
      api-key: "sk-packycode123"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config-disabled.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := appconfig.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load disabled config: %v", err)
	}

	// 验证禁用状态
	if len(cfg.Tppc.Providers) != 1 {
		t.Fatalf("Expected 1 provider, got %d", len(cfg.Tppc.Providers))
	}
	if cfg.Tppc.Providers[0].Enabled {
		t.Error("Expected packycode provider to be disabled")
	}

	// 验证验证函数对禁用提供商的处理
	if err := appconfig.ValidateTppc(cfg); err != nil {
		t.Errorf("Expected validation to pass for disabled provider, got error: %v", err)
	}
}
