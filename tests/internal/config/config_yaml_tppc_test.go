package config_test

import (
	"path/filepath"
	"testing"

	appconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// TestConfigYamlTppc验证实际config.yaml文件中的tppc配置
func TestConfigYamlTppc(t *testing.T) {
	configPath := filepath.Join("../../../", "config.yaml")

	cfg, err := appconfig.LoadConfigOptional(configPath, true)
	if err != nil {
		t.Fatalf("Failed to load config.yaml: %v", err)
	}

	// 验证tppc配置存在
	if len(cfg.Tppc.Providers) == 0 {
		t.Error("Expected tppc providers in config.yaml, got none")
	}

	// 验证packycode provider配置
	packycodeProvider := findProvider(cfg.Tppc.Providers, "packycode")
	if packycodeProvider == nil {
		t.Error("Expected packycode provider in config.yaml")
	} else {
		// 验证字段值
		if packycodeProvider.Name != "packycode" {
			t.Errorf("Expected provider name 'packycode', got '%s'", packycodeProvider.Name)
		}
		if packycodeProvider.Enabled {
			t.Error("Expected packycode provider to be disabled by default")
		}
		if packycodeProvider.BaseURL != "https://codex-api-slb.packycode.com/v1" {
			t.Errorf("Expected base-url 'https://codex-api-slb.packycode.com/v1', got '%s'", packycodeProvider.BaseURL)
		}
		if packycodeProvider.APIKey == "" {
			t.Error("Expected non-empty api-key for packycode provider")
		}
	}

	// 验证与packycode配置的兼容性
	if cfg.Packycode.Enabled {
		t.Error("Expected packycode.enabled=false in config.yaml")
	}
}

// findProvider帮助函数
func findProvider(providers []appconfig.TppcProvider, name string) *appconfig.TppcProvider {
	for i := range providers {
		if providers[i].Name == name {
			return &providers[i]
		}
	}
	return nil
}
