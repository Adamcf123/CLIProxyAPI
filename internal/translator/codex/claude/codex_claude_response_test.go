package claude

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertCodexResponseToClaudeFiltersBashOutputStream(t *testing.T) {
	originalRequest := []byte(`{"tools":[{"name":"BashOutput"}]}`)
	var state any

	// Simulate Codex streaming events for a BashOutput function call.
	added := []byte(`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"BashOutput","arguments":"{}"}}`)
	if out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, added, &state); len(out) != 0 {
		t.Fatalf("expected no output for BashOutput function_call, got %v", out)
	}

	delta := []byte(`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\\"bash_id\\":\\"chatcmpl-123\\"}"}`)
	if out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, delta, &state); len(out) != 0 {
		t.Fatalf("expected no output for BashOutput arguments delta, got %v", out)
	}

	done := []byte(`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_1"}}`)
	if out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, done, &state); len(out) != 0 {
		t.Fatalf("expected no output for BashOutput completion, got %v", out)
	}

	completed := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5-codex","usage":{"input_tokens":1,"output_tokens":2}}}`)
	out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, completed, &state)
	if len(out) != 1 {
		t.Fatalf("expected single completion event, got %d", len(out))
	}
	if !strings.Contains(out[0], `"stop_reason":"end_turn"`) {
		t.Fatalf("expected stop_reason end_turn, got %q", out[0])
	}
}

func TestConvertCodexResponseToClaudeNonStreamFiltersBashOutput(t *testing.T) {
	originalRequest := []byte(`{"tools":[{"name":"BashOutput"}]}`)
	raw := []byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5-codex","usage":{"input_tokens":1,"output_tokens":2},"output":[{"type":"function_call","call_id":"call_1","name":"BashOutput","arguments":"{}"},{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}}`)

	resultJSON := ConvertCodexResponseToClaudeNonStream(context.Background(), "", originalRequest, nil, raw, nil)
	if resultJSON == "" {
		t.Fatalf("expected non-empty response JSON")
	}

	var msg struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &msg); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			t.Fatalf("unexpected tool_use block for BashOutput")
		}
	}
}

func TestMixedToolsStreamKeepValidToolUseAndStopReason(t *testing.T) {
	// original tools include both a valid tool and BashOutput
	originalRequest := []byte(`{"tools":[{"name":"ValidTool"},{"name":"BashOutput"}]}`)
	var state any

	// 1) valid tool function_call added
	addedValid := []byte(`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_valid","name":"ValidTool","arguments":"{\"path\":\"/tmp\"}"}}`)
	out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, addedValid, &state)
	if len(out) == 0 || !strings.Contains(out[0], "\"content_block\":{\"type\":\"tool_use\"") {
		t.Fatalf("expected tool_use start for ValidTool, got %v", out)
	}

	// 2) valid tool arguments delta
	deltaValid := []byte(`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"more\":true}"}`)
	if out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, deltaValid, &state); len(out) == 0 {
		t.Fatalf("expected delta output for ValidTool")
	}

	// 3) insert BashOutput which should be filtered entirely
	addedBash := []byte(`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","call_id":"call_bash","name":"BashOutput","arguments":"{}"}}`)
	if out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, addedBash, &state); len(out) != 0 {
		t.Fatalf("expected no output for BashOutput added, got %v", out)
	}

	// 4) complete valid tool
	doneValid := []byte(`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_valid"}}`)
	if out := ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, doneValid, &state); len(out) == 0 {
		t.Fatalf("expected stop event for ValidTool")
	}

	// 5) response completed should indicate tool_use because a valid tool existed
	completed := []byte(`data: {"type":"response.completed","response":{"id":"resp_2","model":"gpt-5-codex","usage":{"input_tokens":1,"output_tokens":2}}}`)
	out = ConvertCodexResponseToClaude(context.Background(), "", originalRequest, nil, completed, &state)
	if len(out) != 1 || !strings.Contains(out[0], "\"stop_reason\":\"tool_use\"") {
		t.Fatalf("expected stop_reason tool_use, got %v", out)
	}
}
