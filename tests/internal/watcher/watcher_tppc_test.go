package watcher_test

import (
	"path/filepath"
	"testing"

	appconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/watcher"
)

func TestSnapshotCoreAuths_TppcProviders(t *testing.T) {
	cfg := &appconfig.Config{
		Tppc: appconfig.TppcConfig{
			Providers: []appconfig.TppcProvider{
				{
					Name:    "fox",
					Enabled: true,
					BaseURL: "https://fox.example.com/v1",
					APIKey:  "sk-fox-123",
				},
				{
					Name:    "wolf",
					Enabled: false,
					BaseURL: "https://wolf.example.com/v1",
					APIKey:  "sk-wolf-456",
				},
			},
		},
	}

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	authDir := t.TempDir()
	w, err := watcher.NewWatcher(configPath, authDir, nil)
	if err != nil {
		t.Fatalf("NewWatcher error: %v", err)
	}
	w.SetConfig(cfg)

	auths := w.SnapshotCoreAuths()
	if len(auths) != 1 {
		t.Fatalf("expected exactly one synthesized auth, got %d", len(auths))
	}

	var foxAuthFound bool
	for _, a := range auths {
		if a == nil || a.Provider != "fox" {
			continue
		}
		foxAuthFound = true
		if a.Label != "fox" {
			t.Errorf("expected label=fox, got %q", a.Label)
		}
		if a.Attributes == nil {
			t.Fatalf("expected attributes for fox provider auth")
		}
		if got := a.Attributes["base_url"]; got != "https://fox.example.com/v1" {
			t.Errorf("expected base_url=https://fox.example.com/v1, got %q", got)
		}
		if got := a.Attributes["api_key"]; got != "sk-fox-123" {
			t.Errorf("expected api_key=sk-fox-123, got %q", got)
		}
		if got := a.Attributes["source"]; got != "config:tppc[fox]" {
			t.Errorf("expected source=config:tppc[fox], got %q", got)
		}
	}
	if !foxAuthFound {
		t.Fatalf("expected synthesized fox provider auth for enabled tppc provider")
	}
}
