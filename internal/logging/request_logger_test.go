package logging

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteRequestInfoWithBody_OmitsSensitiveHeaders(t *testing.T) {
	var buf bytes.Buffer
	headers := map[string][]string{
		"authorization":       {"Bearer sk-THIS_SHOULD_NEVER_APPEAR"},
		"X-Management-Key":    {"phase11-dev-secret"},
		"Proxy-Authorization": {"Basic dXNlcjpwYXNz"},
		"Content-Type":        {"application/json"},
		"User-Agent":          {"CLIProxyAPI-test"},
	}

	err := writeRequestInfoWithBody(
		&buf,
		"/v1/chat/completions",
		"POST",
		headers,
		[]byte(`{"ok":true}`),
		"",
		time.Unix(0, 0).UTC(),
	)
	if err != nil {
		t.Fatalf("writeRequestInfoWithBody returned error: %v", err)
	}

	out := buf.String()

	// Keys and values for sensitive headers must not appear anywhere in disk logs.
	for _, forbidden := range []string{
		"Authorization:",
		"authorization:",
		"X-Management-Key:",
		"Proxy-Authorization:",
		"sk-THIS_SHOULD_NEVER_APPEAR",
		"phase11-dev-secret",
		"dXNlcjpwYXNz",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("expected output to omit %q, but it was present. output=%q", forbidden, out)
		}
	}

	// Non-sensitive headers still appear, proving the headers section is not disabled.
	for _, required := range []string{
		"=== HEADERS ===\n",
		"Content-Type: application/json\n",
		"User-Agent: CLIProxyAPI-test\n",
	} {
		if !strings.Contains(out, required) {
			t.Fatalf("expected output to contain %q, but it was missing. output=%q", required, out)
		}
	}
}
