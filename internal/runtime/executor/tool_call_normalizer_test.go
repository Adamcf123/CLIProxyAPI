package executor

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// TestToolCallNormalizer_Scenario1_ConvertConcatenatedEdits tests that concatenated
// edit JSON objects are converted to a single valid apply_patch payload.
func TestToolCallNormalizer_Scenario1_ConvertConcatenatedEdits(t *testing.T) {
	normalizer := NewToolCallNormalizer()

	// Simulate concatenated edit objects from gemini-3-flash-preview
	// These are multiple edit operations concatenated together
	concatenatedInput := `{"file":"file1.txt","text":"content1","operations":[{"type":"replace","text":"replacement1"}]}` +
		`{"file":"file2.txt","text":"content2","operations":[{"type":"replace","text":"replacement2"}]}`

	toolName := "edit"
	args := []byte(concatenatedInput)

	result := normalizer.NormalizeToolCall(toolName, args)

	// Should not return error
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should convert to apply_patch
	if result.ToolName != "apply_patch" {
		t.Fatalf("tool name = %q, want %q", result.ToolName, "apply_patch")
	}

	// The args should be a valid JSON object with patchText containing all edits
	patchText := gjson.GetBytes(result.Args, "patchText").String()
	if patchText == "" {
		t.Fatalf("expected patchText in normalized args, got: %s", result.Args)
	}

	// Verify all file edits are included
	if !strings.Contains(patchText, "file1.txt") {
		t.Fatalf("patchText missing file1.txt, got: %s", patchText)
	}
	if !strings.Contains(patchText, "file2.txt") {
		t.Fatalf("patchText missing file2.txt, got: %s", patchText)
	}
}

// TestToolCallNormalizer_Scenario2_PreserveSingleObject tests that valid single-object
// edit payloads are forwarded without conversion.
func TestToolCallNormalizer_Scenario2_PreserveSingleObject(t *testing.T) {
	normalizer := NewToolCallNormalizer()

	// Single valid edit object
	singleInput := `{"file":"main.go","text":"old content","operations":[{"type":"replace","text":"new content"}]}`

	toolName := "edit"
	args := []byte(singleInput)

	result := normalizer.NormalizeToolCall(toolName, args)

	// Should not return error
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should remain as edit
	if result.ToolName != "edit" {
		t.Fatalf("tool name = %q, want %q (should preserve)", result.ToolName, "edit")
	}

	// Args should be unchanged
	if string(result.Args) != singleInput {
		t.Fatalf("args changed unexpectedly: got %s, want %s", result.Args, singleInput)
	}
}

// TestToolCallNormalizer_Scenario4_MultipleFilesAndHunks tests that the normalizer
// handles more than three files and hunks with deterministic ordering.
func TestToolCallNormalizer_Scenario4_MultipleFilesAndHunks(t *testing.T) {
	normalizer := NewToolCallNormalizer()

	// Create concatenated input with 4+ files and multiple hunks per file
	concatenatedInput := `{"file":"a.txt","text":"a","operations":[{"type":"replace","text":"A"}]}` +
		`{"file":"b.txt","text":"b","operations":[{"type":"replace","text":"B"}]}` +
		`{"file":"c.txt","text":"c","operations":[{"type":"replace","text":"C"}]}` +
		`{"file":"d.txt","text":"d","operations":[{"type":"replace","text":"D"}]}`

	toolName := "edit"
	args := []byte(concatenatedInput)

	result := normalizer.NormalizeToolCall(toolName, args)

	// Should not return error
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Should convert to apply_patch
	if result.ToolName != "apply_patch" {
		t.Fatalf("tool name = %q, want %q", result.ToolName, "apply_patch")
	}

	// Verify all 4 files are present
	patchText := gjson.GetBytes(result.Args, "patchText").String()
	files := []string{"a.txt", "b.txt", "c.txt", "d.txt"}
	for _, f := range files {
		if !strings.Contains(patchText, f) {
			t.Fatalf("patchText missing %s, got: %s", f, patchText)
		}
	}

	// Verify deterministic order - a.txt should appear before b.txt, etc.
	idxA := strings.Index(patchText, "a.txt")
	idxB := strings.Index(patchText, "b.txt")
	idxC := strings.Index(patchText, "c.txt")
	idxD := strings.Index(patchText, "d.txt")

	if idxA >= idxB || idxB >= idxC || idxC >= idxD {
		t.Fatalf("patchText not in deterministic order: got positions a=%d, b=%d, c=%d, d=%d",
			idxA, idxB, idxC, idxD)
	}
}

// TestToolCallNormalizer_Scenario4_MultipleHunksPerFile tests that multiple hunks
// within the same file are handled correctly.
func TestToolCallNormalizer_Scenario4_MultipleHunksPerFile(t *testing.T) {
	normalizer := NewToolCallNormalizer()

	// One edit object can carry multiple operations (hunks), and all should be preserved.
	concatenatedInput := `{"file":"main.go","text":"line1","operations":[{"type":"replace","text":"new1"},{"type":"replace","text":"new2"}]}` +
		`{"file":"main.go","text":"line3","operations":[{"type":"replace","text":"new3"}]}`

	toolName := "edit"
	args := []byte(concatenatedInput)

	result := normalizer.NormalizeToolCall(toolName, args)

	// Should not return error
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if result.ToolName != "apply_patch" {
		t.Fatalf("tool name = %q, want %q", result.ToolName, "apply_patch")
	}

	patchText := gjson.GetBytes(result.Args, "patchText").String()

	// All hunks should be in the patchText
	hunks := []string{"new1", "new2", "new3"}
	for _, h := range hunks {
		if !strings.Contains(patchText, h) {
			t.Fatalf("patchText missing hunk %s, got: %s", h, patchText)
		}
	}
}

// TestToolCallNormalizer_InvalidJSON tests safe failure for malformed concatenated payloads.
func TestToolCallNormalizer_InvalidJSON(t *testing.T) {
	normalizer := NewToolCallNormalizer()

	// Invalid concatenated JSON - second object is malformed.
	invalidInput := `{"file":"a.txt"}{invalid json}`

	toolName := "edit"
	args := []byte(invalidInput)

	result := normalizer.NormalizeToolCall(toolName, args)
	if result.Err == nil {
		t.Fatalf("expected normalization error for malformed concatenated payload")
	}
}

func TestToolCallNormalizer_SingleObjectWithBracesInStringIsPreserved(t *testing.T) {
	normalizer := NewToolCallNormalizer()
	singleInput := `{"file":"main.go","text":"func x() { return y; }","operations":[{"type":"replace","text":"func x() { return z; }"}]}`

	result := normalizer.NormalizeToolCall("edit", []byte(singleInput))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.ToolName != "edit" {
		t.Fatalf("tool name = %q, want %q", result.ToolName, "edit")
	}
	if string(result.Args) != singleInput {
		t.Fatalf("single payload should stay unchanged")
	}
}

func TestToolCallNormalizer_RejectsUnsafeFilePath(t *testing.T) {
	normalizer := NewToolCallNormalizer()
	cases := []string{
		`../secret.txt`,
		`..\\secret.txt`,
		`C:\\secret.txt`,
		`\\\\server\\share\\a.txt`,
		"bad\x00path.txt",
	}
	for _, unsafePath := range cases {
		input := `{"file":"` + unsafePath + `","text":"old","operations":[{"type":"replace","text":"new"}]}` +
			`{"file":"ok.txt","text":"a","operations":[{"type":"replace","text":"b"}]}`
		result := normalizer.NormalizeToolCall("edit", []byte(input))
		if result.Err == nil {
			t.Fatalf("expected path validation error for %q", unsafePath)
		}
	}
}

// TestToolCallNormalizer_NonEditTool tests that non-edit tools are passed through unchanged.
func TestToolCallNormalizer_NonEditTool(t *testing.T) {
	normalizer := NewToolCallNormalizer()

	args := []byte(`{"key":"value"}`)

	result := normalizer.NormalizeToolCall("other_tool", args)

	// Should not return error
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	if result.ToolName != "other_tool" {
		t.Fatalf("tool name changed: got %q, want %q", result.ToolName, "other_tool")
	}
	if string(result.Args) != `{"key":"value"}` {
		t.Fatalf("args changed: got %s, want %s", result.Args, args)
	}
}
