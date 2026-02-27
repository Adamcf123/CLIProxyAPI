# Claude Beta 字符串参考记录

## 文档目的

本文档是 `internal/runtime/executor/claude_executor.go` 中 OAuth 默认 beta 字符串（`baseBetas`）的**手动更新参考记录**。

由于 Anthropic 会随 Claude Code CLI 版本更新 `Anthropic-Beta` header 的值，而本代理需要在 OAuth 模式下主动构造该 header，因此需要定期通过抓包方式获取最新值并更新源码。

---

## 更新流程

### 第一步：使用 mitmdump 拦截流量

启动 mitmdump，将 Claude Code CLI 的 HTTPS 流量代理到本地：

```bash
mitmdump -p 8080 --ssl-insecure -w claude_traffic.mitm 'host("api.anthropic.com")'
```

配置 CLI 走代理：

```bash
export HTTPS_PROXY=http://127.0.0.1:8080
export NODE_EXTRA_CA_CERTS=/path/to/mitmproxy-ca-cert.pem
claude ...
```

### 第二步：找到目标请求

在捕获的流量中找到：

```
POST https://api.anthropic.com/v1/messages
```

**注意**：只看 `POST /v1/messages` 的请求，忽略 `GET` 类请求（其 beta header 通常不同或缺失）。

### 第三步：提取 `Anthropic-Beta` header 值

从该 POST 请求的请求头中提取 `Anthropic-Beta` 字段的完整值，例如：

```
Anthropic-Beta: claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,...
```

### 第四步：更新 `claude_executor.go`

找到文件约第 947 行的 `baseBetas` 默认赋值：

```go
promptCachingBeta := "prompt-caching-2024-07-31"
baseBetas := "claude-code-20250219,oauth-2025-04-20,..." + promptCachingBeta
```

用新值替换 `baseBetas` 的字符串字面量部分（`+` 拼接的 `promptCachingBeta` 部分见下一步）。

### 第五步：检查 `promptCachingBeta` 逻辑是否需要调整

检查抓到的新 beta 字符串中是否已包含 `prompt-caching` 相关项（如 `prompt-caching-scope-2026-01-05`）：

- 若新字符串中**已包含** `prompt-caching-2024-07-31` 的等效替代，则需评估是否应将 `promptCachingBeta` 常量及其 `append` 逻辑（第 954-956 行）一并更新或移除，避免重复拼接。
- 若新字符串中**仍包含** `prompt-caching-2024-07-31`，则保持现有逻辑不变。

```go
// 当前的 append 逻辑（若新版不再需要该 beta 则考虑移除）
if !strings.Contains(baseBetas, promptCachingBeta) {
    baseBetas += "," + promptCachingBeta
}
```

---

## 版本记录表

| CLI 版本 | 抓包日期   | Anthropic-Beta                                                                                                                                     | 备注                          |
|----------|------------|----------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------|
| 2.1.62   | 2026-02-27 | `claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,adaptive-thinking-2026-01-28` | mitmdump 实测，POST /v1/messages |
