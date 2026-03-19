package executor

import (
	"testing"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestMaybeBuildMiniMaxAnthropicCodexResponseRequest_BuildsAnthropicMessagesShape(t *testing.T) {
	input := []byte(`{
		"model":"gpt-5.4",
		"instructions":"You are helpful.",
		"max_output_tokens":256,
		"stream":false,
		"reasoning":{"effort":"medium"},
		"input":[
			{"role":"system","content":[{"type":"input_text","text":"Follow repo rules."}]},
			{"role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"}
		],
		"tools":[
			{"type":"function","name":"exec_command","description":"Run command","parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}},
			{"type":"custom","name":"apply_patch"}
		],
		"tool_choice":"required"
	}`)

	out, ok := maybeBuildMiniMaxAnthropicCodexResponseRequest(
		"https://api.minimaxi.com/anthropic",
		sdktranslator.FormatOpenAIResponse,
		"MiniMax-M2.7-highspeed",
		input,
		false,
	)
	if !ok {
		t.Fatalf("expected minimax anthropic request builder to activate")
	}

	if got := gjson.GetBytes(out, "model").String(); got != "MiniMax-M2.7-highspeed" {
		t.Fatalf("model = %q, want %q", got, "MiniMax-M2.7-highspeed")
	}
	if got := gjson.GetBytes(out, "system").String(); got != "You are helpful.\n\nFollow repo rules." {
		t.Fatalf("system = %q", got)
	}
	if got := gjson.GetBytes(out, "messages.0.role").String(); got != "user" {
		t.Fatalf("messages.0.role = %q, want %q", got, "user")
	}
	if got := gjson.GetBytes(out, "messages.0.content.0.type").String(); got != "text" {
		t.Fatalf("messages.0.content.0.type = %q, want %q", got, "text")
	}
	if got := gjson.GetBytes(out, "messages.1.content.0.type").String(); got != "tool_use" {
		t.Fatalf("messages.1.content.0.type = %q, want %q", got, "tool_use")
	}
	if got := gjson.GetBytes(out, "messages.2.content.0.type").String(); got != "tool_result" {
		t.Fatalf("messages.2.content.0.type = %q, want %q", got, "tool_result")
	}
	if got := gjson.GetBytes(out, "tools.#").Int(); got != 1 {
		t.Fatalf("tools count = %d, want 1: %s", got, gjson.GetBytes(out, "tools").Raw)
	}
	if got := gjson.GetBytes(out, "tools.0.input_schema.type").String(); got != "object" {
		t.Fatalf("tools.0.input_schema.type = %q, want %q", got, "object")
	}
	if got := gjson.GetBytes(out, "tool_choice.type").String(); got != "any" {
		t.Fatalf("tool_choice.type = %q, want %q", got, "any")
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
		t.Fatalf("thinking.type = %q, want %q", got, "adaptive")
	}
	if got := gjson.GetBytes(out, "output_config.effort").String(); got != "high" {
		t.Fatalf("output_config.effort = %q, want %q", got, "high")
	}
}

func TestMaybeBuildMiniMaxAnthropicCodexResponseRequest_SkipsNonMiniMaxOrNonResponses(t *testing.T) {
	input := []byte(`{"input":[]}`)

	if _, ok := maybeBuildMiniMaxAnthropicCodexResponseRequest(
		"https://api.minimaxi.com/anthropic",
		sdktranslator.FormatOpenAI,
		"MiniMax-M2.7-highspeed",
		input,
		false,
	); ok {
		t.Fatal("expected non-responses source format to skip builder")
	}

	if _, ok := maybeBuildMiniMaxAnthropicCodexResponseRequest(
		"https://api.anthropic.com",
		sdktranslator.FormatOpenAIResponse,
		"MiniMax-M2.7-highspeed",
		input,
		false,
	); ok {
		t.Fatal("expected non-minimax base url to skip builder")
	}
}
