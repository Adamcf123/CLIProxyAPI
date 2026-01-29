package logging

import (
	"regexp"
	"sync"
	"testing"
)

func TestGenerateRequestID_Length(t *testing.T) {
	id := GenerateRequestID()
	if len(id) != 16 {
		t.Fatalf("expected length 16, got %d (%q)", len(id), id)
	}
}

func TestGenerateRequestID_HexFormat(t *testing.T) {
	id := GenerateRequestID()
	if len(id) != 16 {
		t.Fatalf("expected length 16, got %d (%q)", len(id), id)
	}

	// hex.EncodeToString uses lowercase; keep format strict for easier log/DB searches.
	re := regexp.MustCompile("^[0-9a-f]{16}$")
	if !re.MatchString(id) {
		t.Fatalf("expected 16-char lowercase hex, got %q", id)
	}
}

func TestGenerateRequestID_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := GenerateRequestID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestGenerateRequestID_Concurrent(t *testing.T) {
	const workers = 100
	const perWorker = 100

	var wg sync.WaitGroup
	ids := make(chan string, workers*perWorker)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				ids <- GenerateRequestID()
			}
		}()
	}

	wg.Wait()
	close(ids)

	re := regexp.MustCompile("^[0-9a-f]{16}$")
	seen := make(map[string]struct{}, workers*perWorker)
	for id := range ids {
		if !re.MatchString(id) {
			t.Fatalf("invalid request ID format: %q", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate in concurrent test: %q", id)
		}
		seen[id] = struct{}{}
	}
}
