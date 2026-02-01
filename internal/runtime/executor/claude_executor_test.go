package executor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

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

func TestEnsureKimiToolCallReasoningContent(t *testing.T) {
	t.Run("kimi-for-coding adds empty reasoning_content for tool_use when thinking enabled", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		msg := gjson.GetBytes(out, "messages.0")
		if !msg.Get("reasoning_content").Exists() {
			t.Fatalf("expected reasoning_content to be set")
		}
		if got := msg.Get("reasoning_content").String(); got != "." {
			t.Fatalf("reasoning_content=%q, want %q", got, ".")
		}
	})

	t.Run("kimi-for-coding overwrites empty/whitespace reasoning_content", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","reasoning_content":"   ","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if got := gjson.GetBytes(out, "messages.0.reasoning_content").String(); got != "." {
			t.Fatalf("reasoning_content=%q, want %q", got, ".")
		}
	})

	t.Run("kimi-for-coding overwrites null reasoning_content", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","reasoning_content":null,"content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if got := gjson.GetBytes(out, "messages.0.reasoning_content").String(); got != "." {
			t.Fatalf("reasoning_content=%q, want %q", got, ".")
		}
	})

	t.Run("kimi-for-coding does not override existing reasoning_content", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","reasoning_content":"keep","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if got := gjson.GetBytes(out, "messages.0.reasoning_content").String(); got != "keep" {
			t.Fatalf("reasoning_content=%q, want %q", got, "keep")
		}
	})

	t.Run("non-kimi model is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"claude-3-5-sonnet-20241022","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("claude-3-5-sonnet-20241022", input)
		if gjson.GetBytes(out, "messages.0.reasoning_content").Exists() {
			t.Fatalf("expected reasoning_content to remain absent for non-kimi model")
		}
	})

	t.Run("kimi-for-coding without thinking enabled is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if gjson.GetBytes(out, "messages.0.reasoning_content").Exists() {
			t.Fatalf("expected reasoning_content to remain absent when thinking is not enabled")
		}
	})

	t.Run("kimi-for-coding assistant text message is unchanged", func(t *testing.T) {
		input := []byte(`{"model":"kimi-for-coding","thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"assistant","content":[{"type":"text","text":"hi"}]}]}`)
		out := ensureKimiToolCallReasoningContent("kimi-for-coding", input)
		if gjson.GetBytes(out, "messages.0.reasoning_content").Exists() {
			t.Fatalf("expected reasoning_content to remain absent for non tool_use assistant message")
		}
	})
}

func TestClaudeExecutor_ExecuteStream_KimiToolUseReasoningContentPatched(t *testing.T) {
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

		rc := gjson.GetBytes(b, "messages.1.reasoning_content")
		if !rc.Exists() || strings.TrimSpace(rc.String()) == "" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("thinking is enabled but reasoning_content is missing in assistant tool call message at index 2"))
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {}\n"))
	}))
	defer server.Close()

	exec := NewClaudeExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{"api_key": "test", "base_url": server.URL}}
	payload := []byte(`{"model":"kimi-for-coding","max_tokens":1,"stream":true,"thinking":{"type":"enabled","budget_tokens":10},"tools":[{"name":"my_tool","input_schema":{"type":"object"}}],"tool_choice":{"type":"auto"},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_tool","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`)
	req := cliproxyexecutor.Request{Model: "kimi-for-coding", Payload: payload}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude"), OriginalRequest: payload}

	stream, err := exec.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}

	select {
	case <-stream:
	case <-ctx.Done():
		t.Fatalf("timeout waiting for stream")
	}
	if len(captured) == 0 {
		t.Fatalf("expected upstream request to be captured")
	}
	if got := strings.TrimSpace(gjson.GetBytes(captured, "messages.1.reasoning_content").String()); got != "." {
		t.Fatalf("upstream reasoning_content=%q, want %q", got, ".")
	}
}

func TestClaudeExecutor_Execute_KimiToolUseReasoningContentPatched(t *testing.T) {
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

		rc := gjson.GetBytes(b, "messages.1.reasoning_content")
		if !rc.Exists() || strings.TrimSpace(rc.String()) == "" {
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
	if got := strings.TrimSpace(gjson.GetBytes(captured, "messages.1.reasoning_content").String()); got != "." {
		t.Fatalf("upstream reasoning_content=%q, want %q", got, ".")
	}
}
