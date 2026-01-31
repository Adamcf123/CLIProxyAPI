package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

type chatCompletionRequest struct {
	Model    string          `json:"model"`
	Messages json.RawMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

func main() {
	port := 53357
	if v := os.Getenv("PORT"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			port = parsed
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", handleChatCompletions)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("mock upstream listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatCompletionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// Keep this forgiving: upstream mocks should be easy to use.
		http.Error(w, fmt.Sprintf("bad json: %v", err), http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		http.Error(w, "missing model", http.StatusBadRequest)
		return
	}

	if req.Stream {
		writeAndDisconnectStream(w, req.Model)
		return
	}

	// Non-streaming: return a minimal OpenAI-compatible response without usage.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	payload := map[string]any{
		"id":      "mockcmpl-1",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "hello from mock upstream (no usage)",
				},
				"finish_reason": "stop",
			},
		},
		// Intentionally omit "usage" to exercise the no-usage path.
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAndDisconnectStream(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	fl, ok := w.(http.Flusher)
	if !ok {
		return
	}

	// Emit 1-2 SSE events, then drop the connection without sending [DONE].
	event1 := map[string]any{
		"id":      "mockcmpl-1",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{"content": "partial "},
			},
		},
	}
	writeSSE(w, event1)
	fl.Flush()

	time.Sleep(50 * time.Millisecond)

	event2 := map[string]any{
		"id":      "mockcmpl-1",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"delta": map[string]any{"content": "data"},
			},
		},
	}
	writeSSE(w, event2)
	fl.Flush()

	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
	_ = conn.Close()
}

func writeSSE(w http.ResponseWriter, obj any) {
	b, err := json.Marshal(obj)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
}

// Ensure net is referenced for go vet paranoia in some environments.
var _ = net.IPv4len
