package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricsruntime"
)

func TestResponseWriterWrapper_StreamingWrite_RecordsFirstContentToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "http://example.com/v1/test", nil)

	state := metricsruntime.NewRequestState(true, "gpt-5.2")
	state.StartedAt = time.Unix(100, 0)
	metricsruntime.AttachRequestState(c, state)

	w := NewResponseWriterWrapper(c.Writer, nil, &RequestInfo{
		URL:       "/v1/test",
		Method:    "POST",
		Headers:   map[string][]string{},
		Body:      nil,
		RequestID: "req_test",
		Timestamp: time.Unix(100, 0),
	})
	w.ginCtx = c

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)

	// Role-only chunks must not count as content tokens.
	msgStart := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"role\":\"assistant\",\"content\":[]}}\n\n")
	_, _ = w.Write(msgStart)

	snap := state.Snapshot()
	if snap.FirstContentTokenAt != nil {
		t.Fatalf("expected FirstContentTokenAt to remain nil for role-only chunk, got %v", snap.FirstContentTokenAt)
	}
	if snap.ContentTokenChunks != 0 {
		t.Fatalf("expected ContentTokenChunks=0 for role-only chunk, got %d", snap.ContentTokenChunks)
	}
	if snap.StatusCode != 200 {
		t.Fatalf("expected StatusCode=200 after WriteHeader, got %d", snap.StatusCode)
	}

	content := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
	_, _ = w.Write(content)

	snap = state.Snapshot()
	if snap.FirstContentTokenAt == nil {
		t.Fatalf("expected FirstContentTokenAt to be set after content chunk")
	}
	if snap.ContentTokenChunks != 1 {
		t.Fatalf("expected ContentTokenChunks=1 after first content chunk, got %d", snap.ContentTokenChunks)
	}
}
