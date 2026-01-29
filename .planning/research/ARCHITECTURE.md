# Architecture Patterns

**Domain:** API Proxy TPS (Tokens Per Second) Metrics
**Researched:** 2026-01-29
**Overall confidence:** HIGH

## Executive Summary

TPS metrics for API proxies require architectural integration at the streaming response layer. Unlike traditional request latency metrics (which measure wall-clock time), TPS metrics require correlating **token count** with **time duration** during active streaming. The existing codebase's layered architecture (API → Handler → Auth → Translation → Executor) with a plugin-based usage system provides the foundation for TPS collection without impacting response latency.

## Recommended Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           API Layer (Gin)                                │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  Request Middleware (TPS Tracking Init)                            │ │
│  │  - Capture request_start_time                                      │ │
│  │  - Generate request_id for correlation                             │ │
│  │  - Attach tps_tracker to context                                   │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        Handler Layer                                     │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  BaseAPIHandler.ExecuteStreamWithAuthManager                       │ │
│  │  - Receives <-chan []byte (streaming chunks)                       │ │
│  │  - Wraps data channel with TPSCollector                            │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  ForwardStream (Modified)                                          │ │
│  │  - For each chunk: TPSCollector.Record(chunk)                      │ │
│  │  - Extract token count from chunk (parse SSE/payload)              │ │
│  │  - Calculate: tokens / (now - request_start_time)                  │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                       Usage Plugin Layer (New)                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  TPSMetricsPlugin (implements coreusage.Plugin)                    │ │
│  │  HandleUsage(ctx, record) {                                        │ │
│  │    - Extract tps_data from context                                 │ │
│  │    - Aggregate TPS by: model, provider, time_window                │ │
│  │    - Store in TPSSnapshotStore                                     │ │
│  │  }                                                                  │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    Management API (Extension)                           │
│  - GET /management/tps/snapshot - Current TPS metrics                  │
│  - GET /management/tps/by-model/{model} - Per-model TPS                │
│  - GET /management/tps/by-provider/{provider} - Per-provider TPS       │
└──────────────────────────────────────────────────────────────────────────┘
```

## Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| **TPSMiddleware** (Gin) | - Initialize TPS tracking context<br/>- Capture request metadata<br/>- Generate tracking ID | Handler Layer, gin.Context |
| **TPSCollector** (struct) | - Count tokens from streaming chunks<br/>- Calculate incremental TPS<br/>- Track time windows | Handler ForwardStream, Usage Plugin |
| **TokenCounter** (interface) | - Parse SSE chunks for token counts<br/>- Handle provider-specific formats | TPSCollector |
| **TPSMetricsPlugin** (usage.Plugin) | - Receive completed usage records<br/>- Aggregate TPS data<br/>- Maintain time-windowed snapshots | Usage Manager, TPSSnapshotStore |
| **TPSSnapshotStore** (struct) | - Thread-safe TPS aggregation<br/>- Sliding window storage<br/>- Query operations | TPSMetricsPlugin, Management API |
| **ManagementAPI** (handlers) | - Expose TPS metrics endpoints<br/>- Return JSON snapshots | TPSSnapshotStore, HTTP clients |

## Data Flow

### Request Lifecycle (Streaming)

```
1. Client Request
   ↓
2. TPSMiddleware (gin)
   - tps_tracker := NewTPSTracker(request_id, model, provider)
   - ctx.Set("tps_tracker", tps_tracker)
   ↓
3. Handler.ExecuteStreamWithAuthManager(ctx, ...)
   - Creates streaming request to upstream
   - Returns <-chan []byte data channel
   ↓
4. Handler.ForwardStream(data, errs, opts)
   - For each chunk:
     a. token_count := TokenCounter.Count(chunk)
     b. tps_tracker.Record(token_count, time.Now())
     c. Forward chunk to client (non-blocking)
   ↓
5. Stream Complete
   - tps_tracker.Finalize()
   - Usage record emitted with TPS data attached
   ↓
6. TPSMetricsPlugin.HandleUsage(ctx, record)
   - Extract tps_data from context
   - Aggregate by model/provider/time-window
   - Update TPSSnapshotStore
```

### TPS Calculation Formula

```
TPS (instantaneous) = delta_tokens / delta_time
TPS (session)      = total_tokens / (end_time - start_time)
TPS (windowed)     = total_tokens_in_window / window_duration

Where:
- delta_tokens: tokens since last measurement
- delta_time: elapsed time since last measurement
- window: fixed time bucket (e.g., 1s, 10s, 60s)
```

## Patterns to Follow

### Pattern 1: Non-Blocking TPS Collection

**What:** TPS tracking must not add latency to streaming responses. All counting happens asynchronously to the client response path.

**When:** Always - blocking token counting would defeat the purpose of TPS measurement by itself becoming the bottleneck.

**Example:**
```go
type TPSCollector struct {
    mu         sync.Mutex
    tokenCount int64
    startTime  time.Time
    lastTime   time.Time
    lastCount  int64
}

func (t *TPSCollector) Record(tokens int64, ts time.Time) {
    t.mu.Lock()
    defer t.mu.Unlock()

    t.tokenCount += tokens
    t.lastTime = ts
    t.lastCount = tokens
}

// CalculateTPS runs after stream completes, not during
func (t *TPSCollector) CalculateTPS() float64 {
    t.mu.Lock()
    defer t.mu.Unlock()

    duration := t.lastTime.Sub(t.startTime).Seconds()
    if duration == 0 {
        return 0
    }
    return float64(t.tokenCount) / duration
}
```

### Pattern 2: Usage Plugin Extension

**What:** Leverage the existing `coreusage.Plugin` interface (`HandleUsage(ctx, record)`) to receive completed usage records and aggregate TPS data.

**When:** Integrating with the existing usage tracking system without modifying core executor code.

**Example:**
```go
type TPSMetricsPlugin struct {
    store *TPSSnapshotStore
}

func (p *TPSMetricsPlugin) HandleUsage(ctx context.Context, record coreusage.Record) {
    // Extract TPS data from context (attached by middleware)
    tpsData := extractTPSFromContext(ctx)
    if tpsData == nil {
        return
    }

    // Calculate final TPS for the request
    tps := calculateFinalTPS(tpsData)

    // Aggregate by model and time window
    p.store.Aggregate(record.Model, record.Provider, tps, tpsData.Timestamp)
}
```

### Pattern 3: Sliding Window Aggregation

**What:** Maintain TPS metrics in rolling time windows (e.g., last 10s, 60s, 300s) to provide both real-time and historical views.

**When:** Exposing TPS metrics via management API - different windows serve different monitoring needs.

**Example:**
```go
type TPSSnapshotStore struct {
    mu     sync.RWMutex
    windows map[time.Duration]*TimeWindow
}

type TimeWindow struct {
    duration  time.Duration
    buckets   map[string]*Bucket  // key: "model:provider"
    bucketExp time.Time
}

func (s *TPSSnapshotStore) Aggregate(model, provider string, tps float64, ts time.Time) {
    s.mu.Lock()
    defer s.mu.Unlock()

    for _, window := range s.windows {
        if ts.After(window.bucketExp) {
            // Rotate bucket - start new time window
            window.rotateBucket(ts)
        }
        key := model + ":" + provider
        window.buckets[key].Add(tps)
    }
}
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Synchronous Token Parsing

**What:** Parsing SSE chunks to count tokens before forwarding to client, adding parsing latency to the hot path.

**Why bad:** Every microsecond of parsing latency directly impacts TTFT (Time to First Token), which is the most critical UX metric for streaming LLM responses.

**Instead:** Count tokens asynchronously in a separate goroutine, or parse after forwarding the chunk to the client.

```go
// BAD - blocks client response
chunk := <-data
tokenCount := ParseTokens(chunk)  // SLOW
tpsTracker.Record(tokenCount, time.Now())
c.Writer.Write(chunk)  // Delayed!

// GOOD - non-blocking
chunk := <-data
c.Writer.Write(chunk)  // Immediate!
go func(ch []byte) {
    tokenCount := ParseTokens(ch)
    tpsTracker.Record(tokenCount, time.Now())
}(chunk)
```

### Anti-Pattern 2: TPS Calculation in Hot Path

**What:** Calculating TPS (division operation) for every chunk or every N tokens during active streaming.

**Why bad:** Unnecessary CPU work that doesn't benefit the client. TPS is an observability metric, not a request-processing requirement.

**Instead:** Accumulate raw data (token count, timestamp) during streaming, calculate TPS after stream completes or in a background aggregator.

### Anti-Pattern 3: Global Lock for TPS Aggregation

**What:** Using a single global mutex to protect all TPS metrics aggregation across all requests.

**Why bad:** Creates contention at scale - concurrent streaming requests will contend on the lock, potentially slowing responses.

**Instead:** Use sharded locks (one per model or provider) or lock-free data structures (sync/atomic for counters, channel-based aggregation).

### Anti-Pattern 4: TPS as Request-Level Metric Only

**What:** Only tracking per-request TPS, not aggregating across time windows.

**Why bad:** Per-request TPS doesn't answer operational questions like "Is this model degrading?" or "Are we hitting provider rate limits?"

**Instead:** Maintain both per-request TPS (for debugging) and time-windowed TPS (for monitoring/alerting).

## Scalability Considerations

| Concern | At 100 users | At 10K users | At 1M users |
|---------|--------------|--------------|-------------|
| **Memory per request** | ~512 bytes (tracker) | ~512 bytes (tracker) | ~512 bytes (tracker) |
| **Aggregation store** | In-memory map (KB) | In-memory map (MB) | External metrics store (Prometheus/Redis) |
| **Lock contention** | Single global lock OK | Sharded locks per model | Fully sharded or lock-free |
| **Token parsing** | Inline async OK | Worker pool | Dedicated token-counting service |

### Build Order Implications

**Phase 1: Foundation (independent, can be built in parallel)**
1. **TokenCounter interface** - Parse chunks for token counts (provider-specific)
2. **TPSCollector struct** - Accumulate token/timestamp data
3. **TPSMiddleware** - Initialize context with tracker

**Phase 2: Integration (depends on Phase 1)**
4. **Modify ForwardStream** - Wire in TPSCollector (requires Phase 1)
5. **TPSMetricsPlugin** - Implement usage.Plugin (requires existing usage system)

**Phase 3: Storage & API (depends on Phase 2)**
6. **TPSSnapshotStore** - Aggregate metrics (requires Phase 2 plugin data)
7. **Management API endpoints** - Expose metrics (requires Phase 3 store)

**Rationale:**
- Phase 1 establishes the data structures and interfaces without modifying existing code paths
- Phase 2 integrates with the existing streaming and usage infrastructure
- Phase 3 adds observability and query capabilities on top of collected data

## Integration Points with Existing Architecture

### 1. Handler Layer (`sdk/api/handlers/handlers.go`)

The `ForwardStream` function is the primary integration point for TPS collection:

```go
// Existing function signature
func (h *BaseAPIHandler) ForwardStream(
    c *gin.Context,
    flusher http.Flusher,
    cancel func(error),
    data <-chan []byte,
    errs <-chan *interfaces.ErrorMessage,
    opts StreamForwardOptions,
)
```

**Integration approach:** Wrap `opts.WriteChunk` to capture token counts before forwarding.

### 2. Usage Manager (`sdk/cliproxy/usage/manager.go`)

The existing `Plugin` interface provides the hook for TPS aggregation:

```go
type Plugin interface {
    HandleUsage(ctx context.Context, record Record)
}
```

**Integration approach:** Register `TPSMetricsPlugin` alongside existing `LoggerPlugin`.

### 3. Management API (`internal/api/handlers/management/`)

Extend existing management endpoints to include TPS metrics alongside usage statistics.

## Sources

### Codebase Analysis (HIGH confidence)
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/handlers.go` - Handler layer architecture
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go` - Streaming response handling
- `/home/adam/projects/CLIProxyAPI/sdk/cliproxy/usage/manager.go` - Usage plugin system
- `/home/adam/projects/CLIProxyAPI/internal/usage/logger_plugin.go` - Existing usage tracking
- `/home/adam/projects/CLIProxyAPI/internal/api/handlers/management/usage.go` - Management API patterns

### External References (MEDIUM confidence - verified with official sources)
- [vLLM Metrics Documentation](https://docs.vllm.ai/en/latest/design/metrics/) - "prompt tokens processed per second" and "new tokens generated per second" metrics
- [gRPC in Go: Streaming RPCs, Interceptors, and Metadata (VictoriaMetrics, 2025)](https://victoriametrics.com/blog/go-grpc-basic-streaming-interceptor/) - Go streaming and interceptor patterns
- [Understand LLM Latency and Throughput Metrics (Anyscale)](https://docs.anyscale.com/llm/serving/benchmarking/metrics) - Time to first token, inter-token latency, throughput metrics
- [Beyond Tokens-per-Second: How to Balance Speed, Cost and Quality in LLM Inference (BentoML, 2026)](https://www.bentoml.com/blog/beyond-tokens-per-second-how-to-balance-speed-cost-and-quality-in-llm-inference) - TPS evaluation in streaming contexts
- [Best Practices for LLM Latency Benchmarking (newline.co, 2025)](https://www.newline.co/@zaoyang/best-practices-for-llm-latency-benchmarking--257f132d) - Monitoring tokens per second implementation

### Community Patterns (LOW confidence - single sources, needs validation)
- [Kong AI Gateway resource sizing guidelines](https://developer.konghq.com/ai-gateway/resource-sizing-guidelines-ai/) - AI inference performance depends on token streaming latency and throughput
- [How API Gateways Proxy LLM Requests (API7, 2025)](https://api7.ai/learning-center/api-gateway-guide/api-gateway-proxy-llm-requests) - API gateway flow control and rate limiting
- [How to get token usage for each openAI ChatCompletion API call in streaming mode (StackOverflow)](https://stackoverflow.com/questions/75824798/how-to-get-token-usage-for-each-openai-chatcompletion-api-call-in-streaming-mode) - Token usage in streaming mode

### Gaps Requiring Phase-Specific Research
1. **Token counting accuracy:** How to reliably count tokens from SSE chunks across different providers (OpenAI, Anthropic, Gemini, etc.)? May need provider-specific parsers.
2. **Compression impact:** Do compressed responses affect token counting accuracy? The existing codebase handles gzip decompression in proxy layer.
3. **TPS calculation window size:** What time windows (1s, 10s, 60s) are most useful for operational monitoring? User requirements may vary.
