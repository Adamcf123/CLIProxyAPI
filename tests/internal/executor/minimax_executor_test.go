package executor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	sdkexec "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

// When claude-api-key base_url points to a MiniMax Anthropic-compatible endpoint,
// MiniMaxExecutor should synthesize a runtime auth from config and route via Claude-compatible path.
func TestMiniMaxExecutor_FallbackToClaudeAnthropic_NonStream(t *testing.T) {
	var gotPath, gotAuth, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		// Minimal Claude messages style JSON (enough for translator path)
		_, _ = w.Write([]byte(`{"id":"m","content":[{"text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{}
	cfg.ClaudeKey = []config.ClaudeKey{{
		APIKey:  "tok",
		BaseURL: srv.URL, // treated as Anthropic-compatible endpoint in test
	}}
	exec := executor.NewMiniMaxExecutor(cfg)
	ctx := context.Background()
	// Incoming auth can be minimal; fallback uses cfg.ClaudeKey
	auth := &coreauth.Auth{Attributes: map[string]string{"api_key": "unused"}}
	req := sdkexec.Request{Model: "MiniMax-M2", Payload: []byte(`{"messages":[{"role":"user","content":"hi"}]}`)}
	opts := sdkexec.Options{SourceFormat: sdktranslator.FromString("openai")}
	resp, err := exec.Execute(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Payload) == 0 {
		t.Fatalf("expected response payload")
	}
	if gotPath != "/v1/messages" && gotPath != "/v1/messages"+"?beta=true" {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("unexpected Authorization: %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("unexpected Content-Type: %q", gotCT)
	}
}
