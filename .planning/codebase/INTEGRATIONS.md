# External Integrations

**Analysis Date:** 2026-01-29

## APIs & External Services

**OAuth2 Authentication Providers:**
- Google Gemini CLI - OAuth2 device/code flow for Gemini API access
  - SDK/Client: golang.org/x/oauth2
  - Auth flow: Device authorization grant
  - Config: `internal/auth/gemini/`, `sdk/auth/gemini.go`

- OpenAI Codex - OAuth2 for Codex/GPT models access
  - SDK/Client: golang.org/x/oauth2
  - Auth flow: PKCE OAuth2
  - Config: `internal/auth/codex/`, `sdk/auth/codex.go`

- Anthropic Claude - OAuth2 for Claude/Claude Code access
  - SDK/Client: golang.org/x/oauth2
  - Auth flow: PKCE OAuth2
  - Config: `internal/auth/claude/`, `sdk/auth/claude.go`

- Qwen Code - OAuth2 device flow for Qwen API access
  - SDK/Client: golang.org/x/oauth2
  - Auth flow: Device authorization grant
  - Config: `internal/auth/qwen/`, `sdk/auth/qwen.go`

- iFlow - OAuth2 for iFlow API access
  - SDK/Client: golang.org/x/oauth2
  - Auth flow: PKCE OAuth2
  - Config: `internal/auth/iflow/`, `sdk/auth/iflow.go`

- Antigravity - OAuth2 for Antigravity API access
  - SDK/Client: golang.org/x/oauth2
  - Config: `internal/auth/antigravity/`, `sdk/auth/antigravity.go`

**Vertex AI (Service Account):**
- Google Vertex AI - Service account key import for Vertex API access
  - SDK/Client: golang.org/x/oauth2
  - Config: `internal/auth/vertex/`
  - Env var: `vertex-import` flag

**Amp CLI Integration:**
- Ampcode.com - Amp CLI upstream proxy and management
  - Purpose: Proxy Amp CLI OAuth and management endpoints
  - Config: `ampcode.upstream-url`
  - Model mappings for unavailable models
  - Per-client upstream API key routing

## Data Storage

**Primary Token/Config Stores:**

**Local Filesystem (Default):**
- Location: `~/.cli-proxy-api/` or configured `auth-dir`
- Implementation: `sdk/auth/filestore.go`
- No external dependencies

**PostgreSQL (Optional):**
- Connection: env var `PGSTORE_DSN`
- Client: github.com/jackc/pgx/v5 (stdlib)
- Schema: configurable via `PGSTORE_SCHEMA`
- Tables: `config_store`, `auth_store`
- Local spool: mirrors to local filesystem
- Implementation: `internal/store/postgresstore.go`

**Git-Backed Config (Optional):**
- Connection: env var `GITSTORE_GIT_URL`
- Client: github.com/go-git/go-git/v6
- Auth: username + token (`GITSTORE_GIT_USERNAME`, `GITSTORE_GIT_TOKEN`)
- Local spool: mirrors to local filesystem
- Implementation: `internal/store/gitstore.go`

**S3-Compatible Object Storage (Optional):**
- Connection: env var `OBJECTSTORE_ENDPOINT`
- Client: github.com/minio/minio-go/v7
- Auth: access key + secret key (`OBJECTSTORE_ACCESS_KEY`, `OBJECTSTORE_SECRET_KEY`)
- Bucket: `OBJECTSTORE_BUCKET`
- Local spool: mirrors to local filesystem
- Implementation: `internal/store/objectstore.go`

**File Storage:**
- Local filesystem only for tokens, configuration, and logs
- No external file storage services

**Caching:**
- In-memory token caching (no external cache)
- In-memory usage statistics (optional, disabled by default)

## Authentication & Identity

**Auth Providers (OAuth2):**
- Google (Gemini CLI, Vertex AI)
- OpenAI (Codex)
- Anthropic (Claude)
- Alibaba (Qwen)
- iFlow
- Antigravity

**API Key Authentication:**
- Inline API keys in config.yaml
- OAuth token refresh via registered stores
- Management API: bcrypt hashed secret key

**No external identity providers detected** - All OAuth flows are direct to provider endpoints

## Monitoring & Observability

**Error Tracking:**
- None (log-based only)

**Logs:**
- logrus (github.com/sirupsen/logrus) - structured logging
- lumberjack - rotating log files
- Optional file-based logging with size limits

**Usage Statistics:**
- In-memory aggregation (optional, disabled by default)
- No external telemetry/analytics

## CI/CD & Deployment

**Hosting:**
- Container-ready (Dockerfile, docker-compose.yml)
- Base image: golang:1.24-alpine (builder), alpine:3.22.0 (runtime)
- GitHub Releases via goreleaser

**CI Pipeline:**
- GitHub Actions (`.github/` directory present)
- goreleaser for automated releases (`.goreleaser.yml`)

## Environment Configuration

**Required env vars (for optional features):**
- `PGSTORE_DSN` - PostgreSQL connection string (postgres store)
- `PGSTORE_SCHEMA` - Postgres schema name (postgres store)
- `PGSTORE_LOCAL_PATH` - Local spool path (postgres store)
- `GITSTORE_GIT_URL` - Git remote URL (git store)
- `GITSTORE_GIT_USERNAME` - Git username (git store)
- `GITSTORE_GIT_TOKEN` - Git access token (git store)
- `GITSTORE_LOCAL_PATH` - Local spool path (git store)
- `OBJECTSTORE_ENDPOINT` - S3 endpoint (object store)
- `OBJECTSTORE_BUCKET` - S3 bucket name (object store)
- `OBJECTSTORE_ACCESS_KEY` - S3 access key (object store)
- `OBJECTSTORE_SECRET_KEY` - S3 secret key (object store)
- `OBJECTSTORE_LOCAL_PATH` - Local spool path (object store)
- `MANAGEMENT_PASSWORD` - Management API password (optional)
- `DEPLOY` - Deployment mode ("cloud" for cloud deploy)

**Secrets location:**
- Environment variables for store backends
- YAML config file for API keys and OAuth tokens
- Bcrypt hashed management key in config

## Webhooks & Callbacks

**Incoming:**
- OAuth2 callback endpoints on ports:
  - 8085 (OpenAI Codex)
  - 1455 (Anthropic Claude)
  - 54545 (Qwen)
  - 51121 (iFlow)
  - 11451 (Antigravity)
- Configurable via `-oauth-callback-port` flag

**Outgoing:**
- OAuth2 authorization requests to provider endpoints
- Token refresh requests to provider endpoints
- No other webhooks detected

---

*Integration audit: 2026-01-29*