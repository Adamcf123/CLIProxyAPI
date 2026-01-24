# Codebase Structure

**Analysis Date:** 2026-01-29

## Directory Layout

```
CLIProxyAPI/
├── cmd/                    # Application entry points
│   └── server/             # Main server binary entry point
├── internal/               # Private implementation packages
│   ├── access/             # Request authentication providers
│   ├── api/                # HTTP API server and handlers
│   │   ├── handlers/       # Request handlers
│   │   ├── middleware/     # HTTP middleware
│   │   └── modules/        # Modular API features
│   ├── auth/               # OAuth authentication implementations
│   ├── config/             # Configuration loading and management
│   ├── logging/            # Logging infrastructure
│   ├── managementasset/    # Management UI asset management
│   ├── registry/           # Model registry and static definitions
│   ├── runtime/            # Runtime executors
│   ├── store/              # Token storage backends
│   ├── translator/         # Request/response schema translators
│   ├── thinking/           # Thinking mode support
│   ├── usage/              # Usage tracking
│   ├── watcher/            # File system watching
│   └── wsrelay/            # WebSocket relay for AI Studio
├── sdk/                    # Public SDK for embedding
│   ├── access/             # Access control interfaces
│   ├── api/                # API handler implementations
│   ├── auth/               # Authentication SDK
│   ├── cliproxy/           # Core service SDK
│   │   ├── auth/           # Core auth manager
│   │   ├── executor/       # Executor interfaces
│   │   ├── pipeline/       # Execution pipeline
│   │   └── usage/          # Usage tracking SDK
│   ├── config/             # SDK configuration types
│   └── translator/         # Translator SDK
├── docs/                   # Documentation
├── examples/               # Usage examples
├── test/                   # Test files
└── auths/                  # OAuth token storage directory
```

## Directory Purposes

**cmd/server:**
- Purpose: Entry point for the CLI Proxy API server binary
- Contains: Single `main.go` with flag parsing and service initialization
- Key files: `main.go`

**internal/api:**
- Purpose: HTTP API server implementation using Gin framework
- Contains: Server setup, routing, management handlers, middleware
- Key files: `server.go`, `handlers/management/handler.go`, `middleware/`

**internal/auth:**
- Purpose: OAuth 2.0 authentication implementations for various providers
- Contains: Provider-specific auth flows (Claude, Codex, Gemini, Qwen, iFlow, Antigravity)
- Key files: `claude/anthropic_auth.go`, `codex/openai_auth.go`, `gemini/gemini_auth.go`

**internal/translator:**
- Purpose: Request/response schema transformation between API formats
- Contains: Matrix of translator implementations (source_format -> target_format)
- Key files: `init.go`, `gemini/openai/chat-completions/`, `claude/gemini/`

**internal/config:**
- Purpose: Configuration file loading and management
- Contains: YAML parsing, validation, migration, persistence
- Key files: `config.go`, `oauth_model_alias_migration.go`

**internal/runtime/executor:**
- Purpose: Provider-specific request execution logic
- Contains: Executors for each AI provider
- Key files: `executor.go`, provider-specific executor files

**internal/registry:**
- Purpose: Static model definitions and model registry
- Contains: Model metadata, thinking support flags, static definitions
- Key files: `registry.go`, `models.go`

**internal/watcher:**
- Purpose: File system monitoring for config and auth changes
- Contains: Config file watcher, auth directory watcher, diff detection
- Key files: `watcher.go`, `synthesizer/`

**internal/store:**
- Purpose: Token storage backend implementations
- Contains: File, Git, PostgreSQL, Object storage backends
- Key files: `filestore.go`, `postgres.go`, `gitstore.go`, `objectstore.go`

**sdk/cliproxy:**
- Purpose: Public SDK for embedding the proxy in other applications
- Contains: Service lifecycle, auth manager, executor interfaces, pipeline
- Key files: `service.go`, `auth/conductor.go`, `pipeline/context.go`

**sdk/auth:**
- Purpose: Authentication SDK for token management
- Contains: Token store interface, OAuth authenticators, refresh logic
- Key files: `manager.go`, `filestore.go`, `claude.go`

**sdk/translator:**
- Purpose: Translator SDK for schema transformation
- Contains: Pipeline, registry, format definitions, built-in translators
- Key files: `pipeline.go`, `registry.go`, `builtin/builtin.go`

## Key File Locations

**Entry Points:**
- `cmd/server/main.go`: Main server binary entry point with flag parsing and service startup

**Configuration:**
- `internal/config/config.go`: Configuration struct and loading logic
- `config.yaml`: Runtime configuration file
- `config.example.yaml`: Configuration template

**Core Logic:**
- `sdk/cliproxy/service.go`: Core service lifecycle and orchestration
- `sdk/cliproxy/auth/conductor.go`: Core auth manager with selection and execution
- `internal/api/server.go`: HTTP server setup and routing
- `internal/translator/init.go`: Translator initialization and registration

**Authentication:**
- `sdk/auth/manager.go`: Token store and OAuth authenticator management
- `internal/auth/claude/anthropic_auth.go`: Claude OAuth implementation
- `internal/auth/codex/openai_auth.go`: Codex OAuth implementation
- `internal/auth/gemini/gemini_auth.go`: Gemini OAuth implementation

**API Handlers:**
- `sdk/api/handlers/handlers.go`: Base API handler with request routing
- `sdk/api/handlers/openai/openai_handlers.go`: OpenAI-compatible handlers
- `sdk/api/handlers/gemini/gemini_handlers.go`: Gemini-compatible handlers
- `sdk/api/handlers/claude/code_handlers.go`: Claude Code handlers

**Model Registry:**
- `internal/registry/registry.go`: Model registry implementation
- `internal/registry/models.go`: Static model definitions

**Testing:**
- `test/`: Integration test files

## Naming Conventions

**Files:**
- `snake_case.go`: All Go source files use snake_case
- `*_test.go`: Test files
- `init.go`: Package initialization files (translator registration)

**Directories:**
- `lowercase`: All directories use lowercase
- Provider names in subdirectories: `claude`, `codex`, `gemini`, `qwen`, `iflow`, `antigravity`

**Package Names:**
- `lowercase`: All package names use lowercase
- Single-word preferred, underscores for compound names

**Types:**
- `PascalCase`: All exported types use PascalCase
- Interfaces: `PascalCase` (e.g., `ModelRegistry`, `TokenStore`)

**Functions:**
- `PascalCase`: Exported functions
- `camelCase`: Unexported functions

## Where to Add New Code

**New Feature:**
- Primary code: `internal/` for private implementation
- SDK code: `sdk/` if it should be publicly accessible
- Tests: `internal/<package>/<file>_test.go`

**New Provider:**
- Implementation: `internal/auth/<provider>/`
- Executors: `internal/runtime/executor/<provider>_executor.go`
- Translators: `internal/translator/<provider>/` (matrix of target formats)
- SDK support: `sdk/auth/<provider>.go`

**New Component/Module:**
- API module: `internal/api/modules/<module>/`
- Management handler: `internal/api/handlers/management/<module>.go`

**Utilities:**
- Shared helpers: `internal/misc/` or `internal/util/`
- SDK utilities: `sdk/` appropriate subdirectory

**New Translator Format:**
- Implementation: `internal/translator/<source_format>/<target_format>/`
- Registry: Add to `internal/translator/init.go`
- SDK: Add to `sdk/translator/builtin/builtin.go`

## Special Directories

**auths:**
- Purpose: Runtime storage for OAuth token files
- Generated: Yes (by OAuth flows)
- Committed: No (local runtime data)

**logs:**
- Purpose: Application log files
- Generated: Yes
- Committed: No

**.planning:**
- Purpose: Project planning and codebase documentation
- Generated: No
- Committed: Yes

**static:**
- Purpose: Static assets for management UI
- Generated: No
- Committed: Yes

**examples:**
- Purpose: Usage examples and sample code
- Generated: No
- Committed: Yes

---

*Structure analysis: 2026-01-29*