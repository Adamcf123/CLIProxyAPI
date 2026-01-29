# Domain Pitfalls

**Domain:** TPS (Tokens Per Second) Metrics for AI API Proxy
**Researched:** 2026-01-29
**Focus:** Streaming response metrics collection in Go-based multi-protocol proxy

## Critical Pitfalls

Mistakes that cause rewrites or major issues.

### Pitfall 1: Cardinality Explosion from High-Cardinality Metrics Labels

**What goes wrong:** Including high-cardinality values (user IDs, full request paths, IP addresses, trace IDs) as metric labels causes unbounded growth in time series count. This overwhelms Prometheus/scraping targets, leading to OOM crashes, slow queries, and eventual monitoring system failure.

**Why it happens:** Developers treat metrics like logging, adding labels for every dimension that seems useful. Each unique label combination creates a new time series. With thousands of users/requests, cardinality explodes exponentially.

**Consequences:**
- Prometheus server crashes with OOM
- Query performance degrades to seconds/minutes
- Monitoring ingestion failures
- Increased infrastructure costs (storage, compute)
- Complete observability blackout during production incidents

**Prevention:**
1. **Label whitelist approach** - Pre-define allowed labels, reject any others at the metrics emission point
2. **Cardinality limits** - Enforce maximum unique values per label (e.g., max 100 model names)
3. **High-cardinality data goes to logs/traces, not metrics** - Request IDs, user IDs belong in structured logs with correlation IDs, not metric labels
4. **Bucket high-cardinality dimensions** - Instead of `tps_by_user_id`, use `tps_by_user_tier` (free/pro/enterprise)
5. **Use histograms for distributions** - Measure latency/token throughput distributions with fixed buckets, not per-request labels

**Detection:**
- Run `topk(20, count_by_filename_name)` in Prometheus to find highest-cardinality metrics
- Alert on: `sum(prometheus_tsdb_symbol_table_size_bytes) > threshold`
- Grafana has [high-cardinality alert examples](https://grafana.com/docs/grafana/latest/alerting/examples/high-cardinality-alerts/)
- Monitor ingestion failures in metrics backend logs

**Phase:** Phase 1 (Metrics Foundation) - Cardinality strategy must be designed before any metrics are emitted. Retrofitting low-cardinality constraints is painful.

---

### Pitfall 2: Blocking Metrics Collection in Streaming Hot Path

**What goes wrong:** Synchronous metrics emission (Prometheus exposition, HTTP calls to push gateway, mutex contention) blocks streaming response forwarding. This causes user-visible latency spikes, especially during metrics scrape intervals.

**Why it happens:** Prometheus client libraries use atomic operations and mutexes for thread safety. When called in the streaming forward loop (every chunk), these operations contend with goroutine schedulers. Even microsecond-level delays accumulate into perceptible lag when processing thousands of chunks.

**Consequences:**
- TTFB (Time To First Byte) increases by milliseconds
- Streaming "stutter" during chunk delivery
- CPU usage spikes from goroutine scheduling pressure
- Under high load, request goroutines pile up, causing cascading latency
- Go 1.26+ will expose these blocked goroutines via new [per-state goroutine metrics](https://go.dev/doc/go1.26)

**Prevention:**
1. **Non-blocking metrics buffer** - Collect metrics in a channel-backed goroutine, flush periodically
2. **Metrics decoupling** - Streaming goroutine writes to pre-allocated struct/atomic counters; separate metrics goroutine reads
3. **Batch metric updates** - Instead of `tokens += delta` per chunk, aggregate and update once per 100ms or request end
4. **Use atomic operations** - `atomic.AddInt64` instead of mutex-protected counters
5. **Pre-allocate metric instances** - Create metric vectors at startup, avoid `MustCreate` in hot path

**Detection:**
- Enable Go 1.26 goroutine state metrics (running, waiting, blocked)
- Profile with `go tool pprof -block` to find contention points
- Monitor P95/P99 latency correlation with metrics scrape intervals
- Check for goroutine leaks: `runtime.NumGoroutine()` trending up

**Phase:** Phase 2 (TPS Collection) - Streaming integration must be designed with non-blocking patterns from day one. The [Rust vs Go 2026 latency comparison](https://medium.com/@yashbatra11111/rust-vs-go-2026-one-of-them-gave-us-5-latency-wins-that-forced-backend-migrations-120403d7caf2) specifically calls out metrics emission blocking.

---

### Pitfall 3: Token Counting Inconsistency Across Providers

**What goes wrong:** Token counts from upstream providers (OpenAI, Anthropic, Gemini) disagree with client-side tiktoken counts. Providers count differently (special tokens, formatting, caching) and don't expose their tokenization algorithms. TPS calculations using wrong denominators produce misleading metrics.

**Why it happens:**
- Each provider uses different tokenization (cl100k_base, claude-tokenizer, gemini-tokenizer)
- Providers include/exclude special tokens in reported counts inconsistently
- Cached tokens are counted differently (some include in input, some separate)
- Streaming responses may not include usage metadata until final chunk
- Some providers don't report usage at all in streaming mode

**Consequences:**
- TPS metrics vary 10-30% from reality
- Cost calculations based on tokens are wrong
- Comparisons between providers are meaningless
- Users lose trust in metrics dashboard
- Billing disputes if tokens drive pricing

**Prevention:**
1. **Use provider-reported counts when available** - Trust `usage.total_tokens` from OpenAI, `usage.input_tokens` from Anthropic
2. **Normalize at collection time** - Store both provider-reported and estimated counts, label which is which
3. **Handle missing usage metadata** - Some Gemini responses don't include usage; estimate client-side or flag as `unknown`
4. **Document discrepancies** - Metrics metadata should include "provider-reported" vs "tiktoken-estimated"
5. **Add confidence intervals** - Report TPS as "50 ± 5 tokens/s" when estimation is involved
6. **Test against real responses** - Unit tests comparing streaming vs non-streaming token counts for same prompt

**Detection:**
- A/B tests: Same prompt, streaming vs non-streaming, compare totals
- Monitor `stream_usage` vs `final_usage` deltas; large deltas indicate missing interim counts
- Compare tiktoken estimates against provider-reported counts across sample requests
- Track "unknown token count" percentage by provider

**Phase:** Phase 2 (TPS Collection) - Token counting strategy must be provider-aware. The [Stack Overflow discussion on streaming token usage](https://stackoverflow.com/questions/75824798/how-to-get-token-usage-for-each-openai-chatcompletion-api-call-in-streaming-mode) shows this is a known challenge.

---

## Moderate Pitfalls

Mistakes that cause delays or technical debt.

### Pitfall 4: TPS Calculation Missing First-Token Latency

**What goes wrong:** TPS calculated as `total_tokens / total_time` ignores streaming's batched nature. This masks TTFT (Time To First Token) issues, reporting "average" throughput that doesn't reflect user-perceived performance.

**Why it happens:** Simple implementation measures request start → request end. For streaming, this includes initial model loading, prompt processing, and first token generation—all latency that users perceive as "slowness" before any content appears.

**Consequences:**
- Metrics look healthy (e.g., 100 tokens/s) but users experience 3-second delays before first token
- Optimizations target throughput instead of TTFT, missing user-visible improvements
- A/B tests show no improvement despite TPS gains
- The industry consensus is that [first-token latency matters more than total throughput](https://www.codeant.ai/blogs/ai-first-token-latency)

**Prevention:**
1. **Report multiple TPS metrics**:
   - `ttft_ms` - Time to first token (request start → first chunk)
   - `tps_inter_token` - Average time between chunks during streaming
   - `tps_overall` - Total tokens / total time (end-to-end)
2. **Separate phases in metrics** - Distinguish "generation phase" from "initialization phase"
3. **Percentile reporting** - P50/P95/P99 for both TTFT and inter-token latency
4. **Label by model** - Different models have different TTFT baselines

**Detection:**
- Scatter plot of `ttft_ms` vs `tps_overall` to identify outliers (good TPS, bad TTFT)
- Compare streaming vs non-streaming TTFT for same model
- User complaints about "slow first response" despite good throughput metrics

**Phase:** Phase 2 (TPS Collection) - Design metrics schema to capture TTFT from day one. Adding it later requires replaying logs or waiting for new data.

---

### Pitfall 5: Metrics Collection Breaking When Upstream Changes Response Format

**What goes wrong:** Hardcoded JSON paths for token counts (`usage.total_tokens`) break when providers add fields, change nesting, or introduce new response formats. This causes metrics to silently report zero or crash the collector.

**Why it happens:** Upstream providers evolve their APIs. New fields are added (e.g., `reasoning_tokens`), existing fields move, or streaming formats diverge from non-streaming. Codebases using `gjson.GetBytes(response, "usage.total_tokens")` silently return zero when path changes.

**Consequences:**
- Metrics suddenly drop to zero, causing false alarms
- Collector panics on unexpected JSON structures
- Hours spent debugging why "no tokens" are being counted
- Loss of historical data during outages

**Prevention:**
1. **Graceful degradation** - Missing fields should log warnings, not panic
2. **Multiple path fallbacks** - Try `usage.total_tokens`, then `usage_metadata.totalTokenCount`, then estimate
3. **Schema versioning** - Label metrics with provider version, allow multiple parsers in parallel
4. **Integration tests** - Weekly jobs hitting real upstreams to validate parsing
5. **Parser metrics** - Track `parser_success_rate` by provider to catch silent failures

**Detection:**
- Alert on sudden drops in token counts per provider
- Monitor `parse_failure_total` counter
- Weekly integration tests against staging environments
- Track percentage of requests with `total_tokens = 0`

**Phase:** Phase 2 (TPS Collection) - Build resilient parsers. The existing codebase has examples of this pattern in `usage_helpers.go` (checking multiple field paths), extend to TPS metrics.

---

### Pitfall 6: Memory Leaks from Metrics Accumulation

**What goes wrong:** Per-request metrics objects (histograms, summaries) are created but never released. Under load, memory grows unbounded until OOM kills the process.

**Why it happens:** Developers instantiate `prometheus.NewHistogramVec` per request or store metrics in maps without cleanup. Go's GC doesn't collect metrics registered with Prometheus's default registry.

**Consequences:**
- Memory usage grows linearly with request count
- OOM kills process after hours/days under load
- Restarting clears memory but loses all metrics context
- Production incidents during traffic spikes

**Prevention:**
1. **Use metric vectors at package scope** - Create once at startup, reuse
2. **Avoid per-request metrics** - Label existing vectors instead of creating new instances
3. **If using maps, use expiring cache** - `github.com/patrickmn/go-cache` with TTL
4. **Profile memory** - `go tool pprof -heap` before/after load tests
5. **Track object counts** - Count histograms in registry, alert on growth

**Detection:**
- `runtime.ReadMemStats()` trending up
- `pprof` heap profiles showing metric objects as top allocators
- Prometheus `prometheus_tsdb_symbol_table_size_bytes` (scraping target's own metrics)

**Phase:** Phase 1 (Metrics Foundation) - Memory patterns should be established in the initial metrics design. Datadog's [Go memory leak guide](https://www.datadoghq.com/blog/go-memory-leaks/) covers this well.

---

## Minor Pitfalls

Mistakes that cause annoyance but are fixable.

### Pitfall 7: Metrics Not Surviving Process Restarts

**What goes wrong:** TPS metrics reset to zero on deployment/restart, losing context for "is this better or worse than before?"

**Prevention:**
- Use Prometheus pushgateway for ephemeral metrics (not recommended for production)
- Design metrics as rates (per second) not cumulative counts
- Compare against baselines, not absolute values

**Phase:** Phase 3 (Dashboards) - Dashboard design should handle restarts gracefully.

---

### Pitfall 8: Time Zone Issues in Time-Based Aggregations

**What goes wrong:** TPS aggregated by hour shows double-counted or missing hours due to UTC vs local time confusion.

**Prevention:**
- Always store/aggregate in UTC
- Convert to local time only at display time
- Include timezone label in queries

**Phase:** Phase 3 (Dashboards) - Visualization layer concern.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| **Phase 1: Metrics Foundation** | Cardinality explosion (Pitfall 1) | Design label whitelist before writing any metrics code |
| **Phase 2: TPS Collection** | Blocking metrics in streaming (Pitfall 2) | Prototype non-blocking patterns early, profile under load |
| **Phase 2: TPS Collection** | Token counting inconsistency (Pitfall 3) | Test against all providers' streaming responses before declaring done |
| **Phase 3: Dashboards** | Missing TTFT visibility (Pitfall 4) | Include TTFT as first-class metric in all dashboard designs |
| **Phase 4: Optimization** | Optimizing wrong metric (throughput vs TTFT) | Validate improvements against user-perceived latency, not just TPS |

---

## Sources

### High Confidence (Official Documentation)
- [Go 1.26 Release Notes - Goroutine Metrics](https://go.dev/doc/go1.26) - New per-state goroutine metrics for detecting blocking operations
- [Grafana High-Cardinality Alerts](https://grafana.com/docs/grafana/latest/alerting/examples/high-cardinality-alerts/) - Official detection patterns
- [Datadog Go Memory Leak Guide](https://www.datadoghq.com/blog/go-memory-leaks/) - Memory pitfalls in Go applications

### Medium Confidence (Multiple Credible Sources)
- [Managing High Cardinality in Prometheus - Last9](https://last9.io/blog/how-to-manage-high-cardinality-metrics-in-prometheus/) - Comprehensive cardinality management
- [Understanding Prometheus Metric Types - Dash0](https://www.dash0.com/knowledge/prometheus-metrics) - Notes cardinality explosion consequences
- [Why Metric Cardinality Keeps Exploding - Sawmills](https://www.sawmills.ai/blog/metric-cardinality-explained-sre-fixes) - SRE perspective on fixing cardinality
- [I Deleted Prometheus and Nothing Broke - Medium](https://medium.com/beyond-localhost/i-deleted-prometheus-and-nothing-broke-0d071b46d5ad) - Real-world cardinality in production (Jan 2026)
- [Golang Application Performance Monitoring - Atatus](https://www.atatus.com/blog/golang-application-monitoring/) - Go-specific performance issues (July 2025)
- [OpenMeter Token Usage with Streams](https://openmeter.io/blog/token-usage-with-openai-streams-and-nextjs) - Streaming token counting patterns
- [Rust vs Go 2026: 5x Latency Wins - Medium](https://medium.com/@yashbatra11111/rust-vs-go-2026-one-of-them-gave-us-5-latency-wins-that-forced-backend-migrations-120403d7caf2) - Mentions metrics emission blocking (Jan 2026)
- [Why Faster First Tokens Matter - CodeAnt AI](https://www.codeant.ai/blogs/ai-first-token-latency) - TTFT vs throughput tradeoffs (Jan 2026)
- [Uncounted Tokens: AI Gateway Rate Limiting - Dev.to](https://dev.to/spacewander/uncounted-tokens-the-game-of-attack-and-defense-in-ai-gateway-rate-limiting-3mnk) - Streaming token counting vulnerabilities (Dec 2025)

### Low Confidence (Single Source / Unverified)
- [LlamaIndex Issue #19740](https://github.com/run-llama/llama_index/issues/19740) - Streaming token counting bug report (Aug 2025)
- [Stack Overflow: OpenAI Streaming Token Usage](https://stackoverflow.com/questions/75824798/how-to-get-token-usage-for-each-openai-chatcompletion-api-call-in-streaming-mode) - Streaming `stream_options` pattern
- [Galileo Tiktoken Guide](https://galileo.ai/blog/tiktoken-guide-production-ai) - Tiktoken implementation (Aug 2025)
- [Kubernetes Gateway API Inference Extension Issue #178](https://github.com/kubernetes-sigs/gateway-api-inference-extension/issues/178) - Per-output-token latency metrics discussion (Jan 2025)

### Codebase References
- `/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go` - Existing token parsing patterns for OpenAI, Claude, Gemini
- `/home/adam/projects/CLIProxyAPI/internal/api/handlers/management/usage.go` - Current usage statistics snapshot implementation
- `/home/adam/projects/CLIProxyAPI/internal/api/modules/amp/proxy.go` - Streaming proxy patterns, gzip handling, SSE detection

### Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Cardinality Pitfalls | HIGH | Multiple authoritative sources, well-documented in Prometheus ecosystem |
| Blocking Metrics | MEDIUM | Go 1.26 metrics + Medium article; could benefit from Go-specific performance studies |
| Token Counting | MEDIUM | Multiple sources agree it's problematic, but solutions vary by provider |
| TTFT Importance | MEDIUM | Recent 2026 article, aligns with industry consensus but limited sources |
| Memory Leaks | HIGH | Datadog guide + established Go patterns |

### Gaps to Address

- **Go-specific streaming benchmarks**: Would benefit from load testing different metrics collection approaches in the actual codebase
- **Provider-specific token counting**: Each provider (OpenAI, Anthropic, Gemini) may have quirks not covered in general research
- **TPS calculation validation**: No strong sources on "correct" TPS formulas for streaming; may need empirical testing
