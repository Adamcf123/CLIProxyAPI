package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/auth/claude"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/browser"
	// legacy client removed
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/misc"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	log "github.com/sirupsen/logrus"
)

// ClaudeAuthenticator implements the OAuth login flow for Anthropic Claude accounts.
type ClaudeAuthenticator struct {
	CallbackPort int
}

// NewClaudeAuthenticator constructs a Claude authenticator with default settings.
func NewClaudeAuthenticator() *ClaudeAuthenticator {
	return &ClaudeAuthenticator{CallbackPort: 54545}
}

func (a *ClaudeAuthenticator) Provider() string {
	return "claude"
}

func (a *ClaudeAuthenticator) RefreshLead() *time.Duration {
	d := 4 * time.Hour
	return &d
}

func (a *ClaudeAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*TokenRecord, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	pkceCodes, err := claude.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("claude pkce generation failed: %w", err)
	}

	state, err := misc.GenerateRandomState()
	if err != nil {
		return nil, fmt.Errorf("claude state generation failed: %w", err)
	}

	oauthServer := claude.NewOAuthServer(a.CallbackPort)
	if err = oauthServer.Start(); err != nil {
		if strings.Contains(err.Error(), "already in use") {
			return nil, claude.NewAuthenticationError(claude.ErrPortInUse, err)
		}
		return nil, claude.NewAuthenticationError(claude.ErrServerStartFailed, err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if stopErr := oauthServer.Stop(stopCtx); stopErr != nil {
			log.Warnf("claude oauth server stop error: %v", stopErr)
		}
	}()

	authSvc := claude.NewClaudeAuth(cfg)

	authURL, returnedState, err := authSvc.GenerateAuthURL(state, pkceCodes)
	if err != nil {
		return nil, fmt.Errorf("claude authorization url generation failed: %w", err)
	}
	state = returnedState

	if !opts.NoBrowser {
		log.Info("Opening browser for Claude authentication")
		if !browser.IsAvailable() {
			log.Warn("No browser available; please open the URL manually")
			util.PrintSSHTunnelInstructions(a.CallbackPort)
			log.Infof("Visit the following URL to continue authentication:\n%s", authURL)
		} else if err = browser.OpenURL(authURL); err != nil {
			log.Warnf("Failed to open browser automatically: %v", err)
			util.PrintSSHTunnelInstructions(a.CallbackPort)
			log.Infof("Visit the following URL to continue authentication:\n%s", authURL)
		}
	} else {
		util.PrintSSHTunnelInstructions(a.CallbackPort)
		log.Infof("Visit the following URL to continue authentication:\n%s", authURL)
	}

	log.Info("Waiting for Claude authentication callback...")

	result, err := oauthServer.WaitForCallback(5 * time.Minute)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return nil, claude.NewAuthenticationError(claude.ErrCallbackTimeout, err)
		}
		return nil, err
	}

	if result.Error != "" {
		return nil, claude.NewOAuthError(result.Error, "", http.StatusBadRequest)
	}

	if result.State != state {
		return nil, claude.NewAuthenticationError(claude.ErrInvalidState, fmt.Errorf("state mismatch"))
	}

	log.Debug("Claude authorization code received; exchanging for tokens")

	authBundle, err := authSvc.ExchangeCodeForTokens(ctx, result.Code, state, pkceCodes)
	if err != nil {
		return nil, claude.NewAuthenticationError(claude.ErrCodeExchangeFailed, err)
	}

	tokenStorage := authSvc.CreateTokenStorage(authBundle)

	if tokenStorage == nil || tokenStorage.Email == "" {
		return nil, fmt.Errorf("claude token storage missing account information")
	}

	fileName := fmt.Sprintf("claude-%s.json", tokenStorage.Email)
	metadata := map[string]string{
		"email": tokenStorage.Email,
	}

	log.Info("Claude authentication successful")
	if authBundle.APIKey != "" {
		log.Info("Claude API key obtained and stored")
	}

	return &TokenRecord{
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  tokenStorage,
		Metadata: metadata,
	}, nil
}
