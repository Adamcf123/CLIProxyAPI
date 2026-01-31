package metricsruntime

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/metrics"
	"github.com/tidwall/gjson"
)

const (
	// statusClientClosedRequest uses the de-facto nginx convention for "client closed request".
	// We use this as a durable persistence signal for canceled/disconnected requests.
	statusClientClosedRequest = 499
	statusInternalServerError = 500
	statusGatewayTimeout      = 504
)

type RequestState struct {
	mu sync.RWMutex

	TrackingID string
	StartedAt  time.Time
	// FirstContentTokenAt records when the first user-visible content token is observed
	// in a streaming response (not merely a metadata/role chunk or keep-alive).
	FirstContentTokenAt *time.Time
	// ContentTokenChunks counts how many chunks contained user-visible content.
	// This is used as a confidence signal for TPS/TPOT on streaming responses.
	ContentTokenChunks int
	Streaming          bool
	RequestedModel     string
	Provider           string
	Model              string

	RequestPath string
	StatusCode  int

	InputTokens  *int
	OutputTokens *int
	Metrics      *metrics.RequestMetrics

	LastError string

	firstTokenOnce sync.Once
}

type RequestStateSnapshot struct {
	TrackingID          string
	StartedAt           time.Time
	FirstContentTokenAt *time.Time
	ContentTokenChunks  int
	Streaming           bool
	RequestedModel      string
	Provider            string
	Model               string

	RequestPath string
	StatusCode  int

	InputTokens  *int
	OutputTokens *int
	Metrics      *metrics.RequestMetrics

	LastError string
}

func (s RequestStateSnapshot) IsClientCanceled() bool {
	return s.StatusCode == statusClientClosedRequest
}

func (s RequestStateSnapshot) IsFailure() bool {
	return s.StatusCode >= 400 || s.LastError != ""
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
		TrackingID:         s.TrackingID,
		StartedAt:          s.StartedAt,
		Streaming:          s.Streaming,
		RequestedModel:     s.RequestedModel,
		Provider:           s.Provider,
		Model:              s.Model,
		RequestPath:        s.RequestPath,
		StatusCode:         s.StatusCode,
		LastError:          s.LastError,
		ContentTokenChunks: s.ContentTokenChunks,
	}
	if s.FirstContentTokenAt != nil {
		t := *s.FirstContentTokenAt
		snap.FirstContentTokenAt = &t
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
	// Priority: failure > canceled > success.
	//
	// Once we mark a request as client-canceled (499), we must not let handler tail
	// code (often `state.SetStatusCode(c.Writer.Status())`) overwrite it back into
	// a <400 status. Explicit failures (>= 400) are still allowed to override.
	if s.StatusCode == statusClientClosedRequest {
		if code > 0 && code < 400 {
			s.mu.Unlock()
			return
		}
	}
	if s.StatusCode >= 400 && s.StatusCode != statusClientClosedRequest {
		s.mu.Unlock()
		return
	}
	if s.LastError != "" {
		// Failure already established via error_info; don't allow a later non-failure
		// status to weaken it.
		if code >= 400 {
			s.StatusCode = code
		}
		s.mu.Unlock()
		return
	}
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
		// Ensure statusClientClosedRequest is ONLY used for canceled.
		// Any explicit error upgrades the outcome to failure.
		if s.StatusCode < 400 || s.StatusCode == statusClientClosedRequest {
			if errors.Is(err, context.DeadlineExceeded) {
				s.StatusCode = statusGatewayTimeout
			} else {
				s.StatusCode = statusInternalServerError
			}
		}
	}
	s.mu.Unlock()
}

// MarkClientCanceled marks the request as canceled/disconnected.
// This MUST NOT write error_info (LastError) and MUST NOT override an explicit failure.
func (s *RequestState) MarkClientCanceled() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.StatusCode >= 400 {
		return
	}
	if s.LastError != "" {
		return
	}
	s.StatusCode = statusClientClosedRequest
}

func (s *RequestState) IsClientCanceled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.StatusCode == statusClientClosedRequest
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
	// Align stderr metrics_summary.tracking_id with SQLite metrics.request_id.
	// The request_id is generated and stored in Gin context by middleware; persistence uses the
	// same request_id from context. If we leave TrackingID as a random UUID, runtime evidence is
	// not correlatable across stderr and SQLite.
	if rid := logging.GetGinRequestID(c); rid != "" {
		state.TrackingID = rid
	} else if state.TrackingID == "" {
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

// MaybeRecordFirstContentToken records TTFT when the first content token is observed.
// "Content" here means user-visible generation output (e.g. delta.content / delta.text),
// not role/metadata chunks nor SSE keep-alives.
func MaybeRecordFirstContentToken(c *gin.Context, chunk []byte, now time.Time) {
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
	if !chunkHasContentToken(chunk) {
		return
	}
	state.mu.Lock()
	state.ContentTokenChunks++
	state.mu.Unlock()

	state.firstTokenOnce.Do(func() {
		t := now
		state.mu.Lock()
		state.FirstContentTokenAt = &t
		state.mu.Unlock()
	})
}

func chunkHasContentToken(chunk []byte) bool {
	trimmed := bytes.TrimSpace(chunk)
	if len(trimmed) == 0 {
		return false
	}
	// Filter SSE comment heartbeats / keep-alives (": ...").
	if len(trimmed) > 0 && trimmed[0] == ':' {
		return false
	}
	if bytes.Equal(trimmed, []byte(": keep-alive")) {
		return false
	}

	// SSE: parse data: lines and inspect payload.
	if looksLikeSSE(trimmed) {
		for _, payload := range extractSSEDataPayloads(trimmed) {
			p := bytes.TrimSpace(payload)
			if len(p) == 0 {
				continue
			}
			if bytes.Equal(p, []byte("[DONE]")) {
				continue
			}
			if jsonHasContentToken(p) {
				return true
			}
			// Non-JSON data payload; treat as content.
			if p[0] != '{' && p[0] != '[' {
				return true
			}
		}
		return false
	}

	return jsonHasContentToken(trimmed)
}

func looksLikeSSE(b []byte) bool {
	// Cheap sniffing for SSE frames.
	if bytes.HasPrefix(b, []byte("event:")) || bytes.HasPrefix(b, []byte("data:")) {
		return true
	}
	if bytes.Contains(b, []byte("\ndata:")) || bytes.Contains(b, []byte("\nevent:")) {
		return true
	}
	return false
}

func extractSSEDataPayloads(b []byte) [][]byte {
	// SSE allows multiple data: lines; each is part of the same event.
	// We treat each data: line as a candidate payload.
	lines := bytes.Split(b, []byte("\n"))
	var out [][]byte
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		out = append(out, payload)
	}
	return out
}

func jsonHasContentToken(payload []byte) bool {
	p := bytes.TrimSpace(payload)
	if len(p) == 0 {
		return false
	}
	if bytes.Equal(p, []byte("[DONE]")) {
		return false
	}
	if p[0] != '{' && p[0] != '[' {
		return false
	}

	root := gjson.ParseBytes(p)
	if !root.Exists() {
		return false
	}

	// OpenAI streaming: choices[].delta.content or choices[].text
	choices := root.Get("choices")
	if choices.Exists() && choices.IsArray() {
		has := false
		choices.ForEach(func(_, choice gjson.Result) bool {
			if d := choice.Get("delta"); d.Exists() {
				if c := d.Get("content"); c.Type == gjson.String && strings.TrimSpace(c.String()) != "" {
					has = true
					return false
				}
				if t := d.Get("text"); t.Type == gjson.String && strings.TrimSpace(t.String()) != "" {
					has = true
					return false
				}
			}
			if t := choice.Get("text"); t.Type == gjson.String && strings.TrimSpace(t.String()) != "" {
				has = true
				return false
			}
			if m := choice.Get("message"); m.Exists() {
				if c := m.Get("content"); c.Type == gjson.String && strings.TrimSpace(c.String()) != "" {
					has = true
					return false
				}
			}
			return true
		})
		if has {
			return true
		}
	}

	// Anthropic streaming: delta.text or content_block.text
	if t := root.Get("delta.text"); t.Type == gjson.String && strings.TrimSpace(t.String()) != "" {
		return true
	}
	if t := root.Get("content_block.text"); t.Type == gjson.String && strings.TrimSpace(t.String()) != "" {
		return true
	}
	// Legacy-style completions.
	if t := root.Get("completion"); t.Type == gjson.String && strings.TrimSpace(t.String()) != "" {
		return true
	}

	// Generic fallbacks (string fields only).
	if t := root.Get("text"); t.Type == gjson.String && strings.TrimSpace(t.String()) != "" {
		return true
	}
	if c := root.Get("content"); c.Type == gjson.String && strings.TrimSpace(c.String()) != "" {
		return true
	}

	// content as array of blocks: [{"type":"text","text":"..."}, ...]
	contentArr := root.Get("content")
	if contentArr.Exists() && contentArr.IsArray() {
		has := false
		contentArr.ForEach(func(_, block gjson.Result) bool {
			if t := block.Get("text"); t.Type == gjson.String && strings.TrimSpace(t.String()) != "" {
				has = true
				return false
			}
			return true
		})
		if has {
			return true
		}
	}

	return false
}
