package metricsruntime

import (
	"bytes"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
)

type RequestState struct {
	mu sync.RWMutex

	TrackingID     string
	StartedAt      time.Time
	FirstTokenAt   *time.Time
	Streaming      bool
	RequestedModel string
	Provider       string
	Model          string

	RequestPath string
	StatusCode  int

	InputTokens  *int
	OutputTokens *int
	Metrics      *metrics.RequestMetrics

	LastError string

	firstTokenOnce sync.Once
}

type RequestStateSnapshot struct {
	TrackingID     string
	StartedAt      time.Time
	FirstTokenAt   *time.Time
	Streaming      bool
	RequestedModel string
	Provider       string
	Model          string

	RequestPath string
	StatusCode  int

	InputTokens  *int
	OutputTokens *int
	Metrics      *metrics.RequestMetrics

	LastError string
}

func NewRequestState(streaming bool, requestedModel string) *RequestState {
	now := time.Now()
	return &RequestState{
		TrackingID:     uuid.NewString(),
		StartedAt:      now,
		Streaming:      streaming,
		RequestedModel: requestedModel,
		Model:          requestedModel,
	}
}

func (s *RequestState) Snapshot() RequestStateSnapshot {
	if s == nil {
		return RequestStateSnapshot{}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := RequestStateSnapshot{
		TrackingID:     s.TrackingID,
		StartedAt:      s.StartedAt,
		Streaming:      s.Streaming,
		RequestedModel: s.RequestedModel,
		Provider:       s.Provider,
		Model:          s.Model,
		RequestPath:    s.RequestPath,
		StatusCode:     s.StatusCode,
		LastError:      s.LastError,
	}
	if s.FirstTokenAt != nil {
		t := *s.FirstTokenAt
		snap.FirstTokenAt = &t
	}
	if s.InputTokens != nil {
		v := *s.InputTokens
		snap.InputTokens = &v
	}
	if s.OutputTokens != nil {
		v := *s.OutputTokens
		snap.OutputTokens = &v
	}
	if s.Metrics != nil {
		m := *s.Metrics
		snap.Metrics = &m
	}
	return snap
}

func (s *RequestState) SetProvider(provider string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.Provider = provider
	s.mu.Unlock()
}

func (s *RequestState) SetModel(model string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.Model = model
	s.mu.Unlock()
}

func (s *RequestState) SetRequestPath(path string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.RequestPath = path
	s.mu.Unlock()
}

func (s *RequestState) SetStatusCode(code int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.StatusCode = code
	s.mu.Unlock()
}

func (s *RequestState) SetLastError(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if err == nil {
		s.LastError = ""
	} else {
		s.LastError = err.Error()
	}
	s.mu.Unlock()
}

func (s *RequestState) SetInputTokens(tokens int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	v := tokens
	s.InputTokens = &v
	s.mu.Unlock()
}

func (s *RequestState) SetOutputTokens(tokens int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	v := tokens
	s.OutputTokens = &v
	s.mu.Unlock()
}

func (s *RequestState) SetMetrics(m *metrics.RequestMetrics) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if m == nil {
		s.Metrics = nil
		s.mu.Unlock()
		return
	}
	copied := *m
	s.Metrics = &copied
	s.mu.Unlock()
}

const requestStateContextKey = "metricsruntime.request_state"

func AttachRequestState(c *gin.Context, state *RequestState) {
	if c == nil || state == nil {
		return
	}
	if state.TrackingID == "" {
		state.TrackingID = uuid.NewString()
	}
	if state.StartedAt.IsZero() {
		state.StartedAt = time.Now()
	}
	c.Set(requestStateContextKey, state)
}

func GetRequestState(c *gin.Context) (*RequestState, bool) {
	if c == nil {
		return nil, false
	}
	v, ok := c.Get(requestStateContextKey)
	if !ok {
		return nil, false
	}
	state, ok := v.(*RequestState)
	return state, ok
}

func MaybeRecordFirstToken(c *gin.Context, chunk []byte, now time.Time) {
	state, ok := GetRequestState(c)
	if !ok || state == nil {
		return
	}
	if !state.Streaming {
		return
	}
	if now.IsZero() {
		return
	}

	trimmed := bytes.TrimSpace(chunk)
	if len(trimmed) == 0 {
		return
	}
	// Filter the default SSE keep-alive comment heartbeat.
	if bytes.Equal(trimmed, []byte(": keep-alive")) {
		return
	}

	state.firstTokenOnce.Do(func() {
		t := now
		state.mu.Lock()
		state.FirstTokenAt = &t
		state.mu.Unlock()
	})
}
