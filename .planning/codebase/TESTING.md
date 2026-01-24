# Testing Patterns

**Analysis Date:** 2026-01-29

## Test Framework

**Runner:**
- Go standard `go test`
- Go version: 1.24.0
- Config: None (uses Go defaults)

**Assertion Library:**
- Go standard `testing` package
- No external assertion library (no testify, gomega, etc.)

**Run Commands:**
```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./internal/api/modules/amp

# Run tests with verbose output
go test -v ./...

# Run tests with race detector
go test -race ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test -run TestAmpModule_Name ./internal/api/modules/amp
```

**Note:** Per CLAUDE.md, when running pytest for this project:
- Use `source .venv/bin/activate` or `source venv/bin/activate` to enter virtual environment first
- Run with `pytest -ra --tb=line` (for Python tests if any)
- However, this project is primarily Go, so `go test` is the primary test runner

## Test File Organization

**Location:**
- Co-located with source files (package directory same as source)
- Test files in `test/` directory for integration/e2e tests

**Naming:**
- Source files: `snake_case.go`
- Test files: `snake_case_test.go`
- Example: `claude_openai_response.go` → `claude_openai_response_test.go`

**Structure:**
```
internal/
├── api/
│   ├── modules/
│   │   ├── amp/
│   │   │   ├── amp.go
│   │   │   ├── amp_test.go
│   │   │   ├── proxy.go
│   │   │   ├── proxy_test.go
│   │   │   └── ...
│   │   └── ...
│   └── ...
├── translator/
│   ├── claude/
│   │   └── openai/
│   │       ├── chat-completions/
│   │       │   ├── claude_openai_response.go
│   │       │   └── claude_openai_request.go
│   │       └── ...
│   └── ...
test/
├── amp_management_test.go
├── thinking_conversion_test.go
├── config_migration_test.go
└── ...
```

## Test Structure

**Suite Organization:**
```go
// Package-level init for test setup
func init() {
    gin.SetMode(gin.TestMode)
}

// Helper function marked with t.Helper()
func newAmpTestHandler(t *testing.T) (*management.Handler, string) {
    t.Helper()
    tmpDir := t.TempDir()
    // ... setup code
    return h, configPath
}

// Test function
func TestGetAmpCode(t *testing.T) {
    h, _ := newAmpTestHandler(t)
    r := setupAmpRouter(h)

    req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
    }
    // ... assertions
}
```

**Patterns:**
- **Setup:** Use `t.Helper()` for test setup functions
- **Teardown:** `defer` for cleanup, `t.TempDir()` for temporary files
- **Assertions:** Direct `if` checks with `t.Fatal()` or `t.Errorf()`

**Test setup patterns:**
```go
// Gin test mode setup in init()
func init() {
    gin.SetMode(gin.TestMode)
}

// Temporary directory
tmpDir := t.TempDir()

// Test server
upstream := httptest.NewServer(nil)
defer upstream.Close()
```

## Mocking

**Framework:** None (uses standard interfaces and test doubles)

**Patterns:**
- Use `httptest.NewServer()` for HTTP server mocking
- Use `httptest.NewRequest()` and `httptest.NewRecorder()` for HTTP handler testing
- Use nil checks for optional dependencies
- Create test doubles by implementing interfaces directly

**HTTP mocking example:**
```go
upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"result":"success"}`))
}))
defer upstream.Close()

req := httptest.NewRequest(http.MethodGet, "/test", nil)
w := httptest.NewRecorder()
r.ServeHTTP(w, req)
```

**What to Mock:**
- External HTTP servers (use `httptest.NewServer`)
- File system (use `t.TempDir()`)
- Configuration (create test config structs)

**What NOT to Mock:**
- Simple data transformations
- Business logic that can be tested directly
- Internal function calls (test through public interface)

## Fixtures and Factories

**Test Data:**
- Inline test data in table-driven tests
- Test helper functions create fixtures on demand

**Helper functions:**
```go
func newAmpTestHandler(t *testing.T) (*management.Handler, string) {
    t.Helper()
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.yaml")

    cfg := &config.Config{
        AmpCode: config.AmpCode{
            UpstreamURL: "https://example.com",
            UpstreamAPIKey: "test-api-key-12345",
        },
    }
    // ... create handler
    return h, configPath
}
```

**Location:**
- Helper functions co-located with tests
- Shared helpers in same package

## Coverage

**Requirements:** None enforced

**View Coverage:**
```bash
# Show coverage percentage
go test -cover ./...

# Show coverage in detail
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Show coverage for specific package
go test -cover ./internal/api/modules/amp
```

**Coverage files:**
- No pre-existing coverage configuration found
- Coverage is tracked but not enforced

## Test Types

**Unit Tests:**
- Scope: Single function or method
- Approach: Direct function calls with various inputs
- Location: Co-located with source (`*_test.go`)

**Integration Tests:**
- Scope: Multiple components working together
- Location: `test/` directory
- Examples: `test/amp_management_test.go`, `test/thinking_conversion_test.go`

**E2E Tests:**
- Framework: None (custom test setup)
- Tests in `test/` directory may cover end-to-end scenarios

## Common Patterns

**Table-Driven Tests:**
```go
func TestParseSuffix(t *testing.T) {
    tests := []struct {
        name      string
        model     string
        wantBase  string
        wantLevel string
    }{
        {"no suffix", "glm-4", "glm-4", ""},
        {"glm with suffix", "glm-4.1-flash(high)", "glm-4.1-flash", "high"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := thinking.ParseSuffix(tt.model)
            if result.ModelName != tt.wantBase {
                t.Errorf("ParseSuffix(%q).ModelName = %q, want %q", tt.model, result.ModelName, tt.wantBase)
            }
        })
    }
}
```

**Async Testing:**
```go
// Use context with timeout for async operations
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Wait for condition
select {
case <-done:
    // success
case <-time.After(time.Second):
    t.Fatal("timeout")
}
```

**Error Testing:**
```go
func TestInvalidInput(t *testing.T) {
    err := SomeFunction("invalid")
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "expected message") {
        t.Errorf("error message = %q, want contain %q", err.Error(), "expected message")
    }
}
```

**HTTP Handler Testing:**
```go
func TestHandler(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.GET("/test", handler)

    req := httptest.NewRequest(http.MethodGet, "/test", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
    }
}
```

## Test Helpers

**Common helpers:**
- `t.Helper()`: Mark helper functions
- `t.TempDir()`: Create temporary directories
- `t.Cleanup(func())`: Register cleanup functions
- `t.Parallel()`: Mark parallel tests
- `t.Run(name, func())`: Subtests

**Gin-specific:**
- `gin.SetMode(gin.TestMode)`: Disable Gin logging in tests
- `httptest.NewRequest`: Create HTTP requests
- `httptest.NewRecorder`: Record HTTP responses

**Example:**
```go
func setupTest(t *testing.T) (*gin.Engine, func()) {
    t.Helper()
    gin.SetMode(gin.TestMode)
    r := gin.New()
    // ... setup routes
    return r, func() {
        // cleanup
    }
}
```

## Test Naming Conventions

**Test function names:**
- `Test<FunctionName>` for testing a specific function
- `Test<Feature>` for testing a feature/behavior
- Use descriptive names that explain what is being tested

**Examples:**
- `TestAmpModule_Name`: Tests the Name() method
- `TestAmpModule_Register_WithUpstream`: Tests registration with upstream
- `TestAmpModule_Register_WithoutUpstream`: Tests registration without upstream
- `TestGetAmpCode`: Tests GET /ampcode endpoint

**Subtest names:**
- Use `t.Run()` with descriptive names
- Table-driven tests use struct field `name` for subtest names

## CI/CD Testing

**GitHub Actions:**
- Workflow: `.github/workflows/pr-test-build.yml`
- Runs on pull requests
- Executes `go build` to verify compilation
- Does not explicitly run tests in CI

**Note:** The CI workflow only builds, not tests. Tests should be run locally before pushing.

---

*Testing analysis: 2026-01-29*