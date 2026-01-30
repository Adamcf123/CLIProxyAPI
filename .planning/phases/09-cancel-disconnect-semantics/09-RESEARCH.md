**Researched:** 2026-01-31
**Domain:** 客户端 cancel / 断连（disconnect）语义在写入层/查询层/聚合层的一致性（success/failure/canceled）
**Confidence:** MEDIUM (需要在实现时用新增测试把边界锁死)

## Summary

本阶段要解决的核心问题是：**客户端取消/断连不应被当作 success 落库与聚合**。当前系统的 success/failure 切分仅依赖 `status_code` 是否为 2xx + `error_info` 是否为空（`internal/api/handlers/management/metrics.go`），而 cancel/disconnect 在很多路径下不会写入 `LastError`，并且 handler 通常在流式结束后统一 `state.SetStatusCode(c.Writer.Status())`（多数情况下仍是 200），因此会被误判为 success。

在不引入新对外能力（新端点/新 query 参数）的前提下，最小改动路径是：

1) **在请求生命周期内捕捉 client gone 信号**（request context Done / ResponseWriter.Write error）并把它沉淀到 `metricsruntime.RequestState`；
2) **在落库时持久化一个可被 Query API 识别的 canceled 信号**（避免复用 `error_info`，因为 per-request canceled 必须保持 error 为空）；
3) **在 Query API 输出中显式区分 canceled**，并按已锁定口径完成聚合与统计（percentiles 排除 canceled；buckets 单列 canceled_count；total 包含 canceled；tokens 聚合排除 canceled）。

Phase 09 的 CONTEXT.md 已锁定了语义与对外契约，研究重点在“现有代码哪些地方能可靠检测/标记 canceled”，以及“如何以最小代价在现有 schema/查询实现里表达第三种 outcome”。

## Current Implementation Map (关键文件)

### 1) RequestState: 当前只记录 StatusCode / LastError（无 canceled 字段）

- `internal/metricsruntime/request_state.go`
  - `RequestState.StatusCode`：handler 结束时设置（常见：`state.SetStatusCode(c.Writer.Status())`）。
  - `RequestState.LastError`：流式 terminal error 等路径会设置（例如 `sdk/api/handlers/stream_forwarder.go`）。

### 2) Streaming: ForwardStream 已监听 request context Done，但只做 cancel，不记录 canceled 语义

- `sdk/api/handlers/stream_forwarder.go`
  - `case <-c.Request.Context().Done(): cancel(c.Request.Context().Err()); return`
  - terminal error 会通过 `maybeSetStreamingTerminalLastError` 写入 `RequestState.LastError`。
  - 对 client disconnect 来说，常见 `Err()` 是 `context.Canceled`。

### 3) Metrics 持久化：StatusCode / ErrorInfo 来自 RequestState

- `internal/metricsruntime/usage_plugin.go`
  - 从 `RequestStateSnapshot` 取 `StatusCode` 与 `LastError`，分别写入 `MetricRecord.StatusCode` 与 `MetricRecord.ErrorInfo`。
  - 这意味着：只要 `StatusCode` 还是 200 且 `LastError` 为空，就会在 Query API 中被当作 success。

### 4) Query API：success/failure 二分逻辑

- `internal/api/handlers/management/metrics.go`
  - percentiles：`isSuccess := status_code in [200,300) && error_info empty`；否则 failure。
  - buckets：SQL 里用 `success_flag_case` 做同样二分。
  - 当前输出结构只有 `success` 和 `failure` 两条路径，没有 canceled。

### 5) Handler 结束统一 SetStatusCode，可能覆盖“中途标记”

- 例如 `sdk/api/handlers/openai/openai_handlers.go`：流式分支在 `handleStreamingResponse` 之后执行 `state.SetStatusCode(c.Writer.Status())`。
  - 如果 client 在流式中断连，`c.Writer.Status()` 仍可能是 200（headers 已提交），从而误判 success。

### 6) ResponseWriterWrapper：Write/WriteString 能拿到写入 error，但当前不记录

- `internal/api/middleware/response_writer.go`
  - `Write` / `WriteString` 返回 `(n, err)`，但 wrapper 目前只把 err 透传，不做状态记录。
  - `Finalize(c)` 在 `c.Next()` 之后执行，并能访问 gin.Context（也就能访问 RequestState）。

## Recommended Implementation Strategy (与 CONTEXT 对齐的最小改动路径)

> 下面是研究层的推荐实现方向；具体拆分为多少 plan、如何组织任务交给 planner。

### A. canceled 信号的“单一真相”落点

优先把“是否 canceled”沉淀到 `metricsruntime.RequestState`（或其 snapshot）中，让 `usage_plugin.go` 成为落库的唯一入口。

原因：
- 现有持久化字段来自 RequestState；把 canceled 也放在 RequestState 能保持单一数据流。
- 避免在多个 handler/executor scattered 地做 DB 写入判断。

### B. canceled 的持久化表达（避免复用 error_info）

受限于现有 schema（只有 `status_code` / `error_info`），且 CONTEXT 锁定了“per-request canceled 时 error 为空”，因此不要通过设置 `LastError` 来表达 canceled。

两条候选路径（planner 需要选择其一，并在计划中锁死）：

1) **无 schema 变更（最小）：使用稳定的 status_code 表达 canceled**
   - 例如使用 `499` (Client Closed Request) 表示 canceled。
   - Query API 将 `status_code == 499` 识别为 canceled：
     - percentiles：遇到 499 计入 canceled_count，但不进入 success/failure 的 percentiles 样本。
     - buckets：按 bucket 聚合时计算 `canceled_count`（`SUM(CASE WHEN status_code=499 THEN 1 ELSE 0 END)`），同时 success/failure 的 count 按原规则。
   - 需要保证 handler 不会把 499 覆盖回 200（见下文 D）。

2) **小幅 schema 变更：增加 canceled 标记列**
   - 例如 `ALTER TABLE metrics ADD COLUMN canceled INTEGER NOT NULL DEFAULT 0;`
   - Query API 使用 canceled 列做三分法；旧数据默认 canceled=0，不需要 backfill。
   - 代价：需要 migration + writer insert SQL + types 变更。

考虑到 Phase 09 允许破坏性变更但不希望引入新能力，上述两条都符合边界；建议 planner 在计划中明确选择，并补测试锁死。

### C. client cancel/disconnect 的检测点

要覆盖 streaming 与 non-streaming 两条路径（CONTEXT: 必须覆盖）。推荐组合使用：

1) **Streaming：在 `ForwardStream` 的 request context Done 分支标记 canceled**
   - `sdk/api/handlers/stream_forwarder.go` 已有 `case <-c.Request.Context().Done()`。
   - 在该分支里调用一个小函数（例如 `metricsruntime.MarkClientCanceled(c, c.Request.Context().Err())`）来更新 RequestState。
   - 需要区分 timeout vs canceled：
     - `context.DeadlineExceeded` -> failure
     - `context.Canceled` -> canceled（conservative）

2) **Non-streaming（以及补充 streaming）：在 ResponseWriterWrapper 记录 write error 并在 Finalize 标记 canceled**
   - 在 `internal/api/middleware/response_writer.go` 的 `Write/WriteString` 中，如果 `err != nil`，记录一个布尔或保存错误类型（例如 `writeErr`）。
   - 在 `Finalize(c)`（它发生在 handler 完成之后）读取 gin context 的 RequestState 并做“最终 outcome”补正：
     - 若已是 failure（非 2xx 或 LastError 非空）则不标 canceled。
     - 否则若 write error 发生，则标 canceled。
   - 这条路径的优势是：它能区分“服务端已自然结束写完响应”与“写过程中写失败”。

### D. 防止 canceled 被 handler 的 SetStatusCode 覆盖

因为多个 handler 在结束时会调用 `state.SetStatusCode(c.Writer.Status())`，如果 canceled 用 status_code 表达，需要在 RequestState 层提供“不会被覆盖”的能力：

- 方案 1：新增 `SetStatusCodeIfUnset`（或 `SetFinalStatusCode`）并逐个 handler 替换调用点。
- 方案 2：在现有 `SetStatusCode` 内部加入保护：当 `StatusCode` 已被标记为 canceled 的特定值时，不用 200 覆盖（但允许用非 2xx 覆盖）。

这类保护应尽量集中在 `metricsruntime` 内，避免在每个 handler 写分支逻辑。

## Testing Strategy (必须新增/修改的测试)

### 1) Unit tests: RequestState / canceled 标记不可被覆盖

- 在 `internal/metricsruntime/request_state_test.go`（或新增测试文件）里锁定：
  - 标记 canceled 后，再调用 `SetStatusCode(200)` 不应把状态覆盖为 success。
  - 若后续出现明确 failure（例如 500 或 LastError），failure 应优先（CONTEXT: upstream error = failure）。

### 2) Contract tests: Management metrics 输出口径

扩展 `test/metrics_management_test.go`（已有 percentiles/buckets contract）：

- percentiles：
  - 数据集中混入 canceled 行（按你选择的表达：status_code=499 或 canceled=1）。
  - 断言：percentiles 计算不包含 canceled 样本；但响应里能看到 `canceled_count`（meta 或 group 维度，具体由 planner 设计）。

- buckets：
  - 断言：每个 bucket 都有 `canceled_count`，且 success/failure 计数与 canceled 分离。

- request_id：
  - canceled 行返回中 `outcome`/`status` 为 canceled，且 `error_info` 为空。

### 3) E2E-ish 测试：模拟 client disconnect（尽量稳定、避免 wall-clock）

- streaming：可以在 `sdk/api/handlers/stream_forwarder_test.go` 中创建一个带 cancel 的 request context，触发 `ForwardStream` 的 `Done()` 分支，断言 RequestState 被标记。
- non-streaming：可以通过 ResponseWriterWrapper 的 `Write` 返回 error 来模拟（实现一个返回固定 error 的 fake ResponseWriter），断言 Finalize 后 RequestState 被标记为 canceled。

## Risks / Pitfalls

- **Gin 的 request context 取消原因不总是可区分**：`GetContextWithCancel` 当前会在 goroutine 内对 requestCtx.Done() 只调用 `cancel()`，不携带 cause（Go 1.20 的 WithCancelCause 未使用）。因此“只看 ctx.Err()”可能把 timeout 与 cancel 混淆；建议尽量使用 `c.Request.Context().Err()` 或通过 write error 捕捉。
- **headers 已提交后 status 可能恒为 200**：流式响应中途断连时 `c.Writer.Status()` 仍可能是 200；必须通过显式标记或 write error 纠正。
- **对外破坏性变更**：CONTEXT 允许原地改 `/v0/management/metrics`，但必须同步更新 tests/README（本阶段规划应包含）。

## What Planner Should Derive Into Plans

- 计划需要明确选择“canceled 的持久化表达”（status_code=499 vs schema 新列），并把 Query API/测试按该选择锁死。
- 计划需要把 cancel/disconnect detection 收敛到 1-2 个中心点（ForwardStream + ResponseWriterWrapper/Finalize），避免在每个 provider handler scattered。
- 输出契约需要落实到具体 JSON shape（尤其 buckets 的 `canceled_count` 与 percentiles 的 `canceled_count` meta 放置位置），并用 contract tests 锁死。
