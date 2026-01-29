package metricsruntime

import (
	"bytes"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RequestState struct {
	TrackingID      string
	StartedAt       time.Time
	FirstTokenAt    *time.Time
	Streaming       bool
	RequestedModel  string
	Provider        string
	Model           string

	firstTokenOnce sync.Once
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
		state.FirstTokenAt = &t
	})
}
