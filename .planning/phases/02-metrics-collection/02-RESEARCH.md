# Phase 02: Metrics Collection - Research

**Researched:** 2026-01-29
**Domain:** Go 流式代理（Gin + SSE）中的 TPS/TTFT/TPOT 指标采集、实时显示与 JSONL 落盘
**Confidence:** MEDIUM

## Summary

本阶段要做的不是“再算一套 TPS”，而是把 Phase 1 已经实现的 `internal/metrics.TPSCollector`（TTFT/TPS/TPOT）接入到现有的“流式转发（SSE）”主链路里，同时保证：
1) 主响应流几乎零开销（不阻塞、不加重试/兜底、不改变协议）；
2) 每个请求结束后都有一份“本次请求汇总指标”可展示；
3) 指标以 JSON Lines 写入按日轮转的日志文件。

代码库已经具备两条关键基础设施：
- 流式转发的稳定入口（`/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go` 的 `ForwardStream`），可以在写出每个 chunk 时做轻量 hook（例如记录“首个 chunk 发送时间”）。
- usage 采集的异步分发器（`/home/adam/projects/CLIProxyAPI/sdk/cliproxy/usage/manager.go`），以及 provider 特定的 usage 解析（`/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go`），已经把“跨 provider 的 output_tokens / input_tokens”归一化成 `usage.Record`。

**Primary recommendation:** 以 “请求生命周期状态对象（Per-request state）” 为中心，把 `FirstTokenTime` 从 streaming 转发链路采集，把 `OutputTokens` 从 usage 解析链路采集，然后在 request end 做一次 `TPSCollector.CompleteRequest(...)`，并将结果通过“异步 JSONL writer”落盘。

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `encoding/json` | Go toolchain | JSON 序列化（JSONL） | 内置、稳定、性能足够；`json.Encoder.Encode` 自带换行，天然适合 JSONL |
| Go stdlib `bufio` | Go toolchain | 批量写入、减少 syscalls | 高吞吐写文件标配 |
| Go stdlib `os`/`filepath`/`time` | Go toolchain | 目录创建、按日文件名、时间戳 | 按日轮转和路径拼接只需标准库 |
| Gin `github.com/gin-gonic/gin` | repo 已使用 | HTTP handler + `c.Writer` | 项目现有主框架 |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/tidwall/gjson` | repo 已使用 | 从 SSE/JSON payload 抽取字段 | 若在 handler 层直接解析终止 chunk 的 usage，或需要从响应里提取 model/provider |
| `github.com/sirupsen/logrus` | repo 已使用 | 非结构化运行日志 | 运行日志继续用 logrus；指标 JSONL 不建议复用 logrus 文本格式 |
| `gopkg.in/natefinch/lumberjack.v2` | repo 已使用 | 主日志滚动 | 仅用于 main log；指标日志要求“按日期命名”，lumberjack 默认按大小滚动，不直接匹配该需求 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| 自写 usage 解析 | 复用 `internal/runtime/executor/usage_helpers.go` | 复用可保证跨 provider 一致性；重写会引入重复逻辑与不一致 |
| 在响应里注入 TPS 文本 | 仅 server 侧打印/或独立 telemetry | 注入会改变 SSE/JSON 协议，容易破坏下游客户端；应避免 |

## Architecture Patterns

### Recommended Project Structure

Phase 2 的新增代码建议围绕三个“责任清晰”的组件组织（命名仅示意，planner 决定最终位置）：

```
internal/metrics/              # Phase 1 产物：TPSCollector + SlidingWindow
internal/metricslog/           # Phase 2：JSONL 指标落盘（异步 writer + 按日轮转）
internal/runtime/executor/     # provider 响应解析与 usage 发布（现有）
sdk/api/handlers/              # 流式转发入口（ForwardStream）与各 provider handler
```

关键点是把指标采集拆成“轻量 hook + 结束汇总”两段，避免在每个 chunk 做重活。

### Pattern 1: Per-Request Metrics State（单请求状态对象）

**What:** 在一次请求内共享一个状态对象，写入 `StartTime / FirstTokenTime / OutputTokens / StatusCode / Error` 等字段；不同采集点（streaming forwarder、usage 解析、handler cancel）只负责填自己那部分字段。

**When to use:** 你需要同时从“流式写出时刻”与“终止 usage 信息”采样，并在请求结束时组合计算。

**Why in this repo:** `ForwardStream` 与 `usage.PublishRecord` 之间通过 `context.Context` 贯通（`/home/adam/projects/CLIProxyAPI/sdk/api/handlers/handlers.go` 的 `GetContextWithCancel` 把 `gin` 放进 ctx；`usage_helpers.go` 发布时沿用同一个 ctx）。

**Example (pseudo):**
```go
// state lives for one request
type requestMetricsState struct {
    start time.Time
    firstTokenOnce sync.Once
    firstToken time.Time

    usageOnce sync.Once
    inputTokens  *int64
    outputTokens *int64

    // final computed metrics
    tps  *float64
    ttft *float64
    tpot *float64
}
```

### Pattern 2: Hook `ForwardStream.WriteChunk` for TTFT anchor

**What:** 把“首 token”近似为“首个有效 chunk 写出并 flush 的时间点”。

**Where:** `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go` 的 `writeChunk(chunk); flusher.Flush()`。

**Why:**
- handler 层天然知道什么时候真正写到 client（而不是 upstream 收到）；
- hook 成本极低（一次 `sync.Once` / 原子布尔）。

**Risk:** keep-alive 心跳（默认 `: keep-alive\n\n`）可能早于真正 token；需要过滤：只在“非 keep-alive、非空、非 event-only” chunk 上触发 first token。

### Pattern 3: Reuse `usage.Manager` plugin model for end-of-request aggregation

**What:** 新增一个 usage 插件（类似 `/home/adam/projects/CLIProxyAPI/internal/usage/logger_plugin.go`），订阅 `usage.Record`，在 record 到达时：
- 取 `record.Detail.OutputTokens`（可能为 0 或缺失）；
- 完成 `TPSCollector.CompleteRequest(...)`；
- 生成 JSONL 的 `MetricsLogLine` 并 enqueue 到异步 writer。

**Why:** provider 解析已经集中在 `/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go`，不要把解析逻辑复制到 handler。

## Don’t Hand-Roll

| Problem | Don’t Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| “跨 provider 的 token usage 提取” | handler 里用字符串匹配/手写 JSON 路径 | `/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go` 的 `parse*Usage` | 已覆盖 OpenAI/Claude/Gemini 家族差异，重复实现会不一致 |
| “SSE 转发的主循环” | 每个 handler 自己 select + flush + keep-alive | `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go` 的 `ForwardStream` | 该实现已处理 terminal err、keep-alive、ctx cancel |

**Key insight:** 这个 repo 已经把“流式复杂度”与“usage 解析复杂度”封装好了，Phase 2 的关键是把它们通过一个 per-request state 组合，而不是把逻辑散到每个 handler。

## Common Pitfalls

### Pitfall 1: 把实时 TPS 写进 HTTP 响应导致协议破坏

**What goes wrong:** SSE/JSON 流中插入额外文本（如 `\rTPS=...`）会让下游 SDK/CLI 解析失败。

**How to avoid:** 服务器侧实时显示用 `os.Stderr`（或日志）输出；响应体保持原样转发。

### Pitfall 2: keep-alive 被误认为 first token，TTFT 偏小

**What goes wrong:** `ForwardStream` 默认会写 `: keep-alive\n\n`；如果把任何 chunk 都当成 first token，会把 TTFT 记录成“到第一条 keep-alive 的时间”。

**How to avoid:** first-token 触发条件需要过滤：
- 非空；
- 不是 `: keep-alive`；
- 不是只有 `event:` 的控制行。

### Pitfall 3: usage 在流式路径可能“缺失或延后”，导致 TPS 无法计算

**What goes wrong:** 有些 provider/模式不返回 usage（或只在 terminal chunk 返回），`OutputTokens` 为空会让 `TPSCollector.CompleteRequest` 返回 `ErrNonPositiveTokens`。

**How to avoid:**
- 指标日志字段允许 `null`（ctx 已决策）；
- UI/汇总要能显示“不可用原因”；
- 不要用“猜 tokens”的方式填充（会改变业务语义）。

### Pitfall 4: 异步 writer 队列饱和与“数据完整性”决策冲突

**What goes wrong:** 需求同时要求“不阻塞主响应流”和“始终完整收集（不降级）”。如果队列满了：
- 丢弃会违反“完整”；
- 阻塞 enqueue 会影响请求尾部延迟；
- 无界队列会造成内存风险。

**How to avoid (planning must decide):** 明确“完整性”作用域：
- 只保证“本次请求汇总展示必有”（内存中完整）；
- 落盘失败允许静默丢弃（ctx 已决策）；
或
- 落盘也必须完整，则需要：足够大的 buffer + 高性能批量写入 + 限制并发写盘（单 writer）以避免饱和。

### Pitfall 5: 并发安全

`internal/metrics.TPSCollector` 的 `getOrCreateWindow` 注释明确“不是并发安全”（见 `/home/adam/projects/CLIProxyAPI/internal/metrics/collector.go:180`）。

**How to avoid:** planner 必须决定并发模型：
- 单线程（所有 CompleteRequest 在一个 goroutine）
- 或者在 collector 外部加锁
- 或者把 windows map 改为并发安全结构

## Code Examples

### Example 1: Streaming forwarder hook point

Source: `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go`

```go
case chunk, ok := <-data:
    if !ok { ... }
    writeChunk(chunk)
    flusher.Flush()
```

这里是 TTFT 的关键采样点：第一次成功写出“有效内容 chunk”时记录 `FirstTokenTime`。

### Example 2: Usage plugin dispatch model

Source: `/home/adam/projects/CLIProxyAPI/sdk/cliproxy/usage/manager.go`

```go
type Plugin interface {
    HandleUsage(ctx context.Context, record Record)
}

func PublishRecord(ctx context.Context, record Record) {
    DefaultManager().Publish(ctx, record)
}
```

Phase 2 可新增 `MetricsPlugin`，在 `HandleUsage` 中完成：
- 读取 per-request state
- 填 `OutputTokens`
- 触发 `TPSCollector.CompleteRequest`
- enqueue JSONL

### Example 3: JSON Lines writer (recommended pattern)

```go
// One goroutine owns the file handle + bufio.Writer.
// Drain items from a buffered channel and write json per line.
enc := json.NewEncoder(bufioWriter)
enc.SetEscapeHTML(false)

for {
    select {
    case item := <-ch:
        _ = enc.Encode(item) // adds \n
    case <-ticker.C:
        _ = bufioWriter.Flush()
    }
}
```

需要补充按日轮转：当 `time.Now().Format("2006-01-02")` 变化时，关闭旧文件并打开新文件（append）。

### Example 4: Terminal 同行覆盖显示（server 侧）

```go
// Write to stderr to avoid corrupting SSE stdout/logs.
fmt.Fprintf(os.Stderr, "\r\033[2KTPS: %.2f ttft: %.2fs", tps, ttft)
// \033[2K clears the entire line; \r returns to line start.
```

注意：如果日志输出也在 terminal，会与该行交织；planner 需决定是否只在 TTY 下启用。

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| handler 直接写响应、各处散落 streaming loop | 统一 `ForwardStream` + `usage.Manager` plugin | repo 当前状态 | 更容易在单一位置挂钩做指标采样 |

## Open Questions

1. **“实时 TPS”对准确性的要求是什么？**
   - What we know: 决策要求“每秒更新 + 同一行覆盖”。
   - What's unclear: tokens 是否必须来自 usage（通常只在结束才有），还是允许“基于已接收 token 计数”的近似。
   - Recommendation: planner 需要明确：实时阶段允许 `tps=null` 或 `tps=--`，直到 usage 到达后再显示真实 TPS。

2. **“始终完整收集”到底约束哪个环节？**
   - What we know: 决策同时写了“静默丢弃写入失败”和“始终完整收集”。
   - What's unclear: 落盘是否允许丢，还是必须保证落盘。
   - Recommendation: planner 明确优先级：
     - 若“展示完整”优先，则落盘可 best-effort；
     - 若“落盘完整”也必须，则需要强约束队列不饱和（容量、写入性能、限流策略）。

3. **TPSCollector 并发模型**
   - What we know: `TPSCollector.getOrCreateWindow` 注释说不是并发安全。
   - Recommendation: planner 决定是否让 collector 所有写操作都发生在同一 goroutine（例如通过一个 metrics-event channel 串行化）。

## Sources

### Primary (HIGH confidence)
- Go `encoding/json` docs: https://pkg.go.dev/encoding/json
- Go `bufio` docs: https://pkg.go.dev/bufio
- Go `os` docs: https://pkg.go.dev/os
- Gin ResponseWriter/streaming usage (project code):
  - `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/stream_forwarder.go`
  - `/home/adam/projects/CLIProxyAPI/sdk/api/handlers/openai/openai_responses_handlers.go`
- Usage pipeline (project code):
  - `/home/adam/projects/CLIProxyAPI/internal/runtime/executor/usage_helpers.go`
  - `/home/adam/projects/CLIProxyAPI/sdk/cliproxy/usage/manager.go`

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - 基于标准库与 repo 已使用依赖
- Architecture: MEDIUM - 需要 planner 对“实时 TPS / 完整性 / 并发模型”作关键决策
- Pitfalls: HIGH - 来自现有转发/usage 机制与决策冲突分析

**Research date:** 2026-01-29
**Valid until:** 2026-02-28
