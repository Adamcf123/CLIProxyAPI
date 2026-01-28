# Architecture

**Analysis Date:** 2025-01-29

## Pattern Overview

**Overall:** Layered Proxy Architecture with Plugin System

**Key Characteristics:**
- Multi-protocol API proxy supporting OpenAI, Gemini, Claude, and custom formats
- Provider-agnostic authentication layer with OAuth and API key support
- Bidirectional request/response translation between API formats
- Modular executor pattern for provider-specific HTTP execution
- File-based configuration and token storage with hot-reload
- Usage tracking and quota management per credential

## Layers

**API Layer (`internal/api/`, `sdk/api/`):**
- Purpose: HTTP server, routing, and middleware for incoming API requests
- Location: `internal/api/server.go`, `sdk/api/handlers/`
- Contains: Gin-based HTTP server, route handlers, auth middleware, CORS
- Depends on: SDK handlers, access manager, config
- Used by: Main entry point, embedded SDK consumers

**Handler Layer (`sdk/api/handlers/`):**
- Purpose: Protocol-specific request handling (OpenAI, Gemini, Claude)
- Location: `sdk/api/handlers/openai/`, `sdk/api/handlers/gemini/`, `sdk/api/handlers/claude/`
- Contains: Chat completions, models list, streaming response forwarding
- Depends on: Base API handler, translator pipeline, executor
- Used by: API layer router

**Authentication Layer (`sdk/auth/`, `sdk/cliproxy/auth/`, `internal/auth/`):**
- Purpose: Credential management, token refresh, provider authentication
- Location: `sdk/auth/manager.go`, `sdk/cliproxy/auth/conductor.go`
- Contains: OAuth flows, token storage, credential selection, quota cooldown
- Depends on: Token store (file/git/postgres/object), provider-specific auth
- Used by: Handler layer, access manager

**Access Control Layer (`sdk/access/`):**
- Purpose: Request authentication and provider lookup
- Location: `sdk/access/manager.go`
- Contains: API key validation, provider registry, principal resolution
- Depends on: Auth manager, config
- Used by: API middleware

**Translation Layer (`sdk/translator/`, `internal/translator/`):**
- Purpose: Schema conversion between different AI API formats
- Location: `sdk/translator/pipeline.go`, `internal/translator/`
- Contains: Request/response translators, format registry
- Depends on: Provider-specific translators
- Used by: Executors

**Executor Layer (`internal/runtime/executor/`):**
- Purpose: Provider-specific HTTP execution with retry and error handling
- Location: `internal/runtime/executor/`
- Contains: HTTP clients, request signing, response parsing
- Depends on: Auth credentials, config
- Used by: Core auth manager

**Core Runtime Layer (`sdk/cliproxy/auth/conductor.go`):**
- Purpose: Orchestrates auth lifecycle, executor binding, request execution
- Location: `sdk/cliproxy/auth/conductor.go`
- Contains: Credential registration, model registry, selector strategy
- Depends on: Executors, token store, config
- Used by: Service layer

**Configuration Layer (`internal/config/`, `sdk/config/`):**
- Purpose: YAML configuration loading, validation, and hot-reload
- Location: `internal/config/config.go`, `sdk/config/config.go`
- Contains: Config structs, sanitization, migration, persistence
- Depends on: YAML parser
- Used by: All layers

**Storage Layer (`internal/store/`, `sdk/auth/filestore.go`):**
- Purpose: Token and config persistence (file, git, postgres, S3)
- Location: `internal/store/`, `sdk/auth/`
- Contains: Abstract token store interface, implementations
- Depends on: File system, git, postgres, S3 clients
- Used by: Auth manager

**Model Registry (`internal/registry/`):**
- Purpose: Global model catalog with availability tracking
- Location: `internal/registry/`
- Contains: Static model definitions, client registration, availability queries
- Depends on: Auth credentials
- Used by: Handlers, service layer

## Data Flow

**Incoming Request:**

1. HTTP request arrives at Gin router (`internal/api/server.go`)
2. CORS middleware and auth middleware (`sdk/access/manager.Authenticate`)
3. API key resolved to principal and provider
4. Route handler selected (OpenAI/Gemini/Claude)
5. Handler creates execution context with auth metadata

**Request Translation:**

6. Handler invokes `BaseAPIHandler.ExecuteWithTranslator`
7. Core auth manager selects credential (round-robin or fill-first)
8. Translator pipeline transforms request to provider format
9. Thinking suffix injection (if configured)

**Execution:**

10. Executor signs and sends HTTP request to provider
11. Retry on transient errors with exponential backoff
12. Quota cooldown on rate limits

**Response Processing:**

13. Stream chunks or non-stream response
14. Translator converts response to client format
15. Response rewriter applies mutations (Amp model mapping)
16. Handler forwards response to client

**State Management:**
- Config and auth files watched for changes (`internal/watcher/`)
- Hot-reload triggers rebind of executors and model registry updates
- Usage statistics tracked in memory per credential

## Key Abstractions

**Auth (`sdk/cliproxy/auth/types.go`):**
- Purpose: Represents a single credential (OAuth token or API key)
- Examples: `internal/auth/`, `sdk/auth/gemini.go`
- Pattern: Provider-specific auth with common metadata (ID, status, attributes)

**Executor (`internal/runtime/executor/`):**
- Purpose: Provider-specific HTTP execution
- Examples: `internal/runtime/executor/gemini_executor.go`
- Pattern: One executor per provider type, bound to auth entries

**Translator (`sdk/translator/types.go`):**
- Purpose: Bidirectional schema conversion
- Examples: `internal/translator/openai/openai/gemini/`
- Pattern: RequestTransform and ResponseTransform functions registered by (source, target) format

**TokenStore (`sdk/auth/interfaces.go`):**
- Purpose: Abstract credential persistence
- Examples: `sdk/auth/filestore.go`, `internal/store/postgres_store.go`
- Pattern: Read/write/delete auth JSON files with transaction support

**ModelRegistry (`internal/registry/`):**
- Purpose: Track which models are available from which credentials
- Examples: `sdk/cliproxy/model_registry.go`
- Pattern: Client ID to model list mapping with availability queries

## Entry Points

**CLI Entry Point:**
- Location: `cmd/server/main.go`
- Triggers: Binary execution with flags (--login, --codex-login, etc.)
- Responsibilities: Config loading, store selection, login flows, or server start

**Service Entry Point (SDK):**
- Location: `sdk/cliproxy/service.go`
- Triggers: Embedded via `service.Run(ctx)`
- Responsibilities: Auth manager init, file watcher, HTTP server, graceful shutdown

**Management API:**
- Location: `internal/api/handlers/management/handler.go`
- Triggers: HTTP requests to `/v0/management/*`
- Responsibilities: Config CRUD, auth file upload/download, usage stats, OAuth initiation

## Error Handling

**Strategy:** Provider-specific error classification with retry policies

**Patterns:**
- Transient errors (rate limits, network): Retry with exponential backoff
- Auth errors (401/403): Disable credential, skip to next in rotation
- Quota exceeded: Switch project or preview model if configured
- Validation errors: Fail fast with 400 response

**Quota Management:**
- Cooldown period per credential after rate limit
- Auto-refresh tokens before expiry (15-minute interval)
- Status tracking (active, disabled, error)

## Cross-Cutting Concerns

**Logging:** Structured logging with logrus (request ID correlation, file rotation)
**Validation:** Config sanitization on load, model ID normalization
**Authentication:** Multi-provider OAuth (Claude, Codex, Gemini, Qwen, iFlow, Antigravity)
**Usage Tracking:** In-memory token/usage aggregation per credential
**File Watching:** fsnotify-based hot-reload for config and auth directory changes
**Management UI:** Bundled HTML asset fetched from GitHub releases

---

*Architecture analysis: 2025-01-29*
