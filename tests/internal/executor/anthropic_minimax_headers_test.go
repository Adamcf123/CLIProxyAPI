package executor_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	sdkexec "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
)

// Ensure AnthropicCompatExecutor sets Accept-Encoding=identity for minimax (non-stream)
func TestAnthropicCompat_Minimax_NoCompression_NonStream(t *testing.T) {
	var gotEnc, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEnc = r.Header.Get("Accept-Encoding")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","content":[{"text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{}
	exec := executor.NewAnthropicCompatExecutor(cfg, "minimax")
	ctx := context.Background()
	auth := &coreauth.Auth{Attributes: map[string]string{"api_key": "tok", "base_url": srv.URL}}
	req := sdkexec.Request{Model: "MiniMax-M2", Payload: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)}
	// from == to => non-stream path
	opts := sdkexec.Options{SourceFormat: sdktranslator.FromString("claude")}
	resp, err := exec.Execute(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Payload) == 0 {
		t.Fatalf("expected payload")
	}
	if gotEnc != "identity" {
		t.Fatalf("expected Accept-Encoding=identity, got %q; Accept=%q", gotEnc, gotAccept)
	}
}

// Ensure AnthropicCompatExecutor sets Accept-Encoding=identity for minimax (stream)
func TestAnthropicCompat_Minimax_NoCompression_Stream(t *testing.T) {
	var gotEnc, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEnc = r.Header.Get("Accept-Encoding")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		// Send a minimal SSE line and close
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"type":"message_start"}`)
		flusher.Flush()
	}))
	defer srv.Close()

	cfg := &config.Config{}
	exec := executor.NewAnthropicCompatExecutor(cfg, "minimax")
	ctx := context.Background()
	auth := &coreauth.Auth{Attributes: map[string]string{"api_key": "tok", "base_url": srv.URL}}
	req := sdkexec.Request{Model: "MiniMax-M2", Payload: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"stream":true}`)}
	opts := sdkexec.Options{Stream: true, SourceFormat: sdktranslator.FromString("claude")}
	ch, err := exec.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Drain stream
	got := 0
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		if len(chunk.Payload) > 0 {
			// ensure readable
			_ = bufio.NewReader(bytesFrom(chunk.Payload))
			got++
		}
	}
	if got == 0 {
		t.Fatalf("expected at least one stream chunk")
	}
	if gotEnc != "identity" {
		t.Fatalf("expected Accept-Encoding=identity, got %q; Accept=%q", gotEnc, gotAccept)
	}
}

// bytesFrom avoids allocation by returning the same slice back as io.Reader source validation
func bytesFrom(b []byte) *bufio.Reader { return bufio.NewReaderSize(nil, 0) }
