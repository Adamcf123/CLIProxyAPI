// Package gemini provides authentication and token management functionality
// for Google's Gemini AI services. It handles OAuth2 authentication flows,
// including obtaining tokens via web-based authorization, storing tokens,
// and refreshing them when they expire.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/auth/codex"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"golang.org/x/net/proxy"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	geminiOauthClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	geminiOauthClientSecret = "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"
)

var (
	geminiOauthScopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
	}
)

// GeminiAuth provides methods for handling the Gemini OAuth2 authentication flow.
// It encapsulates the logic for obtaining, storing, and refreshing authentication tokens
// for Google's Gemini AI services.
type GeminiAuth struct {
}

// NewGeminiAuth creates a new instance of GeminiAuth.
func NewGeminiAuth() *GeminiAuth {
	return &GeminiAuth{}
}

// GetAuthenticatedClient configures and returns an HTTP client ready for making authenticated API calls.
// It manages the entire OAuth2 flow, including handling proxies, loading existing tokens,
// initiating a new web-based OAuth flow if necessary, and refreshing tokens.
//
// Parameters:
//   - ctx: The context for the HTTP client
//   - ts: The Gemini token storage containing authentication tokens
//   - cfg: The configuration containing proxy settings
//   - noBrowser: Optional parameter to disable browser opening
//
// Returns:
//   - *http.Client: An HTTP client configured with authentication
//   - error: An error if the client configuration fails, nil otherwise
func (g *GeminiAuth) GetAuthenticatedClient(ctx context.Context, ts *GeminiTokenStorage, cfg *config.Config, noBrowser ...bool) (*http.Client, error) {
	// Configure proxy settings for the HTTP client if a proxy URL is provided.
	proxyURL, err := url.Parse(cfg.ProxyURL)
	if err == nil {
		var transport *http.Transport
		if proxyURL.Scheme == "socks5" {
			// Handle SOCKS5 proxy.
			username := proxyURL.User.Username()
			password, _ := proxyURL.User.Password()
			auth := &proxy.Auth{User: username, Password: password}
			dialer, errSOCKS5 := proxy.SOCKS5("tcp", proxyURL.Host, auth, proxy.Direct)
			if errSOCKS5 != nil {
				log.Fatalf("create SOCKS5 dialer failed: %v", errSOCKS5)
			}
			transport = &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				},
			}
		} else if proxyURL.Scheme == "http" || proxyURL.Scheme == "https" {
			// Handle HTTP/HTTPS proxy.
			transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}

		if transport != nil {
			proxyClient := &http.Client{Transport: transport}
			ctx = context.WithValue(ctx, oauth2.HTTPClient, proxyClient)
		}
	}

	// Configure the OAuth2 client.
	conf := &oauth2.Config{
		ClientID:     geminiOauthClientID,
		ClientSecret: geminiOauthClientSecret,
		RedirectURL:  "http://localhost:8085/oauth2callback", // This will be used by the local server.
		Scopes:       geminiOauthScopes,
		Endpoint:     google.Endpoint,
	}

	var token *oauth2.Token

	// If no token is found in storage, initiate the web-based OAuth flow.
	if ts.Token == nil {
		fmt.Printf("Could not load token from file, starting OAuth flow.\n")
		token, err = g.getTokenFromWeb(ctx, conf, noBrowser...)
		if err != nil {
			return nil, fmt.Errorf("failed to get token from web: %w", err)
		}
		// After getting a new token, create a new token storage object with user info.
		newTs, errCreateTokenStorage := g.createTokenStorage(ctx, conf, token, ts.ProjectID)
		if errCreateTokenStorage != nil {
			log.Errorf("Warning: failed to create token storage: %v", errCreateTokenStorage)
			return nil, errCreateTokenStorage
		}
		*ts = *newTs
	}

	// Unmarshal the stored token into an oauth2.Token object.
	tsToken, _ := json.Marshal(ts.Token)
	if err = json.Unmarshal(tsToken, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Return an HTTP client that automatically handles token refreshing.
	return conf.Client(ctx, token), nil
}

// createTokenStorage creates a new GeminiTokenStorage object. It fetches the user's email
// using the provided token and populates the storage structure.
//
// Parameters:
//   - ctx: The context for the HTTP request
//   - config: The OAuth2 configuration
//   - token: The OAuth2 token to use for authentication
//   - projectID: The Google Cloud Project ID to associate with this token
//
// Returns:
//   - *GeminiTokenStorage: A new token storage object with user information
//   - error: An error if the token storage creation fails, nil otherwise
func (g *GeminiAuth) createTokenStorage(ctx context.Context, config *oauth2.Config, token *oauth2.Token, projectID string) (*GeminiTokenStorage, error) {
	httpClient := config.Client(ctx, token)
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
	if err != nil {
		return nil, fmt.Errorf("could not get user info: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			log.Printf("warn: failed to close response body: %v", err)
		}
	}()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get user info request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	emailResult := gjson.GetBytes(bodyBytes, "email")
	if emailResult.Exists() && emailResult.Type == gjson.String {
		fmt.Printf("Authenticated user email: %s\n", emailResult.String())
	} else {
		fmt.Println("Failed to get user email from token")
	}

	var ifToken map[string]any
	jsonData, _ := json.Marshal(token)
	err = json.Unmarshal(jsonData, &ifToken)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	ifToken["token_uri"] = "https://oauth2.googleapis.com/token"
	ifToken["client_id"] = geminiOauthClientID
	ifToken["client_secret"] = geminiOauthClientSecret
	ifToken["scopes"] = geminiOauthScopes
	ifToken["universe_domain"] = "googleapis.com"

	ts := GeminiTokenStorage{
		Token:     ifToken,
		ProjectID: projectID,
		Email:     emailResult.String(),
	}

	return &ts, nil
}

// getTokenFromWeb initiates the web-based OAuth2 authorization flow.
// It starts a local HTTP server to listen for the callback from Google's auth server,
// opens the user's browser to the authorization URL, and exchanges the received
// authorization code for an access token.
//
// Parameters:
//   - ctx: The context for the HTTP client
//   - config: The OAuth2 configuration
//   - noBrowser: Optional parameter to disable browser opening
//
// Returns:
//   - *oauth2.Token: The OAuth2 token obtained from the authorization flow
//   - error: An error if the token acquisition fails, nil otherwise
func (g *GeminiAuth) getTokenFromWeb(ctx context.Context, config *oauth2.Config, noBrowser ...bool) (*oauth2.Token, error) {
	// Use a channel to pass the authorization code from the HTTP handler to the main function.
	codeChan := make(chan string)
	errChan := make(chan error)

	// Create a new HTTP server with its own multiplexer.
	mux := http.NewServeMux()
	server := &http.Server{Addr: ":8085", Handler: mux}
	config.RedirectURL = "http://localhost:8085/oauth2callback"

	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		if err := r.URL.Query().Get("error"); err != "" {
			_, _ = fmt.Fprintf(w, "Authentication failed: %s", err)
			errChan <- fmt.Errorf("authentication failed via callback: %s", err)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			_, _ = fmt.Fprint(w, "Authentication failed: code not found.")
			errChan <- fmt.Errorf("code not found in callback")
			return
		}
		_, _ = fmt.Fprint(w, "<html><body><h1>Authentication successful!</h1><p>You can close this window.</p></body></html>")
		codeChan <- code
	})

	// Start the server in a goroutine.
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	// Open the authorization URL in the user's browser.
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))

	if len(noBrowser) == 1 && !noBrowser[0] {
		fmt.Println("Opening browser for authentication...")

		// Check if browser is available
		if !browser.IsAvailable() {
			log.Warn("No browser available on this system")
			util.PrintSSHTunnelInstructions(8085)
			fmt.Printf("Please manually open this URL in your browser:\n\n%s\n", authURL)
		} else {
			if err := browser.OpenURL(authURL); err != nil {
				authErr := codex.NewAuthenticationError(codex.ErrBrowserOpenFailed, err)
				log.Warn(codex.GetUserFriendlyMessage(authErr))
				util.PrintSSHTunnelInstructions(8085)
				fmt.Printf("Please manually open this URL in your browser:\n\n%s\n", authURL)

				// Log platform info for debugging
				platformInfo := browser.GetPlatformInfo()
				log.Debugf("Browser platform info: %+v", platformInfo)
			} else {
				log.Debug("Browser opened successfully")
			}
		}
	} else {
		util.PrintSSHTunnelInstructions(8085)
		fmt.Printf("Please open this URL in your browser:\n\n%s\n", authURL)
	}

	fmt.Println("Waiting for authentication callback...")

	// Wait for the authorization code or an error.
	var authCode string
	select {
	case code := <-codeChan:
		authCode = code
	case err := <-errChan:
		return nil, err
	case <-time.After(5 * time.Minute): // Timeout
		return nil, fmt.Errorf("oauth flow timed out")
	}

	// Shutdown the server.
	if err := server.Shutdown(ctx); err != nil {
		log.Errorf("Failed to shut down server: %v", err)
	}

	// Exchange the authorization code for a token.
	token, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	fmt.Println("Authentication successful.")
	return token, nil
}
