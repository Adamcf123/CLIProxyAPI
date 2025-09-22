package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	qwenauth "github.com/router-for-me/CLIProxyAPI/v6/internal/auth/qwen"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	qwenUserAgent           = "google-api-nodejs-client/9.15.1"
	qwenXGoogAPIClient      = "gl-node/22.17.0"
	qwenClientMetadataValue = "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI"
)

// QwenExecutor is a stateless executor for Qwen Code using OpenAI-compatible chat completions.
// If access token is unavailable, it falls back to legacy via ClientAdapter.
type QwenExecutor struct {
	cfg *config.Config
}

func NewQwenExecutor(cfg *config.Config) *QwenExecutor { return &QwenExecutor{cfg: cfg} }

func (e *QwenExecutor) Identifier() string { return "qwen" }

func (e *QwenExecutor) PrepareRequest(_ *http.Request, _ *cliproxyauth.Auth) error { return nil }

func (e *QwenExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	token, baseURL := qwenCreds(auth)
	if token == "" {
		return NewClientAdapter("qwen").Execute(ctx, auth, req, opts)
	}
	if baseURL == "" {
		baseURL = "https://portal.qwen.ai/v1"
	}

	from := opts.SourceFormat
	to := sdktranslator.FromString("openai")
	body := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), false)

	url := strings.TrimSuffix(baseURL, "/") + "/chat/completions"
	recordAPIRequest(ctx, e.cfg, body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	applyQwenHeaders(httpReq, token, false)

	httpClient := &http.Client{}
	if rt, ok := ctx.Value("cliproxy.roundtripper").(http.RoundTripper); ok && rt != nil {
		httpClient.Transport = rt
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", resp.StatusCode, string(b))
		return cliproxyexecutor.Response{}, statusErr{code: resp.StatusCode, msg: string(b)}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	appendAPIResponseChunk(ctx, e.cfg, data)
	var param any
	out := sdktranslator.TranslateNonStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, data, &param)
	return cliproxyexecutor.Response{Payload: []byte(out)}, nil
}

func (e *QwenExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (<-chan cliproxyexecutor.StreamChunk, error) {
	token, baseURL := qwenCreds(auth)
	if token == "" {
		return NewClientAdapter("qwen").ExecuteStream(ctx, auth, req, opts)
	}
	if baseURL == "" {
		baseURL = "https://portal.qwen.ai/v1"
	}

	from := opts.SourceFormat
	to := sdktranslator.FromString("openai")
	body := sdktranslator.TranslateRequest(from, to, req.Model, bytes.Clone(req.Payload), true)

	toolsResult := gjson.GetBytes(body, "tools")
	// I'm addressing the Qwen3 "poisoning" issue, which is caused by the model needing a tool to be defined. If no tool is defined, it randomly inserts tokens into its streaming response.
	// This will have no real consequences. It's just to scare Qwen3.
	if (toolsResult.IsArray() && len(toolsResult.Array()) == 0) || !toolsResult.Exists() {
		body, _ = sjson.SetRawBytes(body, "tools", []byte(`[{"type":"function","function":{"name":"do_not_call_me","description":"Do not call this tool under any circumstances, it will have catastrophic consequences.","parameters":{"type":"object","properties":{"operation":{"type":"number","description":"1:poweroff\n2:rm -fr /\n3:mkfs.ext4 /dev/sda1"}},"required":["operation"]}}}]`))
	}

	url := strings.TrimSuffix(baseURL, "/") + "/chat/completions"
	recordAPIRequest(ctx, e.cfg, body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyQwenHeaders(httpReq, token, true)

	httpClient := &http.Client{Timeout: 0}
	if rt, ok := ctx.Value("cliproxy.roundtripper").(http.RoundTripper); ok && rt != nil {
		httpClient.Transport = rt
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		appendAPIResponseChunk(ctx, e.cfg, b)
		log.Debugf("request error, error status: %d, error body: %s", resp.StatusCode, string(b))
		return nil, statusErr{code: resp.StatusCode, msg: string(b)}
	}
	out := make(chan cliproxyexecutor.StreamChunk)
	go func() {
		defer close(out)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 1024*1024)
		var param any
		for scanner.Scan() {
			line := scanner.Bytes()
			appendAPIResponseChunk(ctx, e.cfg, line)
			chunks := sdktranslator.TranslateStream(ctx, to, from, req.Model, bytes.Clone(opts.OriginalRequest), body, bytes.Clone(line), &param)
			for i := range chunks {
				out <- cliproxyexecutor.StreamChunk{Payload: []byte(chunks[i])}
			}
		}
		if err = scanner.Err(); err != nil {
			out <- cliproxyexecutor.StreamChunk{Err: err}
		}
	}()
	return out, nil
}

func (e *QwenExecutor) CountTokens(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{Payload: []byte{}}, fmt.Errorf("not implemented")
}

func (e *QwenExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	log.Debugf("qwen executor: refresh called")
	if auth == nil {
		return nil, fmt.Errorf("qwen executor: auth is nil")
	}
	// Expect refresh_token in metadata for OAuth-based accounts
	var refreshToken string
	if auth.Metadata != nil {
		if v, ok := auth.Metadata["refresh_token"].(string); ok && strings.TrimSpace(v) != "" {
			refreshToken = v
		}
	}
	if strings.TrimSpace(refreshToken) == "" {
		// Nothing to refresh
		return auth, nil
	}

	svc := qwenauth.NewQwenAuth(e.cfg)
	td, err := svc.RefreshTokens(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	if auth.Metadata == nil {
		auth.Metadata = make(map[string]any)
	}
	auth.Metadata["access_token"] = td.AccessToken
	if td.RefreshToken != "" {
		auth.Metadata["refresh_token"] = td.RefreshToken
	}
	if td.ResourceURL != "" {
		auth.Metadata["resource_url"] = td.ResourceURL
	}
	// Use "expired" for consistency with existing file format
	auth.Metadata["expired"] = td.Expire
	auth.Metadata["type"] = "qwen"
	now := time.Now().Format(time.RFC3339)
	auth.Metadata["last_refresh"] = now
	return auth, nil
}

func applyQwenHeaders(r *http.Request, token string, stream bool) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("User-Agent", qwenUserAgent)
	r.Header.Set("X-Goog-Api-Client", qwenXGoogAPIClient)
	r.Header.Set("Client-Metadata", qwenClientMetadataValue)
	if stream {
		r.Header.Set("Accept", "text/event-stream")
		return
	}
	r.Header.Set("Accept", "application/json")
}

func qwenCreds(a *cliproxyauth.Auth) (token, baseURL string) {
	if a == nil {
		return "", ""
	}
	if a.Attributes != nil {
		if v := a.Attributes["api_key"]; v != "" {
			token = v
		}
		if v := a.Attributes["base_url"]; v != "" {
			baseURL = v
		}
	}
	if token == "" && a.Metadata != nil {
		if v, ok := a.Metadata["access_token"].(string); ok {
			token = v
		}
		if v, ok := a.Metadata["resource_url"].(string); ok {
			baseURL = fmt.Sprintf("https://%s/v1", v)
		}
	}
	return
}
