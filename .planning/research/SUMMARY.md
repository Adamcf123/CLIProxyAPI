# Project Research Summary

**Project:** CLIProxyAPI TPS Metrics
**Domain:** API Performance Monitoring - TPS Metrics for AI API Proxy
**Researched:** 2026-01-29
**Confidence:** HIGH

## Executive Summary

This project adds TPS (Tokens Per Second) metrics collection to an existing Go-based AI API proxy that handles OpenAI, Anthropic, and Gemini streaming responses. Unlike traditional request latency metrics, TPS requires correlating token count with time duration during active streaming. The recommended approach integrates metrics collection at the streaming response layer using non-blocking patterns to preserve performance, coupled with an embedded SQLite database for persistence.

Key risks include cardinality explosion in metric labels (which can crash Prometheus), blocking metrics collection that degrades streaming latency, and token counting inconsistencies across providers. Mitigation strategies include a whitelist-based label policy, async channel-based collection, and using provider-reported token counts where available. The existing codebase's layered architecture with a plugin-based usage system provides a clean integration path without modifying core executor code.

## Key Findings

### Recommended Stack

**Summary:** Modern Go-friendly stack with embedded database, native Prometheus instrumentation, and SSE for real-time streaming. Prioritizes simplicity over abstraction layers.

**Core technologies:**
- **modernc.org/sqlite v1.44.3** — Embedded metrics storage, CGo-free for cross-compilation
- **Prometheus Go Client v1.23.2** — Native metrics instrumentation, simpler than OpenTelemetry
- **Server-Sent Events (SSE)** — Real-time metrics streaming, 25% fewer resources than WebSockets
- **tiktoken-go v0.7.0** — Token counting (already in project)
- **DuckDB v1.4.4** — Future analytics queries, 10-100x faster than SQLite for complex aggregations

### Expected Features

**Summary:** MVP focuses on core streaming metrics (TTFT, TPS, TPOT) with structured logging. Differentiators add real-time visibility and historical analysis.

**Must have (table stakes):**
- **Time to First Token (TTFT)** — Most critical UX metric for streaming AI responses
- **Tokens Per Second (TPS)** — Standard throughput metric for LLM APIs
- **Time Per Output Token (TPOT)** — Measures inter-token latency (streaming smoothness)
- **Total Token Count** — Required for cost calculation and usage tracking
- **Summary After Response** — Display metrics when response completes
- **Structured Logging** — JSON format for downstream processing
- **Error Rate Tracking** — 4xx/5xx error percentages separately
- **Request Latency** — End-to-end request/response time

**Should have (competitive):**
- **Real-Time TPS Display During Streaming** — Instant performance feedback during generation
- **Percentile Tracking (p50, p95, p99)** — Reveals tail latency outliers
- **Database Storage for Historical Trends** — Enables long-term analysis
- **Prometheus Export** — Integrates with existing observability stack
- **Multi-Protocol Metric Normalization** — Compare performance across providers uniformly

**Defer (v2+):**
- **Cost Attribution** — Requires pricing configuration
- **Anomaly Detection** — Requires baseline data
- **Geographic Monitoring** — High complexity, lower priority

### Architecture Approach

**Summary:** Layered integration with non-blocking collection at streaming response layer, usage plugin for aggregation, and sliding window store for time-series queries.

**Major components:**
1. **TPSMiddleware (Gin)** — Initialize tracking context, capture request metadata, generate tracking ID
2. **TPSCollector** — Count tokens from streaming chunks, calculate incremental TPS, track time windows
3. **TokenCounter (interface)** — Parse SSE chunks for token counts, handle provider-specific formats
4. **TPSMetricsPlugin (usage.Plugin)** — Receive completed usage records, aggregate TPS data, maintain snapshots
5. **TPSSnapshotStore** — Thread-safe TPS aggregation, sliding window storage, query operations
6. **ManagementAPI** — Expose TPS metrics endpoints, return JSON snapshots

### Critical Pitfalls

**Summary:** Cardinality explosion, blocking collection, and token counting inconsistencies are the top risks. All require architectural decisions before implementation.

1. **Cardinality Explosion from High-Cardinality Labels** — Use label whitelist approach, enforce cardinality limits, bucket high-cardinality dimensions (user tiers instead of user IDs), store high-cardinality data in logs/traces not metrics
2. **Blocking Metrics Collection in Streaming Hot Path** — Use non-blocking channel-based buffer, decouple streaming goroutine from metrics goroutine, batch metric updates, use atomic operations instead of mutexes
3. **Token Counting Inconsistency Across Providers** — Use provider-reported counts when available, normalize at collection time, handle missing usage metadata gracefully, document discrepancies between "provider-reported" and "tiktoken-estimated"
4. **TPS Calculation Missing First-Token Latency** — Report multiple TPS metrics: `ttft_ms`, `tps_inter_token`, `tps_overall`, separate initialization phase from generation phase in metrics
5. **Memory Leaks from Metrics Accumulation** — Use metric vectors at package scope (create once at startup), avoid per-request metrics, use expiring cache if maps are needed

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: Metrics Foundation
**Rationale:** Establish data structures and interfaces without modifying existing code paths. Cardinality strategy must be designed before any metrics are emitted.
**Delivers:** TokenCounter interface, TPSCollector struct, TPSMiddleware, label whitelist policy
**Addresses:** TTFT, TPS, TPOT, Total Token Count, Structured Logging (from FEATURES.md)
**Avoids:** Cardinality explosion (Pitfall 1), memory leaks (Pitfall 6)

### Phase 2: TPS Collection
**Rationale:** Integrate with existing streaming infrastructure. Non-blocking patterns must be designed from day one to avoid latency issues.
**Delivers:** Modified ForwardStream with TPSCollector integration, TPSMetricsPlugin implementation
**Uses:** Prometheus Go Client, tiktoken-go (from STACK.md)
**Implements:** TPSCollector, TokenCounter, TPSMetricsPlugin (from ARCHITECTURE.md)
**Avoids:** Blocking metrics in streaming (Pitfall 2), token counting inconsistency (Pitfall 3)

### Phase 3: Persistence & Querying
**Rationale:** Add storage and query capabilities on top of collected data. Requires SQLite schema design and indexing.
**Delivers:** TPSSnapshotStore with sliding windows, SQLite database with metrics_tps table, Management API endpoints
**Uses:** modernc.org/sqlite (from STACK.md)
**Implements:** TPSSnapshotStore, ManagementAPI (from ARCHITECTURE.md)
**Avoids:** TPS missing TTFT (Pitfall 4), parser fragility (Pitfall 5)

### Phase 4: Real-Time Visibility
**Rationale:** Add SSE streaming for real-time TPS display. This is a key differentiator that provides immediate user value.
**Delivers:** SSE endpoint for real-time TPS updates, throttled display mechanism
**Uses:** SSE (from STACK.md)
**Implements:** Real-Time TPS Display During Streaming (from FEATURES.md)
**Avoids:** Overly granular display updates (Anti-Feature from FEATURES.md)

### Phase 5: Advanced Analytics (Optional)
**Rationale:** Migrate to DuckDB if analytics needs grow. Not required for MVP.
**Delivers:** DuckDB integration, complex aggregations, percentile tracking
**Uses:** DuckDB (from STACK.md)
**Implements:** Percentile Tracking, Database Storage for Historical Trends (from FEATURES.md)

### Phase Ordering Rationale

- **Foundation first:** Phase 1 establishes interfaces and data structures without touching existing code paths, minimizing risk
- **Integration second:** Phase 2 connects TPS collection to existing streaming infrastructure using the usage plugin pattern
- **Storage third:** Phase 3 adds persistence and querying once collection is stable
- **Visibility fourth:** Phase 4 adds real-time display as a differentiator on top of stable metrics
- **Analytics fifth:** Phase 5 is optional and only needed if query complexity grows

This order avoids pitfalls by:
- Designing cardinality strategy before emitting metrics (Phase 1 avoids Pitfall 1)
- Using non-blocking patterns from day one (Phase 2 avoids Pitfall 2)
- Capturing TTFT from the start (Phase 2 avoids Pitfall 4)
- Using metric vectors at package scope (Phase 1 avoids Pitfall 6)

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2:** Token counting accuracy across providers (OpenAI, Anthropic, Gemini) — requires provider-specific parser research
- **Phase 3:** TPS calculation window size optimization — may need load testing to determine optimal windows
- **Phase 4:** SSE throttling strategy — requires empirical testing for optimal update frequency

Phases with standard patterns (skip research-phase):
- **Phase 1:** Well-documented Prometheus patterns, established Go middleware patterns
- **Phase 3:** SQLite time-series patterns documented, existing management API patterns in codebase
- **Phase 5:** DuckDB has comprehensive documentation for OLAP queries

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Official documentation for all major components, verified alternatives |
| Features | HIGH | Multiple authoritative sources (CloudZero, Langfuse, BentoML) |
| Architecture | HIGH | Codebase analysis + verified external patterns (vLLM, VictoriaMetrics) |
| Pitfalls | HIGH | Multiple sources agree on cardinality and blocking issues, Go 1.26 metrics |

**Overall confidence:** HIGH

### Gaps to Address

- **Go-specific streaming benchmarks:** Would benefit from load testing different metrics collection approaches in the actual codebase
- **Provider-specific token counting:** Each provider (OpenAI, Anthropic, Gemini) may have quirks not covered in general research
- **TPS calculation window size:** No strong sources on "correct" time windows (1s, 10s, 60s) — may need empirical testing based on user requirements
- **Compression impact:** Do compressed responses affect token counting accuracy? Existing codebase handles gzip decompression

## Sources

### Primary (HIGH confidence)
- [modernc.org/sqlite v1.44.3](https://pkg.go.dev/modernc.org/sqlite@v1.44.3) — CGo-free SQLite driver
- [Prometheus Go Client v1.23.2](https://github.com/prometheus/client_golang/tree/v1.23.2) — Official Prometheus client
- [The API Metrics Every SaaS Team Must Track In 2026](https://www.cloudzero.com/blog/api-metrics/) — CloudZero
- [Langfuse Token & Cost Tracking Documentation](https://langfuse.com/docs/observability/features/token-and-cost-tracking) — Langfuse
- [API Performance Monitoring—Key Metrics and Best Practices](https://www.catchpoint.com/api-monitoring-tools/api-performance-monitoring) — Catchpoint
- [Key metrics for LLM inference](https://bentoml.com/llm/inference-optimization/llm-inference-metrics) — BentoML
- [Go 1.26 Release Notes - Goroutine Metrics](https://go.dev/doc/go1.26) — New per-state goroutine metrics
- [Grafana High-Cardinality Alerts](https://grafana.com/docs/grafana/latest/alerting/examples/high-cardinality-alerts/) — Official detection patterns

### Secondary (MEDIUM confidence)
- [Why I Recommend Native Prometheus Instrumentation (PromLabs, July 2025)](https://promlabs.com/blog/2025/07/17/why-i-recommend-native-prometheus-instrumentation-over-opentelemetry/) — Prometheus vs OTel comparison
- [Building High-Performance Time Series on SQLite with Go (dev.to, Aug 2025)](https://dev.to/zanzythebar/building-high-performance-time-series-on-sqlite-with-go-uuidv7-sqlc-and-libsql-3ejb) — SQLite time series patterns
- [Why Server-Sent Events Beat WebSockets (Medium, Dec 2025)](https://medium.com/codetodeploy/why-server-sent-events-beat-websockets-for-95-of-real-time-cloud-applications-830eff5a1d7c) — SSE vs WebSocket comparison
- [Managing High Cardinality in Prometheus - Last9](https://last9.io/blog/how-to-manage-high-cardinality-metrics-in-prometheus/) — Cardinality management
- [vLLM Metrics Documentation](https://docs.vllm.ai/en/latest/design/metrics/) — LLM metrics patterns
- [gRPC in Go: Streaming RPCs (VictoriaMetrics, 2025)](https://victoriametrics.com/blog/go-grpc-basic-streaming-interceptor/) — Go streaming patterns
- [Beyond Tokens-per-Second: How to Balance Speed, Cost and Quality (BentoML, 2026)](https://www.bentoml.com/blog/beyond-tokens-per-second-how-to-balance-speed-cost-and-quality-in-llm-inference) — TPS evaluation
- [Rust vs Go 2026: 5x Latency Wins - Medium](https://medium.com/@yashbatra11111/rust-vs-go-2026-one-of-them-gave-us-5-latency-wins-that-forced-backend-migrations-120403d7caf2) — Metrics emission blocking
- [Why Faster First Tokens Matter - CodeAnt AI](https://www.codeant.ai/blogs/ai-first-token-latency) — TTFT vs throughput

### Tertiary (LOW confidence)
- [LlamaIndex Issue #19740](https://github.com/run-llama/llama_index/issues/19740) — Streaming token counting bug report
- [Stack Overflow: OpenAI Streaming Token Usage](https://stackoverflow.com/questions/75824798/how-to-get-token-usage-for-each-openai-chatcompletion-api-call-in-streaming-mode) — Streaming usage pattern
- [Understanding LLM Response Latency](https://medium.com/@gehouz/understanding-llm-response-latency-a-deep-dive-into-input-vs-output-processing-2d83025b8797) — Medium article

### Codebase References
- `/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go` — Existing token parsing patterns
- `/home/adam/projects/CLIProxyAPI/internal/api/handlers/management/usage.go` — Current usage statistics implementation
- `/home/adam/projects/CLIProxyAPI/internal/api/modules/amp/proxy.go` — Streaming proxy patterns
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/handlers.go` — Handler layer architecture
- `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go` — Streaming response handling
- `/home/adam/projects/CLIProxyAPI/sdk/cliproxy/usage/manager.go` — Usage plugin system
- `/home/adam/projects/CLIProxyAPI/internal/usage/logger_plugin.go` — Existing usage tracking

---
*Research completed: 2026-01-29*
*Ready for roadmap: yes*