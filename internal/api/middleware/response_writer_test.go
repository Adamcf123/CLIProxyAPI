package middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricsruntime"
)

func TestExtractRequestBodyPrefersOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	wrapper := &ResponseWriterWrapper{
		requestInfo: &RequestInfo{Body: []byte("original-body")},
	}

	body := wrapper.extractRequestBody(c)
	if string(body) != "original-body" {
		t.Fatalf("request body = %q, want %q", string(body), "original-body")
	}

	c.Set(requestBodyOverrideContextKey, []byte("override-body"))
	body = wrapper.extractRequestBody(c)
	if string(body) != "override-body" {
		t.Fatalf("request body = %q, want %q", string(body), "override-body")
	}
}

func TestExtractRequestBodySupportsStringOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	wrapper := &ResponseWriterWrapper{}
	c.Set(requestBodyOverrideContextKey, "override-as-string")

	body := wrapper.extractRequestBody(c)
	if string(body) != "override-as-string" {
		t.Fatalf("request body = %q, want %q", string(body), "override-as-string")
	}
}

type erroringResponseWriter struct {
	gin.ResponseWriter
	err error
}

func (w *erroringResponseWriter) Write(p []byte) (int, error) {
	if w.err == nil {
		return w.ResponseWriter.Write(p)
	}
	return 0, w.err
}

func (w *erroringResponseWriter) WriteString(s string) (int, error) {
	if w.err == nil {
		return w.ResponseWriter.WriteString(s)
	}
	return 0, w.err
}

func TestResponseWriterWrapper_Finalize_NonStreamingWriteErrorMarksCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "http://example.com/v1/test", nil)

	state := metricsruntime.NewRequestState(false, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)

	base := c.Writer
	errWriter := &erroringResponseWriter{ResponseWriter: base, err: errors.New("client gone")}

	w := NewResponseWriterWrapper(errWriter, nil, &RequestInfo{
		URL:       "/v1/test",
		Method:    "GET",
		Headers:   map[string][]string{},
		Body:      nil,
		RequestID: "req_test",
		Timestamp: time.Unix(100, 0),
	})

	w.WriteHeader(200)
	_, _ = w.Write([]byte("hello"))
	_ = w.Finalize(c)

	snap := state.Snapshot()
	if snap.StatusCode != 499 || !snap.IsClientCanceled() {
		t.Fatalf("expected canceled status_code=499, got %d", snap.StatusCode)
	}
	if snap.LastError != "" {
		t.Fatalf("expected LastError to remain empty for canceled, got %q", snap.LastError)
	}
}

func TestResponseWriterWrapper_Finalize_DeadlineExceededMarksFailureNotCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "http://example.com/v1/test", nil).WithContext(deadlineCtx)

	state := metricsruntime.NewRequestState(false, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)

	base := c.Writer
	errWriter := &erroringResponseWriter{ResponseWriter: base, err: errors.New("client gone")}

	w := NewResponseWriterWrapper(errWriter, nil, &RequestInfo{
		URL:       "/v1/test",
		Method:    "GET",
		Headers:   map[string][]string{},
		Body:      nil,
		RequestID: "req_test",
		Timestamp: time.Unix(100, 0),
	})

	w.WriteHeader(200)
	_, _ = w.WriteString("hello")
	_ = w.Finalize(c)

	snap := state.Snapshot()
	if snap.IsClientCanceled() || snap.StatusCode == 499 {
		t.Fatalf("expected timeout to be failure (not canceled), got status_code=%d", snap.StatusCode)
	}
	if snap.StatusCode != 504 {
		t.Fatalf("expected status_code=504 for DeadlineExceeded, got %d", snap.StatusCode)
	}
	if snap.LastError == "" {
		t.Fatalf("expected LastError to be set for DeadlineExceeded")
	}
}
