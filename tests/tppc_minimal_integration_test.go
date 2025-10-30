package tests

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// TestTppcMinimalIntegration验证tppc功能的最小集成测试
func TestTppcMinimalIntegration(t *testing.T) {
	// 创建配置
	cfg := &config.Config{
		Port: 8317,
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "test-provider",
					Enabled: true,
					BaseURL: "https://test.example.com/v1",
					APIKey:  "sk-test123",
				},
			},
		},
	}

	// 验证配置
	if len(cfg.Tppc.Providers) != 1 {
		t.Fatalf("Expected 1 provider, got %d", len(cfg.Tppc.Providers))
	}

	provider := cfg.Tppc.Providers[0]
	if provider.Name != "test-provider" {
		t.Errorf("Expected provider name 'test-provider', got '%s'", provider.Name)
	}
	if !provider.Enabled {
		t.Error("Expected provider to be enabled")
	}
	if provider.BaseURL != "https://test.example.com/v1" {
		t.Errorf("Expected base-url 'https://test.example.com/v1', got '%s'", provider.BaseURL)
	}
	if provider.APIKey != "sk-test123" {
		t.Errorf("Expected api-key 'sk-test123', got '%s'", provider.APIKey)
	}

	// 验证配置有效性
	if err := config.ValidateTppc(cfg); err != nil {
		t.Errorf("Expected validation to pass, got error: %v", err)
	}

	t.Log("✅ tppc minimal integration test passed")
}

// TestTppcBackwardCompatibility验证tppc与packycode的向后兼容性
func TestTppcBackwardCompatibility(t *testing.T) {
	// 创建同时包含packycode和tppc的配置
	cfg := &config.Config{
		Port: 8317,
		Packycode: config.PackycodeConfig{
			Enabled: false,
			BaseURL: "https://codex-api.packycode.com/v1",
			Credentials: config.PackycodeCredentials{
				OpenAIAPIKey: "sk-packycode123",
			},
		},
		Tppc: config.TppcConfig{
			Providers: []config.TppcProvider{
				{
					Name:    "tppc-provider",
					Enabled: true,
					BaseURL: "https://tppc.example.com/v1",
					APIKey:  "sk-tppc456",
				},
			},
		},
	}

	// 验证两种配置都能独立工作
	if cfg.Packycode.Enabled {
		t.Error("Expected packycode to be disabled")
	}

	if len(cfg.Tppc.Providers) != 1 {
		t.Fatalf("Expected 1 tppc provider, got %d", len(cfg.Tppc.Providers))
	}

	if !cfg.Tppc.Providers[0].Enabled {
		t.Error("Expected tppc provider to be enabled")
	}

	// 验证tppc配置有效性
	if err := config.ValidateTppc(cfg); err != nil {
		t.Errorf("Expected tppc validation to pass, got error: %v", err)
	}

	t.Log("✅ tppc backward compatibility test passed")
}
