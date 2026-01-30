package metricsruntime

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"net/http/httptest"
)

func TestMaybeRecordFirstContentToken_OpenAIChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "gpt-5.2")
	AttachRequestState(c, state)

	roleOnly := []byte(`{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`)
	MaybeRecordFirstContentToken(c, roleOnly, time.Unix(100, 0))
	if snap := state.Snapshot(); snap.FirstContentTokenAt != nil {
		t.Fatalf("expected FirstContentTokenAt to remain nil for role-only chunk, got %v", snap.FirstContentTokenAt)
	}
	if snap := state.Snapshot(); snap.ContentTokenChunks != 0 {
		t.Fatalf("expected ContentTokenChunks=0 for role-only chunk, got %d", snap.ContentTokenChunks)
	}

	content := []byte(`{"choices":[{"index":0,"delta":{"content":"Hello"}}]}`)
	want := time.Unix(101, 0)
	MaybeRecordFirstContentToken(c, content, want)
	if snap := state.Snapshot(); snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set")
	} else if !snap.FirstContentTokenAt.Equal(want) {
		t.Fatalf("expected FirstContentTokenAt=%v, got %v", want, *snap.FirstContentTokenAt)
	}
	if snap := state.Snapshot(); snap.ContentTokenChunks != 1 {
		t.Fatalf("expected ContentTokenChunks=1 after first content chunk, got %d", snap.ContentTokenChunks)
	}

	// Once set, it must not change.
	MaybeRecordFirstContentToken(c, content, time.Unix(102, 0))
	if snap := state.Snapshot(); snap.FirstContentTokenAt == nil || !snap.FirstContentTokenAt.Equal(want) {
		t.Fatalf("expected FirstContentTokenAt to remain %v, got %v", want, snap.FirstContentTokenAt)
	}
	if snap := state.Snapshot(); snap.ContentTokenChunks != 2 {
		t.Fatalf("expected ContentTokenChunks=2 after second content chunk, got %d", snap.ContentTokenChunks)
	}
}

func TestMaybeRecordFirstContentToken_ClaudeSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "minimax-m2.1")
	AttachRequestState(c, state)

	msgStart := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"role\":\"assistant\",\"content\":[]}}\n\n")
	MaybeRecordFirstContentToken(c, msgStart, time.Unix(200, 0))
	if snap := state.Snapshot(); snap.FirstContentTokenAt != nil {
		t.Fatalf("expected FirstContentTokenAt to remain nil for message_start")
	}

	textDelta := []byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
	want := time.Unix(201, 123)
	MaybeRecordFirstContentToken(c, textDelta, want)
	if snap := state.Snapshot(); snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set for content_block_delta")
	} else if !snap.FirstContentTokenAt.Equal(want) {
		t.Fatalf("expected FirstContentTokenAt=%v, got %v", want, *snap.FirstContentTokenAt)
	}
}

func TestPrintSummary_TTFTFallbackWithoutMetrics(t *testing.T) {
	state := NewRequestState(true, "gpt-5.2")
	state.SetProvider("openai")
	state.StartedAt = time.Unix(100, 0)
	t1 := time.Unix(101, 500_000_000) // +1.5s
	state.FirstContentTokenAt = &t1
	state.SetStatusCode(200)
	state.SetRequestPath("/v1/chat/completions")

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	PrintSummary(state)
	_ = w.Close()
	out, _ := io.ReadAll(r)

	line := strings.TrimSpace(string(out))
	if !strings.HasPrefix(line, "metrics_summary ") {
		t.Fatalf("expected metrics_summary prefix, got %q", line)
	}
	jsonPart := strings.TrimPrefix(line, "metrics_summary ")

	var m map[string]any
	if err := json.Unmarshal([]byte(jsonPart), &m); err != nil {
		t.Fatalf("unmarshal summary JSON: %v\nraw=%q", err, jsonPart)
	}
	got, ok := m["ttft"].(float64)
	if !ok {
		t.Fatalf("expected ttft to be a number, got %T (%v)", m["ttft"], m["ttft"])
	}
	if got < 1.49 || got > 1.51 {
		t.Fatalf("expected ttft about 1.5, got %v", got)
	}
}
