package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestParseOpenAIUsageChatCompletions(t *testing.T) {
	data := []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":5}}}`)
	detail := parseOpenAIUsage(data)
	if detail.InputTokens != 1 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 1)
	}
	if detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 2)
	}
	if detail.TotalTokens != 3 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 3)
	}
	if detail.CachedTokens != 4 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 4)
	}
	if detail.ReasoningTokens != 5 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 5)
	}
}

func TestParseOpenAIUsageResponses(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"cached_tokens":7},"output_tokens_details":{"reasoning_tokens":9}}}`)
	detail := parseOpenAIUsage(data)
	if detail.InputTokens != 10 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 10)
	}
	if detail.OutputTokens != 20 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 20)
	}
	if detail.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 30)
	}
	if detail.CachedTokens != 7 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 7)
	}
	if detail.ReasoningTokens != 9 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 9)
	}
}

func TestParseClaudeUsage_EphemeralCacheTokens(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":100,"output_tokens":50,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":50},"cache_read_input_tokens":0}}`)
	detail := parseClaudeUsage(data)
	if detail.InputTokens != 100 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 100)
	}
	if detail.OutputTokens != 50 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 50)
	}
	if detail.CachedTokens != 150 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 150)
	}
}

func TestParseClaudeUsage_ThinkingTokens(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":100,"output_tokens":30,"thinking_tokens":200}}`)
	detail := parseClaudeUsage(data)
	if detail.ReasoningTokens != 200 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 200)
	}
	if detail.TotalTokens != 330 {
		t.Fatalf("total tokens = %d, want %d (input + output + reasoning)", detail.TotalTokens, 330)
	}
}

func TestParseClaudeStreamUsage_EphemeralCacheTokens(t *testing.T) {
	line := []byte(`data: {"type":"message_delta","usage":{"output_tokens":50,"cache_creation":{"ephemeral_5m_input_tokens":80}}}`)
	detail, ok := parseClaudeStreamUsage(line)
	if !ok {
		t.Fatal("parseClaudeStreamUsage returned false, want true")
	}
	if detail.CachedTokens != 80 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 80)
	}
}

func TestUsageReporter_Publish_SkipsAllZeroTokensWhenNotFailed(t *testing.T) {
	ctx := context.Background()
	reporter := newUsageReporter(ctx, "provider", "model", nil)

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()

	calls := 0
	publishUsageRecord = func(ctx context.Context, record usage.Record) {
		calls++
	}

	reporter.publish(ctx, usage.Detail{})
	if calls != 0 {
		t.Fatalf("expected no publish calls, got %d", calls)
	}
}

func TestUsageReporter_EnsurePublished_EmitsOnce(t *testing.T) {
	ctx := context.Background()
	reporter := newUsageReporter(ctx, "provider", "model", nil)

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()

	calls := 0
	var got usage.Record
	publishUsageRecord = func(ctx context.Context, record usage.Record) {
		calls++
		got = record
	}

	reporter.ensurePublished(ctx)
	reporter.ensurePublished(ctx)
	reporter.ensurePublished(ctx)

	if calls != 1 {
		t.Fatalf("expected 1 publish call, got %d", calls)
	}
	if got.Failed {
		t.Fatalf("expected Failed=false, got true")
	}
	if got.Detail != (usage.Detail{}) {
		t.Fatalf("expected empty Detail, got %#v", got.Detail)
	}
}

func TestUsageReporter_PublishThenFinalize_EmitsOnlyOnce(t *testing.T) {
	ctx := context.Background()
	reporter := newUsageReporter(ctx, "provider", "model", nil)

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()

	calls := 0
	publishUsageRecord = func(ctx context.Context, record usage.Record) {
		calls++
	}

	reporter.publish(ctx, usage.Detail{OutputTokens: 1, TotalTokens: 1})
	var err error
	reporter.finalize(ctx, &err)

	if calls != 1 {
		t.Fatalf("expected 1 publish call, got %d", calls)
	}
}

func TestUsageReporter_Finalize_PublishesFailureWhenErrSet(t *testing.T) {
	ctx := context.Background()
	reporter := newUsageReporter(ctx, "provider", "model", nil)

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()

	calls := 0
	var got usage.Record
	publishUsageRecord = func(ctx context.Context, record usage.Record) {
		calls++
		got = record
	}

	err := errors.New("boom")
	reporter.finalize(ctx, &err)

	if calls != 1 {
		t.Fatalf("expected 1 publish call, got %d", calls)
	}
	if !got.Failed {
		t.Fatalf("expected Failed=true, got false")
	}
}

func TestUsageReporter_PublishFailureThenEnsurePublished_EmitsFailureOnlyOnce(t *testing.T) {
	ctx := context.Background()
	reporter := newUsageReporter(ctx, "provider", "model", nil)

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()

	calls := 0
	var got usage.Record
	publishUsageRecord = func(ctx context.Context, record usage.Record) {
		calls++
		got = record
	}

	reporter.publishFailure(ctx)
	reporter.ensurePublished(ctx)

	if calls != 1 {
		t.Fatalf("expected 1 publish call, got %d", calls)
	}
	if !got.Failed {
		t.Fatalf("expected Failed=true, got false")
	}
}

func TestUsageReporter_PublishFailureThenFinalize_EmitsFailureOnlyOnce(t *testing.T) {
	ctx := context.Background()
	reporter := newUsageReporter(ctx, "provider", "model", nil)

	prev := publishUsageRecord
	defer func() { publishUsageRecord = prev }()

	calls := 0
	var got usage.Record
	publishUsageRecord = func(ctx context.Context, record usage.Record) {
		calls++
		got = record
	}

	reporter.publishFailure(ctx)
	var err error
	reporter.finalize(ctx, &err)

	if calls != 1 {
		t.Fatalf("expected 1 publish call, got %d", calls)
	}
	if !got.Failed {
		t.Fatalf("expected Failed=true, got false")
	}
}
