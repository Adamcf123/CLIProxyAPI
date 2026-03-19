package executor

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func maybeBuildMiniMaxAnthropicCodexResponseRequest(baseURL string, from sdktranslator.Format, modelName string, inputRawJSON []byte, stream bool) ([]byte, bool) {
	if !isMiniMaxAnthropicBaseURL(baseURL) {
		return nil, false
	}
	if from != sdktranslator.FormatOpenAIResponse {
		return nil, false
	}
	return buildMiniMaxAnthropicCodexResponseRequest(modelName, inputRawJSON, stream), true
}

func isMiniMaxAnthropicBaseURL(baseURL string) bool {
	normalized := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(normalized, "api.minimaxi.com/anthropic")
}

func buildMiniMaxAnthropicCodexResponseRequest(modelName string, inputRawJSON []byte, stream bool) []byte {
	out := `{"model":"","max_tokens":32000,"messages":[]}`
	root := gjson.ParseBytes(inputRawJSON)

	out, _ = sjson.Set(out, "model", modelName)
	out, _ = sjson.Set(out, "stream", stream)

	if mot := root.Get("max_output_tokens"); mot.Exists() && mot.Int() > 0 {
		out, _ = sjson.Set(out, "max_tokens", mot.Int())
	}
	if v := root.Get("temperature"); v.Exists() {
		out, _ = sjson.Set(out, "temperature", v.Value())
	}
	if v := root.Get("top_p"); v.Exists() {
		out, _ = sjson.Set(out, "top_p", v.Value())
	}
	if metadata := root.Get("metadata"); metadata.Exists() && metadata.IsObject() {
		out, _ = sjson.SetRaw(out, "metadata", metadata.Raw)
	}

	out = applyMiniMaxAnthropicReasoning(out, root)

	systemText := collectMiniMaxAnthropicSystemText(root)
	if strings.TrimSpace(systemText) != "" {
		out, _ = sjson.Set(out, "system", systemText)
	}

	if input := root.Get("input"); input.Exists() && input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			itemType := strings.TrimSpace(item.Get("type").String())
			if itemType == "" && strings.TrimSpace(item.Get("role").String()) != "" {
				itemType = "message"
			}

			switch itemType {
			case "message":
				if strings.EqualFold(item.Get("role").String(), "system") {
					return true
				}
				if msg, ok := buildMiniMaxAnthropicMessage(item); ok {
					out, _ = sjson.SetRaw(out, "messages.-1", msg)
				}
			case "function_call":
				if msg, ok := buildMiniMaxAnthropicToolUseMessage(item); ok {
					out, _ = sjson.SetRaw(out, "messages.-1", msg)
				}
			case "function_call_output":
				if msg, ok := buildMiniMaxAnthropicToolResultMessage(item); ok {
					out, _ = sjson.SetRaw(out, "messages.-1", msg)
				}
			}
			return true
		})
	}

	if tools := root.Get("tools"); tools.Exists() && tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			if !isSupportedMiniMaxAnthropicFunctionTool(tool) {
				return true
			}
			entry := `{"name":"","input_schema":{}}`
			entry, _ = sjson.Set(entry, "name", strings.TrimSpace(tool.Get("name").String()))
			if d := strings.TrimSpace(tool.Get("description").String()); d != "" {
				entry, _ = sjson.Set(entry, "description", d)
			}
			params := tool.Get("parameters")
			if !params.Exists() {
				params = tool.Get("parametersJsonSchema")
			}
			entry, _ = sjson.SetRaw(entry, "input_schema", params.Raw)
			out, _ = sjson.SetRaw(out, "tools.-1", entry)
			return true
		})
	}

	if toolChoice := root.Get("tool_choice"); toolChoice.Exists() {
		if mapped, ok := mapMiniMaxAnthropicToolChoice(toolChoice); ok {
			out, _ = sjson.SetRaw(out, "tool_choice", mapped)
		}
	}

	return []byte(out)
}

func applyMiniMaxAnthropicReasoning(out string, root gjson.Result) string {
	v := root.Get("reasoning.effort")
	if !v.Exists() {
		return out
	}
	if strings.TrimSpace(v.String()) == "" {
		return out
	}

	out, _ = sjson.Set(out, "thinking.type", "adaptive")
	out, _ = sjson.Set(out, "output_config.effort", "high")
	return out
}

func collectMiniMaxAnthropicSystemText(root gjson.Result) string {
	var sections []string
	if v := strings.TrimSpace(root.Get("instructions").String()); v != "" {
		sections = append(sections, v)
	}
	if input := root.Get("input"); input.Exists() && input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			if !strings.EqualFold(item.Get("role").String(), "system") {
				return true
			}
			if text := collectMiniMaxAnthropicTextParts(item.Get("content")); text != "" {
				sections = append(sections, text)
			}
			return true
		})
	}
	return strings.Join(sections, "\n\n")
}

func buildMiniMaxAnthropicMessage(item gjson.Result) (string, bool) {
	role := strings.TrimSpace(item.Get("role").String())
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "assistant" {
		return "", false
	}

	msg := `{"role":"","content":[]}`
	msg, _ = sjson.Set(msg, "role", role)
	added := false

	content := item.Get("content")
	if content.IsArray() {
		content.ForEach(func(_, part gjson.Result) bool {
			switch strings.TrimSpace(part.Get("type").String()) {
			case "input_text", "output_text":
				text := part.Get("text").String()
				if strings.TrimSpace(text) == "" {
					return true
				}
				block := `{"type":"text","text":""}`
				block, _ = sjson.Set(block, "text", text)
				msg, _ = sjson.SetRaw(msg, "content.-1", block)
				added = true
			case "reasoning", "thinking":
				text := part.Get("text").String()
				if text == "" {
					text = part.Get("summary.0.text").String()
				}
				if strings.TrimSpace(text) == "" {
					return true
				}
				block := `{"type":"thinking","thinking":""}`
				block, _ = sjson.Set(block, "thinking", text)
				msg, _ = sjson.SetRaw(msg, "content.-1", block)
				added = true
			}
			return true
		})
	}

	return msg, added
}

func buildMiniMaxAnthropicToolUseMessage(item gjson.Result) (string, bool) {
	name := strings.TrimSpace(item.Get("name").String())
	if name == "" {
		return "", false
	}
	callID := strings.TrimSpace(item.Get("call_id").String())
	if callID == "" {
		callID = genMiniMaxAnthropicToolCallID()
	}

	toolUse := `{"type":"tool_use","id":"","name":"","input":{}}`
	toolUse, _ = sjson.Set(toolUse, "id", callID)
	toolUse, _ = sjson.Set(toolUse, "name", name)

	args := item.Get("arguments")
	switch {
	case args.IsObject():
		toolUse, _ = sjson.SetRaw(toolUse, "input", args.Raw)
	case args.Type == gjson.String && gjson.Valid(args.String()):
		parsed := gjson.Parse(args.String())
		if parsed.IsObject() {
			toolUse, _ = sjson.SetRaw(toolUse, "input", parsed.Raw)
		}
	}

	msg := `{"role":"assistant","content":[]}`
	msg, _ = sjson.SetRaw(msg, "content.-1", toolUse)
	return msg, true
}

func buildMiniMaxAnthropicToolResultMessage(item gjson.Result) (string, bool) {
	callID := strings.TrimSpace(item.Get("call_id").String())
	if callID == "" {
		return "", false
	}

	toolResult := `{"type":"tool_result","tool_use_id":"","content":""}`
	toolResult, _ = sjson.Set(toolResult, "tool_use_id", callID)

	output := item.Get("output")
	switch {
	case output.Type == gjson.String:
		toolResult, _ = sjson.Set(toolResult, "content", output.String())
	case output.Exists():
		toolResult, _ = sjson.Set(toolResult, "content", output.Raw)
	default:
		toolResult, _ = sjson.Set(toolResult, "content", "")
	}

	msg := `{"role":"user","content":[]}`
	msg, _ = sjson.SetRaw(msg, "content.-1", toolResult)
	return msg, true
}

func mapMiniMaxAnthropicToolChoice(toolChoice gjson.Result) (string, bool) {
	switch toolChoice.Type {
	case gjson.String:
		switch strings.TrimSpace(toolChoice.String()) {
		case "auto":
			return `{"type":"auto"}`, true
		case "required":
			return `{"type":"any"}`, true
		default:
			return "", false
		}
	case gjson.JSON:
		if toolChoice.Get("type").String() == "function" {
			name := strings.TrimSpace(toolChoice.Get("function.name").String())
			if name == "" {
				return "", false
			}
			mapped := `{"type":"tool","name":""}`
			mapped, _ = sjson.Set(mapped, "name", name)
			return mapped, true
		}
	}
	return "", false
}

func isSupportedMiniMaxAnthropicFunctionTool(tool gjson.Result) bool {
	if !tool.Exists() || !tool.IsObject() {
		return false
	}
	if toolType := strings.TrimSpace(tool.Get("type").String()); toolType != "" && toolType != "function" {
		return false
	}
	if strings.TrimSpace(tool.Get("name").String()) == "" {
		return false
	}

	params := tool.Get("parameters")
	if !params.Exists() {
		params = tool.Get("parametersJsonSchema")
	}
	if !params.Exists() || !params.IsObject() {
		return false
	}

	return true
}

func collectMiniMaxAnthropicTextParts(content gjson.Result) string {
	if content.Type == gjson.String {
		return strings.TrimSpace(content.String())
	}
	if !content.IsArray() {
		return ""
	}
	var parts []string
	content.ForEach(func(_, part gjson.Result) bool {
		text := strings.TrimSpace(part.Get("text").String())
		if text != "" {
			parts = append(parts, text)
		}
		return true
	})
	return strings.Join(parts, "\n")
}

func genMiniMaxAnthropicToolCallID() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b strings.Builder
	for i := 0; i < 24; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		b.WriteByte(letters[n.Int64()])
	}
	return fmt.Sprintf("toolu_%s", b.String())
}
