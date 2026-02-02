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

	// maxSSEBufferBytes caps the per-request buffer used to parse SSE frames that may be
	// split across TCP/HTTP chunk boundaries.
	maxSSEBufferBytes = 64 * 1024
)

type RequestState struct {
	mu sync.RWMutex

	TrackingID string
	StartedAt  time.Time
	// FirstContentTokenAt records when the first user-visible content token is observed
	// in a streaming response (not merely a metadata/role chunk or keep-alive).
	FirstContentTokenAt *time.Time
	// LastContentTokenAt records when the most recent user-visible content token is observed.
	// This is used to compute streaming TPS/TPOT using the observed content output window.
	LastContentTokenAt *time.Time
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
	WindowStats  RequestWindowStats
	ErrorsTotal  int

	LastError string

	// sseBuf buffers partial SSE frames across chunk boundaries so we can
	// reliably detect first user-visible content.
	sseBuf []byte
	// sseLastEventName tracks the most recently observed SSE event name. Some
	// providers put the event type in the SSE `event:` line while `data:` only
	// carries a minimal JSON payload.
	sseLastEventName string

	firstTokenOnce sync.Once
}

// RequestWindowStats is the metrics_summary.window_stats payload.
// It represents per-provider/model/streaming sliding window averages.
// Avg fields are nil when the window is empty.
type RequestWindowStats struct {
	Count   int      `json:"count"`
	TPSAvg  *float64 `json:"tps_avg"`
	TTFTAvg *float64 `json:"ttft_avg"`
	TPOTAvg *float64 `json:"tpot_avg"`
}

type RequestStateSnapshot struct {
	TrackingID          string
	StartedAt           time.Time
	FirstContentTokenAt *time.Time
	LastContentTokenAt  *time.Time
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
	WindowStats  RequestWindowStats
	ErrorsTotal  int

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
		ErrorsTotal:        s.ErrorsTotal,
	}
	// window_stats uses nil pointers to express "no data" in metrics_summary JSON.
	snap.WindowStats.Count = s.WindowStats.Count
	if s.WindowStats.TPSAvg != nil {
		v := *s.WindowStats.TPSAvg
		snap.WindowStats.TPSAvg = &v
	}
	if s.WindowStats.TTFTAvg != nil {
		v := *s.WindowStats.TTFTAvg
		snap.WindowStats.TTFTAvg = &v
	}
	if s.WindowStats.TPOTAvg != nil {
		v := *s.WindowStats.TPOTAvg
		snap.WindowStats.TPOTAvg = &v
	}
	if s.FirstContentTokenAt != nil {
		t := *s.FirstContentTokenAt
		snap.FirstContentTokenAt = &t
	}
	if s.LastContentTokenAt != nil {
		t := *s.LastContentTokenAt
		snap.LastContentTokenAt = &t
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

func (s *RequestState) SetWindowStats(stats RequestWindowStats) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.WindowStats.Count = stats.Count
	s.WindowStats.TPSAvg = nil
	s.WindowStats.TTFTAvg = nil
	s.WindowStats.TPOTAvg = nil
	if stats.TPSAvg != nil {
		v := *stats.TPSAvg
		s.WindowStats.TPSAvg = &v
	}
	if stats.TTFTAvg != nil {
		v := *stats.TTFTAvg
		s.WindowStats.TTFTAvg = &v
	}
	if stats.TPOTAvg != nil {
		v := *stats.TPOTAvg
		s.WindowStats.TPOTAvg = &v
	}
	s.mu.Unlock()
}

func (s *RequestState) SetErrorsTotal(total int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.ErrorsTotal = total
	s.mu.Unlock()
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
	contentEvents := state.countContentTokenEvents(chunk)
	if contentEvents <= 0 {
		return
	}
	state.mu.Lock()
	state.ContentTokenChunks += contentEvents
	// Always keep the latest observed content token timestamp for rate calculations.
	t := now
	state.LastContentTokenAt = &t
	state.mu.Unlock()

	state.firstTokenOnce.Do(func() {
		first := now
		state.mu.Lock()
		state.FirstContentTokenAt = &first
		state.mu.Unlock()
	})
}

func (s *RequestState) countContentTokenEvents(chunk []byte) int {
	if s == nil {
		return 0
	}
	trimmed := bytes.TrimSpace(chunk)
	if len(trimmed) == 0 {
		return 0
	}

	// If this looks like SSE (or we are already buffering a partial SSE line),
	// use a stateful line-based parser so we can handle TCP/HTTP chunk splits.
	s.mu.Lock()
	useSSE := len(s.sseBuf) > 0 || looksLikeSSE(trimmed)
	if useSSE {
		count, rest, lastEventName := countSSEContentTokenEventsByLines(s.sseBuf, chunk, s.sseLastEventName)
		s.sseBuf = rest
		s.sseLastEventName = lastEventName
		s.mu.Unlock()
		return count
	}
	s.mu.Unlock()

	return countContentTokenEvents(chunk)
}

func countContentTokenEvents(chunk []byte) int {
	trimmed := bytes.TrimSpace(chunk)
	if len(trimmed) == 0 {
		return 0
	}
	// Filter SSE comment heartbeats / keep-alives (": ...").
	if len(trimmed) > 0 && trimmed[0] == ':' {
		return 0
	}
	if bytes.Equal(trimmed, []byte(": keep-alive")) {
		return 0
	}

	// SSE: stateless best-effort parsing for whole frames inside a chunk.
	if looksLikeSSE(trimmed) {
		count, _, _ := countSSEContentTokenEventsByLines(nil, chunk, "")
		return count
	}

	if jsonHasContentToken(trimmed) {
		return 1
	}
	return 0
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

func countSSEContentTokenEventsByLines(buf []byte, chunk []byte, lastEventName string) (count int, rest []byte, updatedEventName string) {
	updatedEventName = lastEventName
	if len(chunk) == 0 {
		return 0, buf, updatedEventName
	}

	combined := append(buf, chunk...)
	if len(combined) > maxSSEBufferBytes {
		combined = combined[len(combined)-maxSSEBufferBytes:]
	}

	// Common fast-path in this codebase: upstream/executor provides whole SSE lines
	// without a trailing newline (handlers may write "\n" separately). If there's no
	// newline in the buffered data, attempt to parse it as a single SSE line.
	if bytes.IndexByte(combined, '\n') < 0 {
		c, rest, updated := countSSEContentTokenEventsFromSingleLine(combined, updatedEventName)
		// If we successfully consumed it (rest empty), avoid buffering indefinitely.
		if len(rest) == 0 {
			return c, nil, updated
		}
		return 0, combined, updatedEventName
	}

	for {
		idx := bytes.IndexByte(combined, '\n')
		if idx < 0 {
			break
		}
		line := combined[:idx]
		combined = combined[idx+1:]

		line = bytes.TrimSuffix(line, []byte("\r"))
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		// SSE comments / keep-alives.
		if trimmed[0] == ':' {
			continue
		}
		if bytes.Equal(trimmed, []byte(": keep-alive")) {
			continue
		}
		if bytes.HasPrefix(trimmed, []byte("event:")) {
			updatedEventName = strings.TrimSpace(string(bytes.TrimPrefix(trimmed, []byte("event:"))))
			continue
		}
		if !bytes.HasPrefix(trimmed, []byte("data:")) {
			continue
		}

		payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
		if len(payload) == 0 {
			continue
		}
		if bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if jsonHasContentToken(payload) {
			count++
			continue
		}
		// Non-JSON data payload; treat as content.
		if payload[0] != '{' && payload[0] != '[' {
			count++
			continue
		}
		// If `event:` carries the semantic type and `data:` omits it, fall back to the last seen event name.
		if strings.TrimSpace(updatedEventName) != "" && sseEventHasContentToken(updatedEventName, payload) {
			count++
			continue
		}
	}

	return count, combined, updatedEventName
}

func countSSEContentTokenEventsFromSingleLine(line []byte, lastEventName string) (count int, rest []byte, updatedEventName string) {
	updatedEventName = lastEventName
	trimmed := bytes.TrimSpace(bytes.TrimSuffix(line, []byte("\r")))
	if len(trimmed) == 0 {
		return 0, nil, updatedEventName
	}
	// Comments / keep-alives.
	if trimmed[0] == ':' {
		return 0, nil, updatedEventName
	}
	if bytes.Equal(trimmed, []byte(": keep-alive")) {
		return 0, nil, updatedEventName
	}
	if bytes.HasPrefix(trimmed, []byte("event:")) {
		updatedEventName = strings.TrimSpace(string(bytes.TrimPrefix(trimmed, []byte("event:"))))
		return 0, nil, updatedEventName
	}
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		// Not an SSE data line; keep buffering.
		return 0, line, updatedEventName
	}

	payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
	if len(payload) == 0 {
		return 0, nil, updatedEventName
	}
	if bytes.Equal(payload, []byte("[DONE]")) {
		return 0, nil, updatedEventName
	}
	if jsonHasContentToken(payload) {
		return 1, nil, updatedEventName
	}
	// Non-JSON payload: treat as content.
	if payload[0] != '{' && payload[0] != '[' {
		return 1, nil, updatedEventName
	}
	if strings.TrimSpace(updatedEventName) != "" && sseEventHasContentToken(updatedEventName, payload) {
		return 1, nil, updatedEventName
	}

	// Valid JSON but doesn't look like a content token event.
	if gjson.ValidBytes(payload) {
		return 0, nil, updatedEventName
	}
	// JSON is incomplete -> buffer.
	return 0, line, updatedEventName
}

func sseEventHasContentToken(eventName string, payload []byte) bool {
	p := bytes.TrimSpace(payload)
	if len(p) == 0 {
		return false
	}
	if bytes.Equal(p, []byte("[DONE]")) {
		return false
	}

	// First, try the existing JSON-based detectors.
	if jsonHasContentToken(p) {
		return true
	}

	// Non-JSON data payload; treat as content.
	if p[0] != '{' && p[0] != '[' {
		return true
	}

	// If the event name carries the type and the payload omits it (common for Responses SSE),
	// interpret a minimal payload for known user-visible events.
	en := strings.TrimSpace(eventName)
	if en == "" {
		return false
	}
	root := gjson.ParseBytes(p)
	if !root.Exists() {
		return false
	}

	switch en {
	case "response.output_text.delta":
		if d := root.Get("delta"); d.Type == gjson.String && strings.TrimSpace(d.String()) != "" {
			return true
		}
	case "response.output_text.done":
		if txt := root.Get("text"); txt.Type == gjson.String && strings.TrimSpace(txt.String()) != "" {
			return true
		}
	case "response.content_part.added", "response.content_part.done":
		if strings.TrimSpace(root.Get("part.type").String()) == "output_text" {
			if txt := root.Get("part.text"); txt.Type == gjson.String && strings.TrimSpace(txt.String()) != "" {
				return true
			}
		}
	case "response.output_item.added", "response.output_item.done":
		if strings.TrimSpace(root.Get("item.type").String()) == "message" {
			contentArr := root.Get("item.content")
			if contentArr.Exists() && contentArr.IsArray() {
				has := false
				contentArr.ForEach(func(_, block gjson.Result) bool {
					bt := strings.TrimSpace(block.Get("type").String())
					if bt != "output_text" && bt != "text" {
						return true
					}
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
		}
	}

	return false
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

	// OpenAI Responses streaming: response.output_text.delta with string delta.
	// Example: {"type":"response.output_text.delta","delta":"hello"}
	if typ := strings.TrimSpace(root.Get("type").String()); typ == "response.output_text.delta" {
		if d := root.Get("delta"); d.Type == gjson.String && strings.TrimSpace(d.String()) != "" {
			return true
		}
	}
	// Some implementations may emit a done event carrying the final text.
	if typ := strings.TrimSpace(root.Get("type").String()); typ == "response.output_text.done" {
		if txt := root.Get("text"); txt.Type == gjson.String && strings.TrimSpace(txt.String()) != "" {
			return true
		}
	}
	// OpenAI Responses content parts: response.content_part.added/done for output_text.
	if typ := strings.TrimSpace(root.Get("type").String()); typ == "response.content_part.added" || typ == "response.content_part.done" {
		if strings.TrimSpace(root.Get("part.type").String()) == "output_text" {
			if txt := root.Get("part.text"); txt.Type == gjson.String && strings.TrimSpace(txt.String()) != "" {
				return true
			}
		}
	}
	// OpenAI Responses items: response.output_item.added/done; message items carry content blocks.
	if typ := strings.TrimSpace(root.Get("type").String()); typ == "response.output_item.added" || typ == "response.output_item.done" {
		if strings.TrimSpace(root.Get("item.type").String()) == "message" {
			contentArr := root.Get("item.content")
			if contentArr.Exists() && contentArr.IsArray() {
				has := false
				contentArr.ForEach(func(_, block gjson.Result) bool {
					bt := strings.TrimSpace(block.Get("type").String())
					if bt != "output_text" && bt != "text" {
						return true
					}
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
		}
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
	// Scheme B: treat non-user-visible deltas (thinking/tool-related) as content too, so TTFT
	// is stable for tool/thinking-heavy models (e.g. kimi-for-coding).
	if typ := strings.TrimSpace(root.Get("type").String()); typ == "content_block_delta" {
		dt := strings.TrimSpace(root.Get("delta.type").String())
		// Known thinking delta shape: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"..."}}
		if dt == "thinking_delta" {
			if th := root.Get("delta.thinking"); th.Type == gjson.String && strings.TrimSpace(th.String()) != "" {
				return true
			}
		}
		// Generic fallback for future delta types: if the delta object contains any non-empty string
		// field (excluding the discriminator "type"), count it as generated content.
		if d := root.Get("delta"); d.Exists() {
			has := false
			d.ForEach(func(k, v gjson.Result) bool {
				if strings.TrimSpace(k.String()) == "type" {
					return true
				}
				if v.Type == gjson.String && strings.TrimSpace(v.String()) != "" {
					has = true
					return false
				}
				return true
			})
			if has {
				return true
			}
		}
	}
	// Anthropic tool streaming: tool_use args arrive as input_json_delta.
	// Treat non-empty partial_json as generated output so we can compute TTFT/TPS/TPOT
	// for tool-heavy models (e.g. kimi-for-coding).
	if typ := strings.TrimSpace(root.Get("type").String()); typ == "content_block_delta" {
		if dt := strings.TrimSpace(root.Get("delta.type").String()); dt == "input_json_delta" {
			if pj := root.Get("delta.partial_json"); pj.Type == gjson.String && strings.TrimSpace(pj.String()) != "" {
				return true
			}
		}
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
