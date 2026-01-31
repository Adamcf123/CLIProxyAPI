package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type chatCompletionPayload struct {
	Model     string `json:"model"`
	Stream    bool   `json:"stream"`
	MaxTokens int    `json:"max_tokens"`
	Messages  []any  `json:"messages"`
}

func main() {
	var (
		baseURL          string
		apiKey           string
		model            string
		cancelAfterBytes int
		timeoutSec       int
	)

	flag.StringVar(&baseURL, "url", "http://127.0.0.1:53356", "CLIProxyAPI base URL")
	flag.StringVar(&apiKey, "api-key", os.Getenv("API_KEY"), "API key (prefer env API_KEY)")
	flag.StringVar(&model, "model", "mock-stream", "Model alias")
	flag.IntVar(&cancelAfterBytes, "cancel-after-bytes", 256, "Cancel after reading N bytes")
	flag.IntVar(&timeoutSec, "timeout-sec", 15, "Overall request timeout seconds")
	flag.Parse()

	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ERROR: api key is required (use env API_KEY or --api-key)")
		os.Exit(2)
	}
	if cancelAfterBytes <= 0 {
		fmt.Fprintln(os.Stderr, "ERROR: cancel-after-bytes must be > 0")
		os.Exit(2)
	}

	payload := chatCompletionPayload{
		Model:     model,
		Stream:    true,
		MaxTokens: 128,
		Messages: []any{
			map[string]any{"role": "user", "content": "stream and then I will cancel"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: marshal payload: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: new request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	buf := make([]byte, 1024)
	readTotal := 0
	for readTotal < cancelAfterBytes {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			readTotal += n
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "ERROR: read response: %v\n", err)
			os.Exit(1)
		}
	}

	// Cancel request context to simulate client disconnect/cancel.
	cancel()
	_ = resp.Body.Close()

	fmt.Printf("HTTP_STATUS=%d\n", resp.StatusCode)
	fmt.Printf("CANCELED_AFTER_BYTES=%d\n", readTotal)
	os.Exit(0)
}
