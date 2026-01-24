# Feature Landscape

**Domain:** API Performance Monitoring - TPS Metrics for AI API Proxy
**Researched:** 2026-01-29

## Table Stakes

Features users expect. Missing = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Time to First Token (TTFT)** | Most critical metric for perceived response speed in streaming AI responses | Low | Measure from request sent to first token received; track p50 and p95 percentiles |
| **Tokens Per Second (TPS)** | Standard throughput metric for LLM APIs; indicates generation speed | Low | Output tokens generated per second during steady-state generation |
| **Time Per Output Token (TPOT)** | Measures inter-token latency; determines smoothness of streaming experience | Low | Average time between consecutive tokens after TTFT |
| **Total Token Count** | Required for cost calculation and usage tracking | Low | Input + output token counts for each request |
| **Summary After Response** | Users expect to see performance summary after completion | Low | Display TTFT, TPS, TPOT, and token counts when response completes |
| **Structured Logging** | Essential for debugging, analytics, and historical analysis | Low | Log metrics in structured format (JSON) for downstream processing |
| **Error Rate Tracking** | Standard reliability metric; indicates system health | Low | Track 4xx/5xx error percentages separately |
| **Request Latency** | Core performance metric for all APIs | Low | End-to-end request/response time |
| **Per-Request Metrics** | Granular visibility into individual API calls | Low | Each request gets its own metrics record |
| **Timestamp Recording** | Foundation for all time-series metrics | Low | Record start, first token, and end timestamps |

## Differentiators

Features that set product apart. Not expected, but valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Real-Time TPS Display During Streaming** | Users see generation speed in real-time; enables instant performance feedback | Medium | Update TPS counter as tokens stream; requires efficient display mechanism |
| **Percentile Tracking (p50, p95, p99)** | Beyond averages; shows tail latency which affects user experience | Medium | Compute percentiles across multiple requests; reveals outliers |
| **Cost Per Request Attribution** | Directly connects performance to economics; critical for AI APIs | Medium | Calculate using token counts and model pricing |
| **Request Aggregation by Feature/Customer** | Identifies expensive workflows and margin-negative usage patterns | High | Group metrics by endpoint, user ID, or custom tags |
| **Cache Effectiveness Metrics** | Shows value of caching strategies; validates optimization investments | Medium | Track cache hit rate and avoided API costs |
| **Database Storage for Historical Trends** | Enables long-term analysis and capacity planning | High | Persist metrics to TSDB for querying over time |
| **Prometheus/OpenTelemetry Export** | Integrates with existing observability stack; standard protocol | Medium | Expose metrics endpoint for Prometheus scraping or OTLP export |
| **Multi-Protocol Metric Normalization** | Compare performance across OpenAI, Gemini, Claude APIs uniformly | Medium | Normalize different response formats to consistent metrics |
| **Token Usage Breakdown by Type** | Detailed visibility (input/output/cached/reasoning tokens) | Medium | Track each token type separately when provider exposes it |
| **Retry Rate and Error Cost Impact** | Quantifies hidden costs of retries and failed requests | Medium | Track retry attempts and their token/cost implications |
| **Anomaly Detection** | Proactive alerts when metrics deviate from baselines | High | Requires statistical analysis and threshold tuning |
| **Geographic/Multi-Location Monitoring** | Reveals region-specific performance issues | High | Monitor from multiple vantage points |

## Anti-Features

Features to explicitly NOT build. Common mistakes in this domain.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| **Average-Only Metrics** | Hides outliers; p99 latency can be 10x average; poor UX for tail users | Track percentiles (p50, p95, p99) alongside averages |
| **Storage of Raw Token Streams** | Massive storage costs; minimal analytical value | Store only aggregated metrics and token counts |
| **Synchronous Database Writes in Hot Path** | Blocks response streaming; destroys performance | Async write to queue/collector; batch commits |
| **Real-Time Dashboard as Only View** | Passive monitoring misses urgent problems; requires someone watching | Add automated alerts with notification channels |
| **Generic Error Aggregation** | Can't distinguish between 400 (user error) and 500 (system error) | Track 4xx and 5xx error rates separately |
| **Metrics Sampling Without Documentation** | Users don't know if metrics are 100% or 1% sampled; misleading | Always document sampling rate; prefer no sampling for core metrics |
| **Complexity-Based Rate Limiting Metrics** | Confuses users; rate limiting should be separate from observability | Keep rate limiting separate; expose raw metrics for user's own policies |
| **Hardcoded Provider Pricing** | Prices change frequently; requires redeploy to update | Fetch pricing from config/API; allow user overrides |
| **Time-Series Storage in Main Database** | Bloates transactional DB; poor query performance for time-series | Use dedicated TSDB (Prometheus, InfluxDB, TimescaleDB) |
| **Blocking Metrics Collection** | If metrics system fails, API fails; reduces availability | Use fire-and-forget or circuit-breaker pattern |
| **Overly Granular Real-Time Display** | UI updates too frequently; performance degradation | Throttle display updates (e.g., every 100ms or 10 tokens) |

## Feature Dependencies

```
Basic Metrics Collection (TTFT, TPS, TPOT, Token Count)
    ↓
Structured Logging
    ↓
    ├─→ Real-Time Display (requires: efficient display mechanism)
    │       ↓
    │   Real-Time TPS Display During Streaming
    │
    ├─→ Database Storage (requires: TSDB choice)
    │       ↓
    │   Historical Trend Analysis
    │       ↓
    │   Percentile Tracking
    │       ↓
    │   Anomaly Detection
    │
    └─→ Cost Attribution (requires: pricing configuration)
            ↓
        Cost Per Request
            ↓
        Request Aggregation by Feature/Customer
```

## MVP Recommendation

For MVP, prioritize:

1. **Basic Metrics Collection** (Table Stakes)
   - TTFT (Time to First Token)
   - TPS (Tokens Per Second)
   - TPOT (Time Per Output Token)
   - Total Token Count (input/output)

2. **Structured Logging** (Table Stakes)
   - Log metrics in JSON format
   - Include request ID, timestamps, all metrics

3. **Summary After Response** (Table Stakes)
   - Display metrics when response completes

4. **One Differentiator**
   - **Real-Time TPS Display During Streaming** - This provides immediate user value and demonstrates the proxy's capabilities

Defer to post-MVP:

- **Database Storage**: Can be added later; logging provides initial persistence
- **Percentile Tracking**: Requires aggregation across requests; log analysis can serve initially
- **Cost Attribution**: Pricing complexity can wait; raw token counts enable manual calculation
- **Prometheus Export**: Integration can be added once metrics are stable
- **Anomaly Detection**: Requires baseline data; cannot be done in MVP

## Phase Structure Recommendations

**Phase 1: Core Metrics (MVP)**
- Implement TTFT, TPS, TPOT, token count tracking
- Add structured logging
- Display summary after response

**Phase 2: Real-Time Visibility**
- Add real-time TPS display during streaming
- Optimize display performance (throttling, efficient UI updates)

**Phase 3: Persistence & Analysis**
- Database storage (Prometheus or InfluxDB)
- Percentile tracking (p50, p95, p99)
- Basic querying and trend visualization

**Phase 4: Advanced Features**
- Cost attribution with provider pricing
- Request aggregation by customer/feature
- Prometheus/OpenTelemetry export

**Phase 5: Intelligence**
- Anomaly detection and alerting
- Geographic/multi-location monitoring

## Sources

- [The API Metrics Every SaaS Team Must Track In 2026](https://www.cloudzero.com/blog/api-metrics/) - CloudZero (HIGH)
- [Langfuse Token & Cost Tracking Documentation](https://langfuse.com/docs/observability/features/token-and-cost-tracking) - Langfuse (HIGH)
- [API Performance Monitoring—Key Metrics and Best Practices](https://www.catchpoint.com/api-monitoring-tools/api-performance-monitoring) - Catchpoint (HIGH)
- [What is API Monitoring: Tools, Metrics & Best Practices for 2026](https://www.levo.ai/resources/blogs/what-is-api-monitoring-tools-metrics-best-practices-2026) - Levo.ai (HIGH)
- [Key metrics for LLM inference](https://bentoml.com/llm/inference-optimization/llm-inference-metrics) - BentoML (HIGH)
- [Metrics That Matter for LLM Inference](https://compute.hivenet.com/post/llm-inference-metrics-ttft-tps) - Compute HiveNet (HIGH)
- [6 Common API Gateway Monitoring Mistakes](https://api7.ai/blog/6-api-gateway-monitoring-mistakes) - API7 (MEDIUM)
- [Understanding LLM Response Latency](https://medium.com/@gehouz/understanding-llm-response-latency-a-deep-dive-into-input-vs-output-processing-2d83025b8797) - Medium (LOW)
- [Prometheus as OpenTelemetry Backend](https://prometheus.io/docs/guides/opentelemetry/) - Prometheus (HIGH)
- [OpenTelemetry Metrics: Types, Examples & Best Practices](https://www.groundcover.com/opentelemetry/opentelemetry-metrics) - Groundcover (HIGH)
- [FastAPI Streaming APIs for Real-Time Dashboards](https://medium.com/@bhagyarana80/10-fastapi-streaming-apis-for-real-time-dashboards-b8b013f92da0) - Medium (LOW - access blocked)
