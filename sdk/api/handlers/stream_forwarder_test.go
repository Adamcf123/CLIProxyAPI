package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metricsruntime"
)

func TestForwardStream_CanceledContextMarksCanceledStatus499(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	ctx, cancelCtx := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil).WithContext(ctx)
	c.Request = req

	state := metricsruntime.NewRequestState(true, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)
	state.SetStatusCode(200)

	h := &BaseAPIHandler{}
	data := make(chan []byte)
	errs := make(chan *interfaces.ErrorMessage)

	cancelCh := make(chan error, 1)
	cancel := func(err error) {
		select {
		case cancelCh <- err:
		default:
		}
	}

	zero := time.Duration(0)
	opts := StreamForwardOptions{KeepAliveInterval: &zero}

	done := make(chan struct{})
	go func() {
		h.ForwardStream(c, c.Writer, cancel, data, errs, opts)
		close(done)
	}()

	cancelCtx()

	select {
	case <-cancelCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancel")
	}
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ForwardStream to return")
	}

	snap := state.Snapshot()
	if !snap.IsClientCanceled() {
		t.Fatalf("expected request to be marked canceled")
	}
	if snap.StatusCode != 499 {
		t.Fatalf("expected status_code=499, got %d", snap.StatusCode)
	}
	if snap.LastError != "" {
		t.Fatalf("expected canceled request to keep LastError empty, got %q", snap.LastError)
	}
}

func TestForwardStream_DeadlineExceededMarksFailureNotCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	ctx, cancelCtx := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancelCtx()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil).WithContext(ctx)
	c.Request = req

	state := metricsruntime.NewRequestState(true, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)
	state.SetStatusCode(200)

	h := &BaseAPIHandler{}
	data := make(chan []byte)
	errs := make(chan *interfaces.ErrorMessage)

	cancelCh := make(chan error, 1)
	cancel := func(err error) {
		select {
		case cancelCh <- err:
		default:
		}
	}

	zero := time.Duration(0)
	opts := StreamForwardOptions{KeepAliveInterval: &zero}

	done := make(chan struct{})
	go func() {
		h.ForwardStream(c, c.Writer, cancel, data, errs, opts)
		close(done)
	}()

	select {
	case <-cancelCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancel")
	}
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ForwardStream to return")
	}

	snap := state.Snapshot()
	if snap.IsClientCanceled() || snap.StatusCode == 499 {
		t.Fatalf("expected timeout to NOT be marked canceled")
	}
	if snap.LastError == "" {
		t.Fatalf("expected timeout to set LastError")
	}
}

func TestForwardStream_UpstreamErrorBeatsCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	ctx, cancelCtx := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil).WithContext(ctx)
	c.Request = req

	state := metricsruntime.NewRequestState(true, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)
	state.SetStatusCode(200)

	h := &BaseAPIHandler{}
	data := make(chan []byte)
	errs := make(chan *interfaces.ErrorMessage, 1)

	cancelCh := make(chan error, 1)
	cancel := func(err error) {
		select {
		case cancelCh <- err:
		default:
		}
	}

	zero := time.Duration(0)
	opts := StreamForwardOptions{KeepAliveInterval: &zero}

	done := make(chan struct{})
	go func() {
		h.ForwardStream(c, c.Writer, cancel, data, errs, opts)
		close(done)
	}()

	errs <- &interfaces.ErrorMessage{StatusCode: 500, Error: errors.New("upstream")}
	cancelCtx()

	select {
	case <-cancelCh:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancel")
	}
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ForwardStream to return")
	}

	snap := state.Snapshot()
	if snap.IsClientCanceled() || snap.StatusCode == 499 {
		t.Fatalf("expected upstream error to override canceled")
	}
	if snap.LastError == "" {
		t.Fatalf("expected upstream error to set LastError")
	}
}

func TestForwardStream_TerminalErrorSetsLastError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	state := metricsruntime.NewRequestState(true, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)

	h := &BaseAPIHandler{}
	data := make(chan []byte)
	errs := make(chan *interfaces.ErrorMessage, 1)

	cancelCh := make(chan error, 1)
	cancel := func(err error) {
		select {
		case cancelCh <- err:
		default:
		}
	}

	zero := time.Duration(0)
	opts := StreamForwardOptions{
		KeepAliveInterval: &zero,
		WriteTerminalError: func(*interfaces.ErrorMessage) {
			// No-op: ForwardStream should set RequestState.LastError before this is invoked.
		},
	}

	done := make(chan struct{})
	go func() {
		h.ForwardStream(c, c.Writer, cancel, data, errs, opts)
		close(done)
	}()

	errs <- &interfaces.ErrorMessage{StatusCode: 500, Error: errors.New("boom")}

	select {
	case err := <-cancelCh:
		if err == nil {
			t.Fatalf("expected cancel error to be non-nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancel")
	}

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ForwardStream to return")
	}

	if got := state.Snapshot().LastError; got != "boom" {
		t.Fatalf("expected LastError=\"boom\", got %q", got)
	}
}

func TestForwardStream_NoErrorLeavesLastErrorEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	state := metricsruntime.NewRequestState(true, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)

	h := &BaseAPIHandler{}
	data := make(chan []byte)
	errs := make(chan *interfaces.ErrorMessage)
	close(data)
	close(errs)

	cancelCh := make(chan error, 1)
	cancel := func(err error) {
		select {
		case cancelCh <- err:
		default:
		}
	}

	zero := time.Duration(0)
	opts := StreamForwardOptions{KeepAliveInterval: &zero}

	done := make(chan struct{})
	go func() {
		h.ForwardStream(c, c.Writer, cancel, data, errs, opts)
		close(done)
	}()

	select {
	case err := <-cancelCh:
		if err != nil {
			t.Fatalf("expected cancel error to be nil, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancel")
	}

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ForwardStream to return")
	}

	if got := state.Snapshot().LastError; got != "" {
		t.Fatalf("expected LastError empty, got %q", got)
	}
}

func TestForwardStream_TerminalErrorWithoutErrorUsesStatusText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	state := metricsruntime.NewRequestState(true, "gpt-5.2")
	metricsruntime.AttachRequestState(c, state)

	h := &BaseAPIHandler{}
	data := make(chan []byte)
	errs := make(chan *interfaces.ErrorMessage, 1)

	cancelCh := make(chan error, 1)
	cancel := func(err error) {
		select {
		case cancelCh <- err:
		default:
		}
	}

	zero := time.Duration(0)
	opts := StreamForwardOptions{KeepAliveInterval: &zero}

	done := make(chan struct{})
	go func() {
		h.ForwardStream(c, c.Writer, cancel, data, errs, opts)
		close(done)
	}()

	errs <- &interfaces.ErrorMessage{StatusCode: 500, Error: nil}

	select {
	case <-cancelCh:
		// cancel error may be nil in this path; we only care about persistence signal.
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancel")
	}

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ForwardStream to return")
	}

	if got := state.Snapshot().LastError; got != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("expected LastError=%q, got %q", http.StatusText(http.StatusInternalServerError), got)
	}
}
