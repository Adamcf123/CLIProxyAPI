package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

// editOperation represents a single edit operation from the edit tool.
type editOperation struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// editObject represents a single edit JSON object.
type editObject struct {
	File       string          `json:"file"`
	Text       string          `json:"text"`
	Operations []editOperation `json:"operations"`
}

// ToolCallNormalizer normalizes tool payloads for specific model and route combinations.
// Handles gemini edit-to-apply_patch conversion.
type ToolCallNormalizer struct{}

// NormalizeResult represents the result of normalization.
type NormalizeResult struct {
	ToolName string
	Args     []byte
	Err      error
}

// NewToolCallNormalizer creates a new ToolCallNormalizer instance.
func NewToolCallNormalizer() *ToolCallNormalizer {
	return &ToolCallNormalizer{}
}

// NormalizeToolCall processes a tool call and returns normalized arguments.
// For "edit" tool with concatenated JSON objects, converts to "apply_patch".
// For a single object payload, preserves as-is.
// Returns error if a concatenated payload cannot be parsed safely.
func (n *ToolCallNormalizer) NormalizeToolCall(toolName string, args []byte) *NormalizeResult {
	if toolName != "edit" {
		return &NormalizeResult{ToolName: toolName, Args: args}
	}
	trimmed := bytes.TrimSpace(args)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return &NormalizeResult{ToolName: toolName, Args: args}
	}

	objectsRaw, err := splitTopLevelJSONObjectSequence(trimmed)
	if err != nil {
		return &NormalizeResult{
			ToolName: toolName,
			Err:      fmt.Errorf("invalid concatenated edit payload: %w", err),
		}
	}
	if len(objectsRaw) <= 1 {
		return &NormalizeResult{ToolName: toolName, Args: args}
	}

	objects := make([]editObject, 0, len(objectsRaw))
	for i := range objectsRaw {
		var obj editObject
		if err := json.Unmarshal(objectsRaw[i], &obj); err != nil {
			return &NormalizeResult{
				ToolName: toolName,
				Err:      fmt.Errorf("invalid edit object at index %d: %w", i, err),
			}
		}
		normalizedPath, err := sanitizePatchPath(obj.File)
		if err != nil {
			return &NormalizeResult{
				ToolName: toolName,
				Err:      fmt.Errorf("invalid edit file path at index %d: %w", i, err),
			}
		}
		obj.File = normalizedPath
		if len(obj.Operations) == 0 {
			return &NormalizeResult{
				ToolName: toolName,
				Err:      fmt.Errorf("ambiguous edit payload at index %d: missing operations", i),
			}
		}
		for j := range obj.Operations {
			opType := strings.TrimSpace(obj.Operations[j].Type)
			if opType != "" && opType != "replace" {
				return &NormalizeResult{
					ToolName: toolName,
					Err:      fmt.Errorf("unsupported edit operation type %q at index %d", opType, i),
				}
			}
		}
		objects = append(objects, obj)
	}

	newName, newArgs, err := n.convertToApplyPatch(objects)
	if err != nil {
		return &NormalizeResult{ToolName: toolName, Err: err}
	}
	return &NormalizeResult{ToolName: newName, Args: newArgs}
}

func splitTopLevelJSONObjectSequence(data []byte) ([][]byte, error) {
	segments := make([][]byte, 0, 4)
	i := 0
	for i < len(data) {
		for i < len(data) && isJSONWhitespace(data[i]) {
			i++
		}
		if i >= len(data) {
			break
		}
		if data[i] != '{' {
			return nil, fmt.Errorf("expected object start at byte %d", i)
		}
		end, err := scanJSONObjectAt(data, i)
		if err != nil {
			return nil, err
		}
		segments = append(segments, data[i:end])
		i = end
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("empty object sequence")
	}
	return segments, nil
}

// convertToApplyPatch converts parsed edit objects to apply_patch format.
func (n *ToolCallNormalizer) convertToApplyPatch(objects []editObject) (string, []byte, error) {
	// Group by file path
	fileEdits := make(map[string][]editObject)
	for _, obj := range objects {
		fileEdits[obj.File] = append(fileEdits[obj.File], obj)
	}

	// Sort file paths for deterministic order
	sortedFiles := make([]string, 0, len(fileEdits))
	for path := range fileEdits {
		sortedFiles = append(sortedFiles, path)
	}
	sort.Strings(sortedFiles)

	// Build unified diff for each file
	var patchBuilder strings.Builder

	for _, filePath := range sortedFiles {
		edits := fileEdits[filePath]

		for _, edit := range edits {
			oldContent := edit.Text
			for _, op := range edit.Operations {
				diff := generateUnifiedDiff(filePath, oldContent, op.Text)
				if diff != "" {
					patchBuilder.WriteString(diff)
				}
				oldContent = op.Text
			}
		}
	}
	if patchBuilder.Len() == 0 {
		return "", nil, fmt.Errorf("normalization failed: no patch operations generated")
	}

	// Create apply_patch payload
	result := map[string]string{
		"patchText": patchBuilder.String(),
		"path":      sortedFiles[0],
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return "", nil, fmt.Errorf("normalization failed: %w", err)
	}

	return "apply_patch", payload, nil
}

func sanitizePatchPath(pathValue string) (string, error) {
	p := strings.TrimSpace(pathValue)
	if p == "" {
		return "", fmt.Errorf("missing file path")
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("file path contains control characters")
		}
	}
	normalized := strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(normalized, "//") {
		return "", fmt.Errorf("network path is not allowed")
	}
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("absolute path is not allowed")
	}
	if len(normalized) >= 2 && normalized[1] == ':' {
		return "", fmt.Errorf("windows drive path is not allowed")
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	return cleaned, nil
}

// generateUnifiedDiff generates a unified diff for one replacement operation.
func generateUnifiedDiff(filePath, oldContent, newContent string) string {
	if oldContent == newContent {
		return ""
	}

	var diffBuilder strings.Builder
	diffBuilder.WriteString(fmt.Sprintf("--- a/%s\n", filePath))
	diffBuilder.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))

	oldLines, oldCount := splitContentLines(oldContent)
	newLines, newCount := splitContentLines(newContent)
	oldStart, newStart := 1, 1
	if oldCount == 0 {
		oldStart = 0
	}
	if newCount == 0 {
		newStart = 0
	}
	diffBuilder.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
	for _, line := range oldLines {
		diffBuilder.WriteString("-" + line + "\n")
	}
	for _, line := range newLines {
		diffBuilder.WriteString("+" + line + "\n")
	}

	return diffBuilder.String()
}

func splitContentLines(content string) ([]string, int) {
	if content == "" {
		return nil, 0
	}
	parts := strings.Split(content, "\n")
	return parts, len(parts)
}

func scanJSONObjectAt(data []byte, start int) (int, error) {
	if start < 0 || start >= len(data) || data[start] != '{' {
		return 0, fmt.Errorf("expected object start at byte %d", start)
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(data); i++ {
		c := data[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
			if depth < 0 {
				return 0, fmt.Errorf("unexpected object close at byte %d", i)
			}
		}
	}
	return 0, fmt.Errorf("unterminated JSON object")
}

func isJSONWhitespace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t'
}
