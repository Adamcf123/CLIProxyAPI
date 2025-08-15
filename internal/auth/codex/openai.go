package codex

// PKCECodes holds PKCE verification codes for OAuth2 PKCE flow
type PKCECodes struct {
	// CodeVerifier is the cryptographically random string used to correlate
	// the authorization request to the token request
	CodeVerifier string `json:"code_verifier"`
	// CodeChallenge is the SHA256 hash of the code verifier, base64url-encoded
	CodeChallenge string `json:"code_challenge"`
}

// CodexTokenData holds OAuth token information from OpenAI
type CodexTokenData struct {
	// IDToken is the JWT ID token containing user claims
	IDToken string `json:"id_token"`
	// AccessToken is the OAuth2 access token for API access
	AccessToken string `json:"access_token"`
	// RefreshToken is used to obtain new access tokens
	RefreshToken string `json:"refresh_token"`
	// AccountID is the OpenAI account identifier
	AccountID string `json:"account_id"`
	// Email is the OpenAI account email
	Email string `json:"email"`
	// Expire is the timestamp of the token expire
	Expire string `json:"expired"`
}

// CodexAuthBundle aggregates authentication data after OAuth flow completion
type CodexAuthBundle struct {
	// APIKey is the OpenAI API key obtained from token exchange
	APIKey string `json:"api_key"`
	// TokenData contains the OAuth tokens from the authentication flow
	TokenData CodexTokenData `json:"token_data"`
	// LastRefresh is the timestamp of the last token refresh
	LastRefresh string `json:"last_refresh"`
}
