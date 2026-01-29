# Coding Conventions

**Analysis Date:** 2026-01-29

## Naming Patterns

**Files:**
- Source files: `snake_case.go` (e.g., `claude_openai_response.go`, `anthropic_auth.go`)
- Test files: `snake_case_test.go` co-located with source
- Package directories: lowercase, may use hyphens for compound names (e.g., `chat-completions`, `gemini-cli`)

**Functions:**
- Exported functions: PascalCase (e.g., `GenerateAuthURL`, `TranslateRequest`)
- Unexported functions: camelCase (e.g., `getAuthMiddleware`, `createReverseProxy`)

**Variables:**
- Exported variables: PascalCase
- Unexported variables: camelCase

**Types:**
- Structs, interfaces: PascalCase (e.g., `ClaudeAuth`, `ConvertAnthropicResponseToOpenAIParams`)
- Type aliases: PascalCase or descriptive names
- Function types: PascalCase (e.g., `TranslateRequestFunc`)

**Constants:**
- Exported constants: PascalCase
- Unexported/package-level constants: UPPER_SNAKE_CASE (e.g., `AuthURL`, `TokenURL`, `dataTag`)

## Code Style

**Formatting:**
- Tool: Standard `go fmt` (no custom formatting config)
- Go version: 1.24.0
- No .editorconfig, .prettierrc, or similar formatting configuration files

**Linting:**
- Tool: None configured (no golangci-lint, biome, etc.)
- CI workflow only validates build, not linting
- Project relies on Go's built-in checks (`go vet`, `go build`)

**Indentation:**
- Tabs for indentation
- Struct field comments use tab indentation

## Import Organization

**Order:**
1. Standard library
2. Third-party packages
3. Project packages (internal, sdk)

**Pattern:**
```go
import (
    "context"
    "fmt"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/sirupsen/logrus"

    "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
    "github.com/router-for-me/CLIProxyAPI/v6/sdk/auth"
)
```

**Path Aliases:**
- SDK imports use alias `sdk` prefix (e.g., `sdkaccess`, `sdkAuth`)
- Long paths may be aliased for readability (e.g., `ampmodule`)
- Logging uses alias `log` for logrus

## Error Handling

**Patterns:**
- Use `fmt.Errorf` with `%w` verb for error wrapping to preserve stack traces
- Use `errors.New` for simple, static error messages
- Return errors as last return value (pattern: `(result, error)`)
- Always check and handle errors, never ignore with `_`

**Example:**
```go
if err != nil {
    return nil, fmt.Errorf("failed to create token request: %w", err)
}
```

**Error types:**
- Custom error types defined in `errors.go` files (e.g., `internal/auth/claude/errors.go`)
- Error types include user-friendly message methods

## Logging

**Framework:** logrus (aliased as `log`)

**Patterns:**
- Structured logging with `log.WithField()` or `log.WithFields()`
- Log levels: `Debug()`, `Info()`, `Warn()`, `Error()`, `Fatal()`
- Request logging includes request ID for AI API endpoints
- Sensitive data is masked (see `internal/util/translator.go`)

**Log levels:**
- Debug: Detailed debugging information
- Info: General informational messages
- Warn: Warning conditions that don't stop execution
- Error: Error conditions
- Fatal: Fatal errors that cause program exit

**Example:**
```go
log.WithField("request_id", requestID).Info("request completed")
log.WithError(err).Errorf("amp secret source error: %v", err)
```

## Comments

**When to Comment:**
- Package declarations: Every package has a comment starting with `// Package <name> provides...`
- Exported functions and types: Documented with godoc comments
- Struct fields: Documented with inline comments
- Complex logic: Explained with inline comments
- Backward compatibility: Explicitly marked with "向后兼容" (backward compatibility) comments

**JSDoc/TSDoc:**
- Go uses godoc format
- Function comments include:
  - Brief description
  - Parameters section (if applicable)
  - Returns section (if applicable)

**Example:**
```go
// Package claude provides OAuth2 authentication functionality for Anthropic's Claude API.
// This package implements the complete OAuth2 flow with PKCE for secure authentication.
package claude

// GenerateAuthURL creates the OAuth authorization URL with PKCE.
//
// Parameters:
//   - state: A random state parameter for CSRF protection
//   - pkceCodes: The PKCE codes for secure code exchange
//
// Returns:
//   - string: The complete authorization URL
//   - string: The state parameter for verification
//   - error: An error if PKCE codes are missing or URL generation fails
```

**Struct field comments:**
```go
type Config struct {
    // Host is the network host/interface on which the API server will bind.
    Host string `yaml:"host" json:"-"`

    // Port is the network port on which the API server will listen.
    Port int `yaml:"port" json:"-"`
}
```

## Function Design

**Size:**
- No explicit size limit enforced
- Functions are generally kept focused and single-purpose
- Large functions are rare; when present, they are well-commented

**Parameters:**
- Context as first parameter when needed (e.g., `func (c *ClaudeAuth) ExchangeCodeForTokens(ctx context.Context, ...)`)
- Prefer explicit parameters over struct wrapping for public APIs
- Use Option pattern for complex construction (e.g., `amp.New(...)`)
- Use `*any` for optional/parameter passing in response translation

**Return Values:**
- Error as last return value when applicable
- Multiple return values common for `(result, error)` pattern
- Functions that cannot fail return single value

## Module Design

**Exports:**
- Exported symbols use PascalCase
- Unexported symbols use camelCase
- Public APIs are exported from `sdk/` directory
- Internal implementation in `internal/` directory

**Barrel Files:**
- `internal/translator/init.go` acts as barrel file for translator registration
- `internal/translator/translator/translator.go` provides wrapper functions
- No extensive barrel file usage; direct imports preferred

**Package Structure:**
- `internal/`: Private implementation
- `sdk/`: Public APIs
- `cmd/`: CLI commands
- `test/`: Top-level integration tests
- `examples/`: Example usage

**Option Pattern:**
Used for complex construction:
```go
type Option func(*AmpModule)

func WithSecretSource(source SecretSource) Option {
    return func(m *AmpModule) {
        m.secretSource = source
    }
}

func New(opts ...Option) *AmpModule {
    m := &AmpModule{}
    for _, opt := range opts {
        opt(m)
    }
    return m
}
```

## Context Usage

**Patterns:**
- Context as first parameter for functions that need cancellation/timeout
- Context propagated through HTTP handlers via `c.Request.Context()`
- Context used for request-scoped values (e.g., request ID)
- `context.WithTimeout()` for bounded operations (e.g., 30-second timeouts)

**Example:**
```go
func (s *OAuthServer) Stop(ctx context.Context) error {
    shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    return s.server.Shutdown(shutdownCtx)
}
```

## Concurrency

**Patterns:**
- Use `sync.RWMutex` for read-write locking
- Use `sync.Once` for one-time initialization
- Use `sync/atomic` for simple atomic operations
- Use `sync.Map` sparingly; prefer regular maps with mutex

**Example:**
```go
type AmpModule struct {
    proxyMu sync.RWMutex // protects proxy for hot-reload
    proxy   *httputil.ReverseProxy
}

func (m *AmpModule) getProxy() *httputil.ReverseProxy {
    m.proxyMu.RLock()
    defer m.proxyMu.RUnlock()
    return m.proxy
}
```

## Backward Compatibility

**Guidelines:**
- Backward compatibility code must be explicitly commented with "向后兼容"
- Compatibility logic only in entry layer
- Use explicit type checking with type assertions
- Maintain old and new code paths when needed

**Example:**
```go
// NewLegacy creates a new Amp routing module using the legacy constructor signature.
// This is provided for backwards compatibility.
//
// DEPRECATED: Use New with options instead.
func NewLegacy(accessManager *sdkaccess.Manager, authMiddleware gin.HandlerFunc) *AmpModule {
    return New(
        WithAccessManager(accessManager),
        WithAuthMiddleware(authMiddleware),
    )
}
```

---

*Convention analysis: 2026-01-29*