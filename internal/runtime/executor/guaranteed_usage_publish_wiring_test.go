package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

func TestGuaranteedUsagePublishWiring(t *testing.T) {
	moduleRoot := findModuleRoot(t)

	// Keep this list explicit to make scope and failures obvious.
	files := []string{
		"internal/runtime/executor/claude_executor.go",
		"internal/runtime/executor/codex_executor.go",
		"internal/runtime/executor/qwen_executor.go",
		"internal/runtime/executor/gemini_executor.go",
		"internal/runtime/executor/gemini_cli_executor.go",
		"internal/runtime/executor/gemini_vertex_executor.go",
		"internal/runtime/executor/aistudio_executor.go",
	}

	streamHookLiteral := "defer reporter.ensurePublished(ctx)"
	nonStreamHookLiteral := "defer reporter.finalize(ctx, &err)"

	streamHookPattern := regexp.MustCompile(`defer\s+reporter\.ensurePublished\(\s*\w+\s*\)`)
	nonStreamHookPattern := regexp.MustCompile(`defer\s+reporter\.finalize\(\s*\w+\s*,\s*&err\s*\)`)

	for _, relPath := range files {
		absPath := filepath.Join(moduleRoot, filepath.FromSlash(relPath))
		b, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("%s: failed to read source file: %v", relPath, err)
		}

		if !streamHookPattern.Match(b) {
			t.Fatalf("%s: missing required streaming hook: %s (pattern: %s)", relPath, streamHookLiteral, streamHookPattern.String())
		}
		if !nonStreamHookPattern.Match(b) {
			t.Fatalf("%s: missing required non-streaming hook: %s (pattern: %s)", relPath, nonStreamHookLiteral, nonStreamHookPattern.String())
		}
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok || thisFile == "" {
		t.Fatal("runtime.Caller(0) failed to resolve test file path")
	}

	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		modPath := filepath.Join(dir, "go.mod")
		if info, err := os.Stat(modPath); err == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("could not locate go.mod within 10 parent directories from %s", thisFile)
	return ""
}
