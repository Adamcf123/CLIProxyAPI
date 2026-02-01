package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestGPT52CodexStreamRequestAndUsage(t *testing.T) {
	var gotPath string
	var gotBody []byte
	var gotAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		gotBody = body

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test-key",
	}}

	// Minimal Codex Responses request payload.
	payload := []byte(`{"model":"gpt-5.2-codex","stream":true,"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	stream, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.2-codex",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("codex"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	chunkCount := 0
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error chunk: %v", chunk.Err)
		}
		chunkCount++
	}
	if chunkCount == 0 {
		t.Fatalf("expected at least 1 stream chunk")
	}

	if gotPath != "/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/responses")
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("accept = %q, want %q", gotAccept, "text/event-stream")
	}

	if gjson.GetBytes(gotBody, "model").String() != "gpt-5.2-codex" {
		t.Fatalf("model = %q, want %q (body=%s)", gjson.GetBytes(gotBody, "model").String(), "gpt-5.2-codex", string(gotBody))
	}
	if !gjson.GetBytes(gotBody, "stream").Exists() || !gjson.GetBytes(gotBody, "stream").Bool() {
		t.Fatalf("expected stream=true in request body (body=%s)", string(gotBody))
	}
	if !gjson.GetBytes(gotBody, "instructions").Exists() {
		t.Fatalf("expected instructions field to exist (body=%s)", string(gotBody))
	}
}

func TestGPT52ResponsesStreamRequestAndUsage(t *testing.T) {
	var gotPath string
	var gotBody []byte
	var gotAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		gotBody = body

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test-key",
	}}

	// This matches the OpenAI Responses API request shape (openai-response handler type).
	payload := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	stream, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.2",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	chunkCount := 0
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("stream error chunk: %v", chunk.Err)
		}
		chunkCount++
	}
	if chunkCount == 0 {
		t.Fatalf("expected at least 1 stream chunk")
	}

	if gotPath != "/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/responses")
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("accept = %q, want %q", gotAccept, "text/event-stream")
	}
	if gjson.GetBytes(gotBody, "model").String() != "gpt-5.2" {
		t.Fatalf("model = %q, want %q (body=%s)", gjson.GetBytes(gotBody, "model").String(), "gpt-5.2", string(gotBody))
	}
}

func TestGPT52CodexUsageParsing(t *testing.T) {
	data := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`)
	detail, ok := parseCodexUsage(data)
	if !ok {
		t.Fatalf("expected parseCodexUsage ok=true")
	}
	if detail.InputTokens != 10 || detail.OutputTokens != 5 || detail.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", detail)
	}
}
