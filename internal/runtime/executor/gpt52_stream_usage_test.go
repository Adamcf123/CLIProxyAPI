package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

// TestGPT52StreamUsageInjection 验证 gpt-5.2 流式请求是否注入 stream_options.include_usage
func TestGPT52StreamUsageInjection(t *testing.T) {
	var gotBody []byte
	var requestCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		body, _ := io.ReadAll(r.Body)
		gotBody = body

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// 模拟真实的 OpenAI 流式响应，包含 usage chunk
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n"))
		// 关键：包含 usage 的 chunk（当 stream_options.include_usage=true 时才会出现）
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test-key",
	}}

	payload := []byte(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	stream, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.2",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	// 消费流式响应
	chunkCount := 0
	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			t.Logf("Stream chunk error: %v", chunk.Err)
		}
		chunkCount++
	}

	if chunkCount == 0 {
		t.Error("No chunks received from stream")
	}

	// 验证请求体中是否包含 stream_options.include_usage=true
	if !gjson.GetBytes(gotBody, "stream_options.include_usage").Exists() {
		t.Errorf("stream_options.include_usage not found in request body. Body: %s", string(gotBody))
	}

	if gjson.GetBytes(gotBody, "stream_options.include_usage").Bool() != true {
		t.Errorf("stream_options.include_usage should be true, got: %s", string(gotBody))
	}

	// 验证原始 payload 中的其他字段被保留
	if !gjson.GetBytes(gotBody, "model").Exists() {
		t.Error("model field missing from request body")
	}

	if gjson.GetBytes(gotBody, "model").String() != "gpt-5.2" {
		t.Errorf("model should be gpt-5.2, got: %s", gjson.GetBytes(gotBody, "model").String())
	}

	t.Logf("✓ Test passed: stream_options.include_usage=true correctly injected for gpt-5.2")
	t.Logf("  Request body: %s", string(gotBody))
}

// TestGPT52StreamUsageParsing 验证 usage chunk 能被正确解析
func TestGPT52StreamUsageParsing(t *testing.T) {
	testCases := []struct {
		name     string
		line     []byte
		expected bool
		detail   struct {
			input  int64
			output int64
			total  int64
		}
	}{
		{
			name:     "usage chunk with prompt/completion tokens",
			line:     []byte(`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`),
			expected: true,
			detail:   struct{ input, output, total int64 }{10, 5, 15},
		},

		{
			name:     "regular chunk without usage",
			line:     []byte(`data: {"choices":[{"delta":{"content":"hello"}}]}`),
			expected: false,
		},
		{
			name:     "[DONE] marker",
			line:     []byte(`data: [DONE]`),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detail, ok := parseOpenAIStreamUsage(tc.line)
			if ok != tc.expected {
				t.Errorf("expected ok=%v, got ok=%v", tc.expected, ok)
			}
			if tc.expected && ok {
				if detail.InputTokens != tc.detail.input {
					t.Errorf("input_tokens: expected %d, got %d", tc.detail.input, detail.InputTokens)
				}
				if detail.OutputTokens != tc.detail.output {
					t.Errorf("output_tokens: expected %d, got %d", tc.detail.output, detail.OutputTokens)
				}
				if detail.TotalTokens != tc.detail.total {
					t.Errorf("total_tokens: expected %d, got %d", tc.detail.total, detail.TotalTokens)
				}
			}
		})
	}
}

// TestGPT52NonStreamNotAffected 验证非流式请求不受影响
func TestGPT52NonStreamNotAffected(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-123","object":"chat.completion","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test-key",
	}}

	payload := []byte(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.2",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	// 非流式请求不应该有 stream_options
	if gjson.GetBytes(gotBody, "stream_options").Exists() {
		t.Errorf("non-stream request should not have stream_options, but found: %s", string(gotBody))
	}

	// 但应该有 model 字段
	if gjson.GetBytes(gotBody, "model").String() != "gpt-5.2" {
		t.Errorf("model should be gpt-5.2, got: %s", string(gotBody))
	}
}

// TestStreamOptionsPreserved 验证如果请求中已有 stream_options，会被保留但不覆盖 include_usage
func TestStreamOptionsPreserved(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test-key",
	}}

	// 请求中已经有 stream_options
	payload := []byte(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":false}}`)
	stream, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.2",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	for range stream.Chunks {
	}

	// 我们的代码应该强制设置 include_usage=true（覆盖用户设置）
	if gjson.GetBytes(gotBody, "stream_options.include_usage").Bool() != true {
		t.Errorf("stream_options.include_usage should be forced to true, got: %s", string(gotBody))
	}
}

// BenchmarkStreamUsageParsing 基准测试 usage parsing 性能
func BenchmarkStreamUsageParsing(b *testing.B) {
	line := []byte(`data: {"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":500,"total_tokens":1500}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseOpenAIStreamUsage(line)
	}
}

// verifyUsageInMetricsSummary 辅助函数：从 stderr 输出中提取 metrics_summary
// 注意：这个函数需要在真实环境中手动验证
func verifyUsageInMetricsSummary(output string) (trackingID string, hasTokens bool) {
	// 查找 metrics_summary JSON 行
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "metrics_summary") {
			// 提取 tracking_id
			trackingID = gjson.Get(line, "tracking_id").String()
			// 检查 tokens 是否为 null
			inputTokens := gjson.Get(line, "input_tokens")
			outputTokens := gjson.Get(line, "output_tokens")
			hasTokens = inputTokens.Exists() && inputTokens.Raw != "null" &&
				outputTokens.Exists() && outputTokens.Raw != "null"
			return
		}
	}
	return "", false
}
