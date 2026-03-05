package claude

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertClaudeRequestToCodex_SystemMessageScenarios(t *testing.T) {
	tests := []struct {
		name             string
		inputJSON        string
		wantHasDeveloper bool
		wantTexts        []string
	}{
		{
			name: "No system field",
			inputJSON: `{
				"model": "claude-3-opus",
				"messages": [{"role": "user", "content": "hello"}]
			}`,
			wantHasDeveloper: false,
		},
		{
			name: "Empty string system field",
			inputJSON: `{
				"model": "claude-3-opus",
				"system": "",
				"messages": [{"role": "user", "content": "hello"}]
			}`,
			wantHasDeveloper: false,
		},
		{
			name: "String system field",
			inputJSON: `{
				"model": "claude-3-opus",
				"system": "Be helpful",
				"messages": [{"role": "user", "content": "hello"}]
			}`,
			wantHasDeveloper: true,
			wantTexts:        []string{"Be helpful"},
		},
		{
			name: "Array system field with filtered billing header",
			inputJSON: `{
				"model": "claude-3-opus",
				"system": [
					{"type": "text", "text": "x-anthropic-billing-header: tenant-123"},
					{"type": "text", "text": "Block 1"},
					{"type": "text", "text": "Block 2"}
				],
				"messages": [{"role": "user", "content": "hello"}]
			}`,
			wantHasDeveloper: true,
			wantTexts:        []string{"Block 1", "Block 2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertClaudeRequestToCodex("test-model", []byte(tt.inputJSON), false)
			resultJSON := gjson.ParseBytes(result)
			inputs := resultJSON.Get("input").Array()

			hasDeveloper := len(inputs) > 0 && inputs[0].Get("role").String() == "developer"
			if hasDeveloper != tt.wantHasDeveloper {
				t.Fatalf("got hasDeveloper = %v, want %v. Output: %s", hasDeveloper, tt.wantHasDeveloper, resultJSON.Get("input").Raw)
			}

			if !tt.wantHasDeveloper {
				return
			}

			content := inputs[0].Get("content").Array()
			if len(content) != len(tt.wantTexts) {
				t.Fatalf("got %d system content items, want %d. Content: %s", len(content), len(tt.wantTexts), inputs[0].Get("content").Raw)
			}

			for i, wantText := range tt.wantTexts {
				if gotType := content[i].Get("type").String(); gotType != "input_text" {
					t.Fatalf("content[%d] type = %q, want %q", i, gotType, "input_text")
				}
				if gotText := content[i].Get("text").String(); gotText != wantText {
					t.Fatalf("content[%d] text = %q, want %q", i, gotText, wantText)
				}
			}
		})
	}
}

func TestConvertClaudeRequestToCodex_DeferLoading_InitialRequest(t *testing.T) {
	input := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]}
		],
		"tools": [
			{
				"name": "ToolSearch",
				"description": "Search for tools",
				"input_schema": {"type": "object", "properties": {"query": {"type": "string"}}}
			},
			{
				"name": "Read",
				"description": "Read a file from the filesystem.",
				"input_schema": {"type": "object", "properties": {"file_path": {"type": "string"}}},
				"defer_loading": true
			}
		]
	}`)

	output := ConvertClaudeRequestToCodex("test-model", input, false)

	if !gjson.Valid(string(output)) {
		t.Fatal("output is not valid JSON")
	}

	toolsResult := gjson.GetBytes(output, "tools")
	if !toolsResult.Exists() || !toolsResult.IsArray() {
		t.Fatal("tools array is missing")
	}

	tools := toolsResult.Array()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool in output, got %d", len(tools))
	}

	if gjson.GetBytes(output, "tools.0.name").String() != "ToolSearch" {
		t.Errorf("expected tools.0.name to be ToolSearch, got %s", gjson.GetBytes(output, "tools.0.name").String())
	}

	for _, tool := range tools {
		if tool.Get("name").String() == "Read" {
			t.Error("Read tool should not appear in output tools (deferred, not loaded)")
		}
	}
}

func TestConvertClaudeRequestToCodex_DeferLoading_WithToolReference(t *testing.T) {
	input := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "tu_1", "name": "ToolSearch", "input": {"query": "select:Read"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tu_1", "content": [{"type": "tool_reference", "tool_name": "Read"}]}]}
		],
		"tools": [
			{
				"name": "ToolSearch",
				"description": "Search for tools",
				"input_schema": {"type": "object", "properties": {"query": {"type": "string"}}}
			},
			{
				"name": "Read",
				"description": "Reads a file from the local filesystem.",
				"input_schema": {"type": "object", "properties": {"file_path": {"type": "string", "description": "The absolute path to the file"}}},
				"defer_loading": true
			}
		]
	}`)

	output := ConvertClaudeRequestToCodex("test-model", input, false)

	if !gjson.Valid(string(output)) {
		t.Fatal("output is not valid JSON")
	}

	tools := gjson.GetBytes(output, "tools").Array()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools in output (ToolSearch + Read), got %d", len(tools))
	}

	var toolOutputMsg gjson.Result
	for _, item := range gjson.GetBytes(output, "input").Array() {
		if item.Get("type").String() == "function_call_output" {
			toolOutputMsg = item
			break
		}
	}
	if !toolOutputMsg.Exists() {
		t.Fatal("function_call_output message not found in output input array")
	}

	if strings.Contains(toolOutputMsg.Get("output").Raw, "tool_reference") {
		t.Error("output should not contain raw tool_reference JSON")
	}

	if toolOutputMsg.Get("output.0.type").String() != "input_text" {
		t.Errorf("expected output.0.type to be input_text, got %s", toolOutputMsg.Get("output.0.type").String())
	}

	text := toolOutputMsg.Get("output.0.text").String()
	if !strings.HasPrefix(text, "Tool 'Read' is now available.") {
		t.Errorf("expected text to start with \"Tool 'Read' is now available.\", got: %s", text)
	}
	if !strings.Contains(text, "Description:") {
		t.Error("expected text to contain 'Description:'")
	}
	if !strings.Contains(text, "Parameters:") {
		t.Error("expected text to contain 'Parameters:'")
	}
}

func TestConvertClaudeRequestToCodex_DeferLoading_MultipleTools(t *testing.T) {
	input := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "tu_1", "name": "ToolSearch", "input": {"query": "select:Read"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tu_1", "content": [{"type": "tool_reference", "tool_name": "Read"}]}]}
		],
		"tools": [
			{
				"name": "ToolSearch",
				"description": "Search for tools",
				"input_schema": {"type": "object", "properties": {"query": {"type": "string"}}}
			},
			{
				"name": "Read",
				"description": "Read a file",
				"input_schema": {"type": "object", "properties": {"file_path": {"type": "string"}}},
				"defer_loading": true
			},
			{
				"name": "Bash",
				"description": "Run a bash command",
				"input_schema": {"type": "object", "properties": {"command": {"type": "string"}}},
				"defer_loading": true
			}
		]
	}`)

	output := ConvertClaudeRequestToCodex("test-model", input, false)

	if !gjson.Valid(string(output)) {
		t.Fatal("output is not valid JSON")
	}

	tools := gjson.GetBytes(output, "tools").Array()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools (ToolSearch + Read), got %d", len(tools))
	}

	toolNames := map[string]bool{}
	for _, tool := range tools {
		toolNames[tool.Get("name").String()] = true
	}

	if !toolNames["ToolSearch"] {
		t.Error("ToolSearch should be in output tools")
	}
	if !toolNames["Read"] {
		t.Error("Read should be in output tools (loaded via tool_reference)")
	}
	if toolNames["Bash"] {
		t.Error("Bash should not be in output tools (deferred, not loaded)")
	}
}

func TestConvertClaudeRequestToCodex_DeferLoading_DuplicateToolReference(t *testing.T) {
	input := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "tu_1", "name": "ToolSearch", "input": {"query": "select:Read"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tu_1", "content": [{"type": "tool_reference", "tool_name": "Read"}]}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "tu_2", "name": "ToolSearch", "input": {"query": "select:Read"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tu_2", "content": [{"type": "tool_reference", "tool_name": "Read"}]}]}
		],
		"tools": [
			{
				"name": "ToolSearch",
				"description": "Search for tools",
				"input_schema": {"type": "object", "properties": {"query": {"type": "string"}}}
			},
			{
				"name": "Read",
				"description": "Read a file",
				"input_schema": {"type": "object", "properties": {"file_path": {"type": "string"}}},
				"defer_loading": true
			}
		]
	}`)

	output := ConvertClaudeRequestToCodex("test-model", input, false)

	if !gjson.Valid(string(output)) {
		t.Fatal("output is not valid JSON")
	}

	outputTools := gjson.GetBytes(output, "tools").Array()
	if len(outputTools) != 2 {
		t.Errorf("expected 2 tools (ToolSearch + Read), got %d", len(outputTools))
	}
	readCount := 0
	for _, tool := range outputTools {
		if tool.Get("name").String() == "Read" {
			readCount++
		}
	}
	if readCount != 1 {
		t.Errorf("expected Read to appear exactly once in output tools, got %d", readCount)
	}

	var toolOutputMsgs []gjson.Result
	for _, item := range gjson.GetBytes(output, "input").Array() {
		if item.Get("type").String() == "function_call_output" {
			toolOutputMsgs = append(toolOutputMsgs, item)
		}
	}
	if len(toolOutputMsgs) != 2 {
		t.Fatalf("expected 2 function_call_output messages, got %d", len(toolOutputMsgs))
	}
	for i, msg := range toolOutputMsgs {
		text := msg.Get("output.0.text").String()
		if !strings.HasPrefix(text, "Tool 'Read' is now available.") {
			t.Errorf("function_call_output[%d]: expected text to start with \"Tool 'Read' is now available.\", got: %s", i, text)
		}
	}
}

func TestConvertClaudeRequestToCodex_DeferLoading_UnknownToolReference(t *testing.T) {
	input := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "tu_1", "name": "ToolSearch", "input": {"query": "select:UnknownTool"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tu_1", "content": [{"type": "tool_reference", "tool_name": "UnknownTool"}]}]}
		],
		"tools": [
			{
				"name": "ToolSearch",
				"description": "Search for tools",
				"input_schema": {"type": "object", "properties": {"query": {"type": "string"}}}
			}
		]
	}`)

	output := ConvertClaudeRequestToCodex("test-model", input, false)

	if !gjson.Valid(string(output)) {
		t.Fatal("output is not valid JSON")
	}

	var toolOutputMsg gjson.Result
	for _, item := range gjson.GetBytes(output, "input").Array() {
		if item.Get("type").String() == "function_call_output" {
			toolOutputMsg = item
			break
		}
	}
	if !toolOutputMsg.Exists() {
		t.Fatal("function_call_output message not found")
	}

	if toolOutputMsg.Get("output.0.type").String() != "input_text" {
		t.Errorf("expected output.0.type to be input_text, got %s", toolOutputMsg.Get("output.0.type").String())
	}

	text := toolOutputMsg.Get("output.0.text").String()
	const wantText = "Tool 'UnknownTool' is now available."
	if text != wantText {
		t.Errorf("expected output.0.text to be %q, got %q", wantText, text)
	}
	if strings.Contains(text, "Description:") {
		t.Error("expected no 'Description:' section for unknown tool (not in toolSchemaMap)")
	}
	if strings.Contains(text, "Parameters:") {
		t.Error("expected no 'Parameters:' section for unknown tool (not in toolSchemaMap)")
	}

	outputTools := gjson.GetBytes(output, "tools").Array()
	if len(outputTools) != 1 {
		t.Errorf("expected 1 tool (ToolSearch only), got %d", len(outputTools))
	}
	if gjson.GetBytes(output, "tools.0.name").String() != "ToolSearch" {
		t.Errorf("expected tools.0.name to be ToolSearch, got %s", gjson.GetBytes(output, "tools.0.name").String())
	}
}

func TestConvertClaudeRequestToCodex_DeferLoading_AllDeferredNoReference(t *testing.T) {
	input := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]}
		],
		"tools": [
			{
				"name": "Bash",
				"description": "Run a bash command",
				"input_schema": {"type": "object", "properties": {"command": {"type": "string"}}},
				"defer_loading": true
			},
			{
				"name": "Read",
				"description": "Read a file",
				"input_schema": {"type": "object", "properties": {"file_path": {"type": "string"}}},
				"defer_loading": true
			}
		]
	}`)

	output := ConvertClaudeRequestToCodex("test-model", input, false)

	if !gjson.Valid(string(output)) {
		t.Fatal("output is not valid JSON")
	}

	toolsResult := gjson.GetBytes(output, "tools")
	if !toolsResult.IsArray() {
		t.Fatal("tools field must be an array (not null) even when empty")
	}
	if len(toolsResult.Array()) != 0 {
		t.Errorf("expected empty tools array, got %d tools", len(toolsResult.Array()))
	}
	if gjson.GetBytes(output, "tool_choice").String() != "auto" {
		t.Errorf("expected tool_choice to be auto, got %s", gjson.GetBytes(output, "tool_choice").String())
	}
	if !gjson.GetBytes(output, "parallel_tool_calls").Bool() {
		t.Error("expected parallel_tool_calls to be true")
	}
}

func TestConvertClaudeRequestToCodex_DeferLoading_MixedContentInToolResult(t *testing.T) {
	input := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello"}]},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "tu_1", "name": "ToolSearch", "input": {"query": "select:Read"}}]},
			{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tu_1", "content": [
				{"type": "text", "text": "search done"},
				{"type": "tool_reference", "tool_name": "Read"}
			]}]}
		],
		"tools": [
			{
				"name": "ToolSearch",
				"description": "Search for tools",
				"input_schema": {"type": "object", "properties": {"query": {"type": "string"}}}
			},
			{
				"name": "Read",
				"description": "Reads a file from the local filesystem.",
				"input_schema": {"type": "object", "properties": {"file_path": {"type": "string", "description": "The absolute path"}}},
				"defer_loading": true
			}
		]
	}`)

	output := ConvertClaudeRequestToCodex("test-model", input, false)

	if !gjson.Valid(string(output)) {
		t.Fatal("output is not valid JSON")
	}

	var toolOutputMsg gjson.Result
	for _, item := range gjson.GetBytes(output, "input").Array() {
		if item.Get("type").String() == "function_call_output" {
			toolOutputMsg = item
			break
		}
	}
	if !toolOutputMsg.Exists() {
		t.Fatal("function_call_output message not found")
	}

	outputArr := toolOutputMsg.Get("output").Array()
	if len(outputArr) != 2 {
		t.Fatalf("expected output array length 2 (text + tool_reference), got %d", len(outputArr))
	}

	if outputArr[0].Get("type").String() != "input_text" {
		t.Errorf("expected output[0].type to be input_text, got %s", outputArr[0].Get("type").String())
	}
	if outputArr[0].Get("text").String() != "search done" {
		t.Errorf("expected output[0].text to be 'search done', got %q", outputArr[0].Get("text").String())
	}

	if outputArr[1].Get("type").String() != "input_text" {
		t.Errorf("expected output[1].type to be input_text, got %s", outputArr[1].Get("type").String())
	}
	refText := outputArr[1].Get("text").String()
	if !strings.HasPrefix(refText, "Tool 'Read' is now available.") {
		t.Errorf("expected output[1].text to start with \"Tool 'Read' is now available.\", got: %s", refText)
	}
	if !strings.Contains(refText, "Description:") {
		t.Error("expected output[1].text to contain 'Description:'")
	}
	if !strings.Contains(refText, "Parameters:") {
		t.Error("expected output[1].text to contain 'Parameters:'")
	}

	toolNames := map[string]bool{}
	for _, tool := range gjson.GetBytes(output, "tools").Array() {
		toolNames[tool.Get("name").String()] = true
	}
	if !toolNames["Read"] {
		t.Error("Read should be in output tools after tool_reference")
	}
}
