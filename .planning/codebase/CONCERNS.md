# Codebase Concerns

**Analysis Date:** 2025-01-29

## Tech Debt

**Hardcoded OAuth Credentials:**
- Issue: OAuth client credentials are hardcoded in source files rather than loaded from configuration
- Files: `internal/runtime/executor/antigravity_executor.go:46-47`
  - `antigravityClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"`
  - `antigravityClientSecret = "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"`
- Impact: Security risk if code is exposed; difficult to rotate credentials; credentials visible in version control
- Fix approach: Move to environment variables or secure configuration with proper secrets management

**Legacy Tier References:**
- Issue: References to "legacy-tier" hardcoded in auth logic
- Files: `internal/auth/antigravity/auth.go:218`
- Impact: Fragile tier management; difficult to evolve pricing/tier system
- Fix approach: Define tier system in configuration with proper model mapping

**Excessive Empty Return Statements:**
- Issue: Widespread use of empty returns (`return []byte{}`, `return []string{}`, `return nil`) without explicit error handling
- Files: Found in 100+ files across translators and handlers
- Impact: Errors may be silently swallowed; difficult to debug; violates fail-loud principles
- Fix approach: Return explicit errors or document why empty result is valid; use typed errors

**Compatibility Comments Without Implementation:**
- Issue: Multiple files claim to maintain compatibility but have scattered conditional logic
- Files: Multiple translator files mention "Claude-compatible", "Gemini-compatible", "OpenAI-compatible"
- Impact: Risk of breaking compatibility when changing core logic; spaghetti compatibility checks
- Fix approach: Centralize compatibility layer at entry points with explicit versioning

## Known Bugs

**OAuth Callback Race Condition:**
- Symptoms: OAuth callbacks may fail under concurrent load due to sleep-based synchronization
- Files: `internal/auth/codex/oauth_server.go:103`, `internal/auth/iflow/oauth_server.go:72`, `internal/auth/claude/oauth_server.go:106`
  - All use `time.Sleep(100 * time.Millisecond)` without proper synchronization
- Trigger: Multiple concurrent OAuth authorization flows
- Workaround: None; assumes sequential OAuth flows
- Fix approach: Use proper async channels or mutex for callback coordination

**context.Background() Usage in Request Paths:**
- Symptoms: Cancellation and timeouts may not propagate correctly; goroutine leaks possible
- Files:
  - `sdk/api/handlers/handlers.go:243` - `parentCtx = context.Background()`
  - `sdk/api/handlers/handlers.go:326` - `ctx = context.Background()`
  - Multiple auth files use `context.Background()` in request handlers
- Trigger: Long-running requests or client disconnections
- Workaround: None
- Fix approach: Always derive from request context; never use `context.Background()` in request paths

**Error Swallowing in WebSocket Handler:**
- Symptoms: Errors in WebSocket message handling are silently ignored
- Files: `internal/wsrelay/http.go:163` - `_ = send(StreamEvent{Type: MessageTypeError, Err: decodeError(msg.Payload)})`
- Trigger: Malformed WebSocket messages
- Workaround: None; errors only appear in logs if enabled
- Fix approach: Log errors explicitly or propagate to client

## Security Considerations

**HTTP Client Timeout Misconfiguration:**
- Risk: Requests may hang indefinitely if no timeout is set
- Files: Multiple files use `http.DefaultClient` directly
  - `internal/util/ssh_helper.go:41`
  - `sdk/auth/filestore.go:200`
  - `internal/api/modules/amp/proxy_test.go:346`
- Impact: Denial-of-service via slow upstream responses; resource exhaustion
- Current mitigation: None visible
- Recommendations: Use configured HTTP client with timeouts (connect, read, idle)

**OAuth Token Storage in Files:**
- Risk: OAuth tokens stored as plaintext files on disk
- Files: `sdk/auth/filestore.go`, `internal/store/postgresstore.go`
  - Tokens written to auth files with `0o700` permissions
- Impact: Tokens readable by process owner; no encryption at rest
- Current mitigation: File permissions only
- Recommendations: Encrypt tokens at rest; use system keyring; implement token rotation

**Management API on All Interfaces:**
- Risk: Management endpoints bound to all interfaces by default
- Files: `internal/api/server.go`
- Impact: Exposure of sensitive management operations if firewall fails
- Current mitigation: localhost restriction in some paths; management key required
- Recommendations: Bind management API to localhost by default; require explicit config for remote access

**Missing Input Validation:**
- Risk: User-controlled values passed directly to external services without validation
- Files: Various translators and executors
- Impact: Potential injection attacks; upstream resource exhaustion
- Current mitigation: Relies on upstream validation
- Recommendations: Validate length, charset, and structure before proxying

## Performance Bottlenecks

**Synchronous File I/O in Request Path:**
- Problem: Config and auth file reads occur on request hot path
- Files: `sdk/cliproxy/auth/conductor.go:2232` - Large conductor file with complex auth logic
- Cause: File-based store without caching layer
- Improvement path: Implement in-memory cache with file watcher invalidation

**Unbounded JSON Parsing:**
- Problem: Large API responses parsed entirely into memory
- Files: `internal/logging/request_logger.go:1227` - Large request logger
- Cause: Streaming not fully utilized for response logging
- Improvement path: Stream JSON parsing for large payloads; implement size limits

**Goroutine Leaks from Background Contexts:**
- Problem: `context.Background()` usage prevents proper cancellation
- Files: 45+ instances across codebase
- Cause: Background contexts don't respect request cancellation
- Improvement path: Replace all `context.Background()` with properly-scoped contexts

**Database Connection Pooling Not Configured:**
- Problem: PostgreSQL store uses default connection pool settings
- Files: `internal/store/postgresstore.go:83` - `db, err := sql.Open("pgx", cfg.DSN)`
- Cause: No `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` calls
- Improvement path: Configure pool based on workload; implement connection health checks

**Sleep-Based Polling:**
- Problem: OAuth token refresh and file watching use sleep-based polling
- Files:
  - `internal/auth/qwen/qwen_auth.go:241-273` - Multiple `time.Sleep(pollInterval)`
  - `internal/watcher/events.go:99` - `time.Sleep(replaceCheckDelay)`
- Cause: No event-driven mechanism
- Improvement path: Use filesystem events (inotify) for file watching; implement exponential backoff for retries

## Fragile Areas

**OAuth Model Aliases:**
- Files: `sdk/cliproxy/auth/oauth_model_alias.go`, `internal/config/oauth_model_alias_migration.go`
- Why fragile: Complex mapping logic between OAuth tokens and model permissions; multiple code paths
- Safe modification: Add integration tests for all OAuth providers before changes; test model alias resolution
- Test coverage: Some test files exist (`sdk/cliproxy/auth/oauth_model_alias_test.go`), but may not cover edge cases

**Auth Conductor:**
- Files: `sdk/cliproxy/auth/conductor.go:2232` (2798 lines - largest file)
- Why fragile: Monolithic file handling all auth logic; high cyclomatic complexity; multiple concerns mixed
- Safe modification: Break into smaller packages by provider; extract common patterns; add contract tests
- Test coverage: Test files exist but may not cover all provider combinations

**Config Reload Logic:**
- Files: `internal/watcher/dispatcher.go`, `internal/watcher/config_reload.go`
- Why fragile: Complex diffing logic with `reflect.DeepEqual`; race conditions possible during hot reload
- Safe modification: Add comprehensive reload tests; use explicit comparison functions instead of reflection
- Test coverage: `internal/watcher/watcher_test.go:1490` exists but extensive

**Translation Layer:**
- Files: Multiple translator packages with similar patterns (`internal/translator/*/`)
- Why fragile: Boilerplate code across providers; format changes require touching many files
- Safe modification: Extract common translation logic; use code generation for boilerplate
- Test coverage: Individual translator tests exist but no cross-provider compatibility tests

**Request Logger:**
- Files: `internal/logging/request_logger.go:1227` (large file with complex compression handling)
- Why fragile: Handles multiple compression formats (brotli, gzip, zstd, flate); complex error paths
- Safe modification: Add tests for all compression combinations; simplify error handling
- Test coverage: Limited visible test coverage for compression edge cases

## Scaling Limits

**Single-Process Architecture:**
- Current capacity: Limited to single process; no horizontal scaling
- Limit: Memory and CPU of single machine; no request distribution
- Scaling path: Extract state to external store (Redis, PostgreSQL); implement distributed locking

**File-Based Auth Storage:**
- Current capacity: Limited by file system performance; no concurrent access optimization
- Limit: File system I/O becomes bottleneck; auth resolution slows with many keys
- Scaling path: Migrate to database-backed auth with caching; implement auth key sharding

**In-Memory Usage Statistics:**
- Current capacity: Usage data stored in memory only
- Limit: Memory grows with request volume; data lost on restart
- Scaling path: Stream usage to external time-series database; implement periodic aggregation

**No Request Queuing:**
- Current capacity: All requests handled immediately by goroutines
- Limit: Goroutine explosion under load; memory exhaustion
- Scaling path: Implement worker pool with bounded concurrency; request prioritization

## Dependencies at Risk

**GitHub Direct Imports in Examples:**
- Risk: `github.com/andybalholm/brotli` and other packages may become unavailable
- Impact: Examples break; build fails
- Migration plan: Vendor critical dependencies; implement fallback decompression

**Go Version Requirement:**
- Risk: `go 1.24.0` specified in go.mod (very recent)
- Impact: Requires recent Go toolchain; may not be available in all environments
- Migration plan: Pin to stable Go version; test on earlier versions; document requirements

**PostgreSQL Dependency:**
- Risk: `github.com/jackc/pgx/v5` for optional PostgreSQL store
- Impact: Cannot run without PostgreSQL when configured
- Migration plan: Implement SQLite fallback for development; document database requirements

**Git Dependency for Config Storage:**
- Risk: `github.com/go-git/go-git/v6` used for Git-backed config
- Impact: Git operations may fail or be slow; repository corruption possible
- Migration plan: Implement file-based fallback; add health checks for git operations

## Missing Critical Features

**Request Rate Limiting:**
- Problem: No per-client or per-key rate limiting
- Blocks: Protection against abuse; fair resource allocation
- Impact: Single client can exhaust resources; DoS vulnerability

**Circuit Breaker Pattern:**
- Problem: No circuit breaker for failing upstream services
- Blocks: Graceful degradation; automatic recovery
- Impact: Cascading failures when upstream degrades; requests queue indefinitely

**Distributed Tracing:**
- Problem: No distributed tracing integration (OpenTelemetry, Jaeger)
- Blocks: Debugging multi-hop requests; performance analysis
- Impact: Difficult to trace request flow through multiple services

**Health Check Endpoints:**
- Problem: No standardized health check endpoints (`/healthz`, `/readyz`)
- Blocks: Kubernetes readiness/liveness probes; load balancer health checks
- Impact: Difficult to run in orchestrated environments; rolling deployments risky

**Metrics Export:**
- Problem: No Prometheus/statsd metrics export
- Blocks: Monitoring and alerting; performance analysis
- Impact: Operational visibility limited to logs; cannot set up automated alerting

## Test Coverage Gaps

**Integration Tests for OAuth Flows:**
- What's not tested: Full OAuth flows with real providers; token refresh edge cases
- Files: `internal/auth/*/` - Individual provider auth packages
- Risk: OAuth changes break without detection; token refresh failures in production
- Priority: High - OAuth is critical for authentication

**Error Path Testing:**
- What's not tested: All error return paths; failure modes
- Files: Throughout codebase, especially translators and executors
- Risk: Errors not handled correctly; unexpected panics in production
- Priority: High - Failures are inevitable

**Concurrent Access Testing:**
- What's not tested: Race conditions in auth selector; concurrent config reload
- Files: `sdk/cliproxy/auth/selector.go`, `internal/watcher/dispatcher.go`
- Risk: Data races; deadlocks; corrupted state under load
- Priority: High - Concurrent access is common

**Translator Compatibility Testing:**
- What's not tested: Cross-provider format compatibility; version mismatches
- Files: `internal/translator/*/` - All translator packages
- Risk: Format changes break silently; unexpected responses to clients
- Priority: Medium - API compatibility is critical

**Load Testing:**
- What's not tested: System behavior under sustained load; memory leak detection
- Files: Entire system
- Risk: Performance regressions; resource exhaustion in production
- Priority: Medium - Scaling issues discovered late

**Security Testing:**
- What's not tested: Input validation; authentication bypass; injection attacks
- Files: All request handlers
- Risk: Security vulnerabilities; unauthorized access
- Priority: High - Security is critical for API proxy

---

*Concerns audit: 2025-01-29*
