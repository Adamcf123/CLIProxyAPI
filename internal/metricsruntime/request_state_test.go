package metricsruntime

import (
	"context"
	"encoding/json"
	"errors"
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

func TestMaybeRecordFirstContentToken_ClaudeToolInputJSONDeltaCountsAsContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "kimi-for-coding")
	AttachRequestState(c, state)

	toolArgsDelta := []byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"x\\\":1\"}}\n\n")
	MaybeRecordFirstContentToken(c, toolArgsDelta, time.Unix(500, 0))

	snap := state.Snapshot()
	if snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set for input_json_delta")
	}
	if snap.ContentTokenChunks != 1 {
		t.Fatalf("expected ContentTokenChunks=1, got %d", snap.ContentTokenChunks)
	}
}

func TestMaybeRecordFirstContentToken_ClaudeThinkingDeltaCountsAsContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "kimi-for-coding")
	AttachRequestState(c, state)

	chunk := []byte("event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n")
	when := time.Unix(600, 0)
	MaybeRecordFirstContentToken(c, chunk, when)

	snap := state.Snapshot()
	if snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set for thinking_delta")
	}
	if !snap.FirstContentTokenAt.Equal(when) {
		t.Fatalf("expected FirstContentTokenAt=%v, got %v", when, *snap.FirstContentTokenAt)
	}
	if snap.ContentTokenChunks != 1 {
		t.Fatalf("expected ContentTokenChunks=1, got %d", snap.ContentTokenChunks)
	}
}

func TestMaybeRecordFirstContentToken_OpenAIResponsesOutputTextDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "gpt-5.2")
	AttachRequestState(c, state)

	chunk := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n")
	want := time.Unix(300, 0)
	MaybeRecordFirstContentToken(c, chunk, want)

	snap := state.Snapshot()
	if snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set")
	}
	if !snap.FirstContentTokenAt.Equal(want) {
		t.Fatalf("expected FirstContentTokenAt=%v, got %v", want, *snap.FirstContentTokenAt)
	}
	if snap.LastContentTokenAt == nil {
		t.Fatalf("expected LastContentTokenAt to be set")
	}
	if !snap.LastContentTokenAt.Equal(want) {
		t.Fatalf("expected LastContentTokenAt=%v, got %v", want, *snap.LastContentTokenAt)
	}
	if snap.ContentTokenChunks != 1 {
		t.Fatalf("expected ContentTokenChunks=1, got %d", snap.ContentTokenChunks)
	}
}

func TestMaybeRecordFirstContentToken_OpenAIResponsesOutputTextDelta_EventLineCarriesType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "gpt-5.2")
	AttachRequestState(c, state)

	chunk := []byte("event: response.output_text.delta\n" +
		"data: {\"delta\":\"hi\"}\n\n")
	want := time.Unix(301, 0)
	MaybeRecordFirstContentToken(c, chunk, want)

	snap := state.Snapshot()
	if snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set")
	}
	if !snap.FirstContentTokenAt.Equal(want) {
		t.Fatalf("expected FirstContentTokenAt=%v, got %v", want, *snap.FirstContentTokenAt)
	}
	if snap.ContentTokenChunks != 1 {
		t.Fatalf("expected ContentTokenChunks=1, got %d", snap.ContentTokenChunks)
	}
}

func TestMaybeRecordFirstContentToken_OpenAIResponsesOutputTextDelta_SplitAcrossChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "gpt-5.2")
	AttachRequestState(c, state)

	// Simulate TCP chunking that splits the JSON payload across writes.
	chunk1 := []byte("event: response.output_text.delta\n" +
		"data: {\"delta\":\"hel")
	chunk2 := []byte("lo\"}\n\n")

	MaybeRecordFirstContentToken(c, chunk1, time.Unix(310, 0))
	if snap := state.Snapshot(); snap.FirstContentTokenAt != nil {
		t.Fatalf("expected FirstContentTokenAt to remain nil for partial SSE frame")
	}

	want := time.Unix(311, 0)
	MaybeRecordFirstContentToken(c, chunk2, want)

	snap := state.Snapshot()
	if snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set")
	}
	if !snap.FirstContentTokenAt.Equal(want) {
		t.Fatalf("expected FirstContentTokenAt=%v, got %v", want, *snap.FirstContentTokenAt)
	}
	if snap.ContentTokenChunks != 1 {
		t.Fatalf("expected ContentTokenChunks=1, got %d", snap.ContentTokenChunks)
	}
}

func TestMaybeRecordFirstContentToken_MultipleEventsInOneChunkCountsBoth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	state := NewRequestState(true, "gpt-5.2")
	AttachRequestState(c, state)

	chunk := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\n")
	MaybeRecordFirstContentToken(c, chunk, time.Unix(400, 0))

	if snap := state.Snapshot(); snap.ContentTokenChunks != 2 {
		t.Fatalf("expected ContentTokenChunks=2 for two content events in one chunk, got %d", snap.ContentTokenChunks)
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

func TestRequestState_MarkClientCanceledIsStickyUntilFailure(t *testing.T) {
	state := NewRequestState(false, "gpt-5.2")
	state.SetStatusCode(200)
	state.MarkClientCanceled()
	if !state.IsClientCanceled() {
		t.Fatalf("expected state to be client-canceled")
	}

	// Handler tail code should not override canceled back to success.
	state.SetStatusCode(200)
	if snap := state.Snapshot(); !snap.IsClientCanceled() || snap.StatusCode != statusClientClosedRequest {
		t.Fatalf("expected canceled to remain status_code=%d, got status_code=%d", statusClientClosedRequest, snap.StatusCode)
	}

	// Explicit failures must override canceled.
	state.SetStatusCode(500)
	if snap := state.Snapshot(); snap.IsClientCanceled() {
		t.Fatalf("expected canceled to be overridden by failure")
	}
	if snap := state.Snapshot(); snap.StatusCode != 500 {
		t.Fatalf("expected status_code=500, got %d", snap.StatusCode)
	}
}

func TestRequestState_LastErrorOverridesCanceledAndUses504ForDeadline(t *testing.T) {
	state := NewRequestState(false, "gpt-5.2")
	state.MarkClientCanceled()
	state.SetLastError(context.DeadlineExceeded)
	if snap := state.Snapshot(); snap.IsClientCanceled() {
		t.Fatalf("expected canceled to be cleared by LastError")
	}
	if snap := state.Snapshot(); snap.StatusCode != statusGatewayTimeout {
		t.Fatalf("expected status_code=%d, got %d", statusGatewayTimeout, snap.StatusCode)
	}
	if snap := state.Snapshot(); snap.LastError == "" {
		t.Fatalf("expected LastError to be set")
	}

	state = NewRequestState(false, "gpt-5.2")
	state.MarkClientCanceled()
	state.SetLastError(errors.New("boom"))
	if snap := state.Snapshot(); snap.IsClientCanceled() {
		t.Fatalf("expected canceled to be cleared by LastError")
	}
	if snap := state.Snapshot(); snap.LastError == "" {
		t.Fatalf("expected LastError to be set")
	}
	if snap := state.Snapshot(); snap.StatusCode != statusInternalServerError {
		t.Fatalf("expected status_code=%d, got %d", statusInternalServerError, snap.StatusCode)
	}
}
