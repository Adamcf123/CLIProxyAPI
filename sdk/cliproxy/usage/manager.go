package usage

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Record contains the usage statistics captured for a single provider request.
type Record struct {
	Provider    string
	Model       string
	APIKey      string
	AuthID      string
	AuthIndex   string
	Source      string
	RequestedAt time.Time
	Failed      bool
	Detail      Detail
}

// Detail holds the token usage breakdown.
type Detail struct {
	InputTokens     int64
	OutputTokens    int64
	ReasoningTokens int64
	CachedTokens    int64
	TotalTokens     int64
}

// Plugin consumes usage records emitted by the proxy runtime.
type Plugin interface {
	HandleUsage(ctx context.Context, record Record)
}

// Manager delivers usage records to registered plugins synchronously.
// Synchronous dispatch ensures that RequestState metrics (e.g. WindowStatsE2E)
// are updated before the handler's defer fires PrintSummary.
type Manager struct {
	pluginsMu sync.RWMutex
	plugins   []Plugin
}

// NewManager constructs a Manager. The buffer parameter is kept for API
// compatibility but is unused (dispatch is now synchronous).
func NewManager(_ int) *Manager {
	return &Manager{}
}

// Start is a no-op kept for API compatibility.
func (m *Manager) Start(_ context.Context) {}

// Stop is a no-op kept for API compatibility.
func (m *Manager) Stop() {}

// Register appends a plugin to the delivery list.
func (m *Manager) Register(plugin Plugin) {
	if m == nil || plugin == nil {
		return
	}
	m.pluginsMu.Lock()
	m.plugins = append(m.plugins, plugin)
	m.pluginsMu.Unlock()
}

// Publish dispatches a usage record to all registered plugins synchronously.
func (m *Manager) Publish(ctx context.Context, record Record) {
	if m == nil {
		return
	}
	m.pluginsMu.RLock()
	plugins := make([]Plugin, len(m.plugins))
	copy(plugins, m.plugins)
	m.pluginsMu.RUnlock()
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		safeInvoke(plugin, ctx, record)
	}
}

func safeInvoke(plugin Plugin, ctx context.Context, record Record) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("usage: plugin panic recovered: %v", r)
		}
	}()
	plugin.HandleUsage(ctx, record)
}

var defaultManager = NewManager(0)

// DefaultManager returns the global usage manager instance.
func DefaultManager() *Manager { return defaultManager }

// RegisterPlugin registers a plugin on the default manager.
func RegisterPlugin(plugin Plugin) { DefaultManager().Register(plugin) }

// PublishRecord publishes a record using the default manager.
func PublishRecord(ctx context.Context, record Record) { DefaultManager().Publish(ctx, record) }

// StartDefault is a no-op kept for API compatibility.
func StartDefault(_ context.Context) {}

// StopDefault is a no-op kept for API compatibility.
func StopDefault() {}
