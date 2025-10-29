package executor

import (
	"context"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

// MiniMaxExecutor is a thin wrapper that delegates to ClaudeExecutor
// for Anthropic-compatible MiniMax endpoints while exposing provider identifier
// as "minimax" to the core manager and routing layer.
type MiniMaxExecutor struct{ cfg *config.Config }

func NewMiniMaxExecutor(cfg *config.Config) *MiniMaxExecutor { return &MiniMaxExecutor{cfg: cfg} }

func (e *MiniMaxExecutor) Identifier() string { return "minimax" }

// resolveClaudeMiniMaxAuth returns a synthesized auth when configuration indicates
// MiniMax via Claude's Anthropic-compatible endpoint. Priority:
// 1) If incoming auth has a base_url that looks Anthropic-compatible and carries api_key, use it.
// 2) Else, scan cfg.ClaudeKey entries and build a fallback auth when possible.
//   - Prefer entries whose base-url points to MiniMax Anthropic endpoint (case-insensitive contains
//     "api.minimaxi.com" or "/api/anthropic").
//   - As a relaxed fallback, if exactly one Claude key is configured with both api-key and base-url,
//     use it.
func (e *MiniMaxExecutor) resolveClaudeMiniMaxAuth(in *cliproxyauth.Auth) *cliproxyauth.Auth {
	if e == nil || e.cfg == nil {
		return nil
	}
	// 1) Incoming auth hints
	if in != nil && in.Attributes != nil {
		base := strings.TrimSpace(in.Attributes["base_url"])
		key := strings.TrimSpace(in.Attributes["api_key"])
		if base != "" && key != "" {
			lower := strings.ToLower(base)
			if strings.Contains(lower, "/api/anthropic") || strings.Contains(lower, "api.minimaxi.com") {
				return in
			}
		}
	}
	// 2) Config claude-api-key entries
	var firstCandidate *cliproxyauth.Auth
	for i := range e.cfg.ClaudeKey {
		ck := e.cfg.ClaudeKey[i]
		base := strings.TrimSpace(ck.BaseURL)
		key := strings.TrimSpace(ck.APIKey)
		if key == "" || base == "" {
			continue
		}
		a := &cliproxyauth.Auth{
			ID:       "minimax-via-claude",
			Provider: "claude",
			Label:    "minimax-via-claude",
			Status:   cliproxyauth.StatusActive,
			ProxyURL: strings.TrimSpace(ck.ProxyURL),
			Attributes: map[string]string{
				"api_key":  key,
				"base_url": base,
			},
		}
		if firstCandidate == nil {
			firstCandidate = a
		}
		lower := strings.ToLower(base)
		if strings.EqualFold(base, "https://api.minimaxi.com/anthropic") ||
			strings.Contains(lower, "api.minimaxi.com") ||
			strings.Contains(lower, "/api/anthropic") {
			return a
		}
	}
	if firstCandidate != nil && len(e.cfg.ClaudeKey) == 1 {
		return firstCandidate
	}
	return nil
}

func (e *MiniMaxExecutor) PrepareRequest(r *http.Request, a *cliproxyauth.Auth) error {
	// Delegate to ClaudeExecutor (no-op)
	return nil
}

func (e *MiniMaxExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	if fallback := e.resolveClaudeMiniMaxAuth(auth); fallback != nil {
		return NewAnthropicCompatExecutor(e.cfg, e.Identifier()).Execute(ctx, fallback, req, opts)
	}
	return NewAnthropicCompatExecutor(e.cfg, e.Identifier()).Execute(ctx, auth, req, opts)
}

func (e *MiniMaxExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (<-chan cliproxyexecutor.StreamChunk, error) {
	if fallback := e.resolveClaudeMiniMaxAuth(auth); fallback != nil {
		return NewAnthropicCompatExecutor(e.cfg, e.Identifier()).ExecuteStream(ctx, fallback, req, opts)
	}
	return NewAnthropicCompatExecutor(e.cfg, e.Identifier()).ExecuteStream(ctx, auth, req, opts)
}

func (e *MiniMaxExecutor) CountTokens(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	if fallback := e.resolveClaudeMiniMaxAuth(auth); fallback != nil {
		return NewAnthropicCompatExecutor(e.cfg, e.Identifier()).CountTokens(ctx, fallback, req, opts)
	}
	return NewAnthropicCompatExecutor(e.cfg, e.Identifier()).CountTokens(ctx, auth, req, opts)
}

func (e *MiniMaxExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	return NewAnthropicCompatExecutor(e.cfg, e.Identifier()).Refresh(ctx, auth)
}
