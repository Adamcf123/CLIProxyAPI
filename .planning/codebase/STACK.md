# Technology Stack

**Analysis Date:** 2026-01-29

## Languages

**Primary:**
- Go 1.24.0 - Core server and all backend services

**Secondary:**
- None detected

## Runtime

**Environment:**
- Go 1.24.0

**Package Manager:**
- Go modules (go.mod, go.sum)
- Lockfile: present (go.sum)

## Frameworks

**Core:**
- github.com/gin-gonic/gin v1.10.1 - HTTP web framework for API server
- github.com/gorilla/websocket v1.5.3 - WebSocket support for streaming

**Testing:**
- Go testing package (standard library)
- No external test framework detected

**Build/Dev:**
- github.com/joho/godotenv v1.5.1 - Environment variable loading from .env files
- fsnotify v1.9.0 - File system watching for config reload

## Key Dependencies

**Critical:**
- github.com/jackc/pgx/v5 v5.7.6 - PostgreSQL driver for optional postgres-backed token store
- github.com/go-git/go-git/v6 v6.0.0 - Git client for optional git-backed config store
- github.com/minio/minio-go/v7 v7.0.66 - S3-compatible object storage client for optional token store
- golang.org/x/oauth2 v0.30.0 - OAuth2 client for authentication flows (Claude, Codex, Gemini, Qwen, iFlow, Antigravity)
- github.com/tidwall/gjson v1.18.0 - JSON parsing and modification

**Infrastructure:**
- github.com/sirupsen/logrus v1.9.3 - Structured logging
- gopkg.in/natefinch/lumberjack.v2 v2.2.1 - Log file rotation
- gopkg.in/yaml.v3 v3.0.1 - YAML configuration parsing
- github.com/google/uuid v1.6.0 - UUID generation
- github.com/tiktoken-go/tokenizer v0.7.0 - Token counting for OpenAI models
- golang.org/x/crypto v0.45.0 - Cryptographic operations (bcrypt for password hashing)

**Compression:**
- github.com/klauspost/compress v1.17.4 - Compression utilities
- github.com/andybalholm/brotli v1.0.6 - Brotli compression

**Transport:**
- golang.org/x/net v0.47.0 - Network utilities including HTTP/2 support

## Configuration

**Environment:**
- YAML-based configuration (config.yaml)
- Optional .env file for environment variables (godotenv)
- Key env vars: PGSTORE_DSN, GITSTORE_GIT_URL, OBJECTSTORE_ENDPOINT, DEPLOY

**Build:**
- Dockerfile for container builds
- docker-compose.yml for local development
- .goreleaser.yml for release automation
- Build flags: VERSION, COMMIT, BUILD_DATE

## Platform Requirements

**Development:**
- Go 1.24.0 or higher
- Linux/macOS/Windows (WSL) support
- Optional: PostgreSQL for postgres-backed store
- Optional: Git for git-backed config store
- Optional: S3-compatible storage for object store

**Production:**
- Linux container (Alpine 3.22.0)
- Port 8317 (default), plus OAuth callback ports (8085, 1455, 54545, 51121, 11451)
- Volume mounts for config, auth files, and logs

---

*Stack analysis: 2026-01-29*