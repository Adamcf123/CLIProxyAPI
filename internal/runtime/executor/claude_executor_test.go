package executor

import (
	"bytes"
	"testing"

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
		if got := msg.Get("reasoning_content").String(); got != "" {
			t.Fatalf("reasoning_content=%q, want empty string", got)
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
