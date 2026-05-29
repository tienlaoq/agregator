package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// vkHTTPTimeout is the end-to-end deadline for VK ID API calls (token
// exchange and user-info). It is intentionally shorter than the per-request
// context timeout set by VKCallback (10 s) so that a slow VK endpoint does
// not pin a goroutine for the full handler budget.
const vkHTTPTimeout = 8 * time.Second

// newVKHTTPClient returns a dedicated *http.Client for VK ID API calls.
// Using a private client instead of http.DefaultClient ensures:
//   - an explicit Timeout that covers dial + TLS + response body read
//   - a private Transport so connection-pool tuning does not bleed into
//     other outbound HTTP made by the process
func newVKHTTPClient() *http.Client {
	return &http.Client{
		Timeout: vkHTTPTimeout,
		Transport: &http.Transport{
			// Keep VK keep-alive connections separate from the default pool.
			MaxIdleConns:        10,
			IdleConnTimeout:     60 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
}

// oauthErrorCode is an opaque enum that the frontend uses to decide which
// localised error message to show. Raw internal details (exchange errors,
// provider messages) must never appear in these codes — they are logged
// server-side instead.
type oauthErrorCode string

const (
	// oauthErrStateMismatch — the oauth_state cookie did not match the state
	// query parameter. Usually caused by an expired cookie or a CSRF attempt.
	oauthErrStateMismatch oauthErrorCode = "oauth_state_mismatch"

	// oauthErrDenied — the user denied the authorisation request in the
	// provider's consent screen (code param absent).
	oauthErrDenied oauthErrorCode = "oauth_denied"

	// oauthErrMissingPKCE — the PKCE verifier cookie was absent or empty at
	// callback time. Caused by an expired cookie or a replay attempt.
	oauthErrMissingPKCE oauthErrorCode = "oauth_missing_pkce"

	// oauthErrExchangeFailed — the server-to-server token exchange with the
	// OAuth provider failed. The underlying error is logged but not forwarded.
	oauthErrExchangeFailed oauthErrorCode = "oauth_exchange_failed"

	// oauthErrUserInfoFailed — fetching the user profile from the provider
	// succeeded at the token level but the user-info endpoint returned an error.
	oauthErrUserInfoFailed oauthErrorCode = "oauth_user_info_failed"

	// oauthErrNoEmail — the provider did not return an email address. This
	// happens when the user's account has no verified email or the scope was
	// not granted.
	oauthErrNoEmail oauthErrorCode = "oauth_no_email"

	// oauthErrAuthFailed — the downstream auth-service rejected the login
	// (e.g. account banned, duplicate provider). The underlying error is logged.
	oauthErrAuthFailed oauthErrorCode = "oauth_auth_failed"
)

type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	VKClientID         string
	VKClientSecret     string
	BaseURL            string
	FrontendURL        string
}

type OAuthHandler struct {
	authClient   authv1.AuthServiceClient
	google       *oauth2.Config
	vk           *oauth2.Config
	vkClient     *http.Client // dedicated client for VK ID API — never http.DefaultClient
	frontURL     string
	secureCookie bool // true when BASE_URL is https — sets Secure flag on state cookie
	log          zerolog.Logger
}

// validateOAuthURL parses rawURL and enforces:
//   - non-empty scheme and host (guards against relative or opaque URLs)
//   - scheme == "https" when ENV=production (prevents plaintext OAuth in prod)
//
// The urlLabel is used only in error messages.
func validateOAuthURL(rawURL, urlLabel string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("oauth: %s must not be empty", urlLabel)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("oauth: %s %q is not a valid URL: %w", urlLabel, rawURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("oauth: %s %q must include a scheme and host (e.g. https://api.example.com)", urlLabel, rawURL)
	}
	isProduction := strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")
	if isProduction && u.Scheme != "https" {
		return nil, fmt.Errorf("oauth: %s %q must use https in production (ENV=production)", urlLabel, rawURL)
	}
	return u, nil
}

// NewOAuthHandler constructs the OAuth handler and validates BaseURL / FrontendURL.
// Returns an error when either URL is invalid or insecure for the current environment,
// so callers (main.go) can treat it as a fatal misconfiguration.
func NewOAuthHandler(log zerolog.Logger, authClient authv1.AuthServiceClient, cfg OAuthConfig) (*OAuthHandler, error) {
	baseU, err := validateOAuthURL(cfg.BaseURL, "BASE_URL")
	if err != nil {
		return nil, err
	}
	if _, err := validateOAuthURL(cfg.FrontendURL, "FRONTEND_URL"); err != nil {
		return nil, err
	}

	// Normalise: strip any trailing slash so callback paths join cleanly.
	baseURL := strings.TrimRight(baseU.String(), "/")

	h := &OAuthHandler{
		authClient:   authClient,
		vkClient:     newVKHTTPClient(),
		frontURL:     strings.TrimRight(strings.TrimSpace(cfg.FrontendURL), "/"),
		secureCookie: baseU.Scheme == "https",
		log:          log,
	}

	if cfg.GoogleClientID != "" {
		h.google = &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  baseURL + "/api/v1/auth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}
	}

	if cfg.VKClientID != "" {
		h.vk = &oauth2.Config{
			ClientID:     cfg.VKClientID,
			ClientSecret: cfg.VKClientSecret,
			RedirectURL:  baseURL + "/api/v1/auth/vk/callback",
			Scopes:       []string{"vkid.personal_info", "vkid.email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://id.vk.com/authorize",
				TokenURL: "https://id.vk.com/oauth2/auth",
			},
		}
	}

	return h, nil
}

// setStateCookie writes a provider-scoped OAuth state cookie.
// Using a distinct name per provider prevents a parallel Google + VK login
// flow from overwriting each other's state and causing spurious state_mismatch
// errors at callback time.
// Secure is set to true automatically when the gateway's BASE_URL uses https —
// this prevents the cookie from being sent over plaintext HTTP connections.
func (h *OAuthHandler) setStateCookie(w http.ResponseWriter, cookieName, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

// setPKCECookie stores the PKCE code_verifier in a separate HttpOnly cookie.
// It is intentionally distinct from oauth_state so that leaking one does not
// compromise the other (state via Referer ≠ verifier needed at token exchange).
func (h *OAuthHandler) setPKCECookie(w http.ResponseWriter, verifier string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "vk_pkce_verifier",
		Value:    verifier,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearPKCECookie expires the PKCE verifier cookie after a successful or
// failed callback — a consumed verifier must not be reusable.
func (h *OAuthHandler) clearPKCECookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "vk_pkce_verifier",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generateState: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generatePKCE returns a (verifier, challenge) pair for PKCE S256.
//
// verifier  — 32 random bytes encoded as base64url (43 chars, within RFC 7636 range of 43–128).
// challenge — BASE64URL(SHA256(verifier)), as required by S256 method.
//
// The verifier is kept secret server-side (stored in an HttpOnly cookie).
// Only the challenge is sent to the authorization server.
func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		err = fmt.Errorf("generatePKCE: %w", err)
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

// --- Google ---

func (h *OAuthHandler) GoogleRedirect(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeCatalog(w, apicatalog.GatewayOauthGoogleNotConfigured)
		return
	}
	state, err := generateState()
	if err != nil {
		h.log.Error().Err(err).Msg("google redirect: failed to generate state")
		writeCatalog(w, apicatalog.GatewayUpstreamInternal)
		return
	}
	h.setStateCookie(w, "google_oauth_state", state)
	http.Redirect(w, r, h.google.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeCatalog(w, apicatalog.GatewayOauthGoogleNotConfigured)
		return
	}

	cookie, err := r.Cookie("google_oauth_state")
	if err != nil || cookie.Value != r.URL.Query().Get("state") {
		h.redirectError(w, r, oauthErrStateMismatch, err)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectError(w, r, oauthErrDenied, nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	token, err := h.google.Exchange(ctx, code)
	if err != nil {
		h.redirectError(w, r, oauthErrExchangeFailed, err)
		return
	}

	userInfo, err := fetchGoogleUserInfo(ctx, h.google, token)
	if err != nil {
		h.redirectError(w, r, oauthErrUserInfoFailed, err)
		return
	}

	h.oauthLogin(w, r, "google", userInfo.ID, userInfo.Email, userInfo.VerifiedEmail, userInfo.Name, userInfo.Picture)
}

type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	// VerifiedEmail is set by Google when the address has passed their verification
	// flow. Only when true may the email be used to link to an existing account.
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func fetchGoogleUserInfo(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (*googleUserInfo, error) {
	client := cfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

// --- VK ---

func (h *OAuthHandler) VKRedirect(w http.ResponseWriter, r *http.Request) {
	if h.vk == nil {
		writeCatalog(w, apicatalog.GatewayOauthVkNotConfigured)
		return
	}
	state, err := generateState()
	if err != nil {
		h.log.Error().Err(err).Msg("vk redirect: failed to generate state")
		writeCatalog(w, apicatalog.GatewayUpstreamInternal)
		return
	}
	h.setStateCookie(w, "vk_oauth_state", state)

	// PKCE S256: verifier and state are independent secrets.
	// Even if state leaks via Referer the verifier remains unknown to the attacker.
	verifier, challenge, err := generatePKCE()
	if err != nil {
		h.log.Error().Err(err).Msg("vk redirect: failed to generate PKCE")
		writeCatalog(w, apicatalog.GatewayUpstreamInternal)
		return
	}
	h.setPKCECookie(w, verifier)

	authU, _ := url.Parse(h.vk.Endpoint.AuthURL)
	aq := url.Values{}
	aq.Set("response_type", "code")
	aq.Set("client_id", h.vk.ClientID)
	aq.Set("redirect_uri", h.vk.RedirectURL)
	aq.Set("scope", "vkid.personal_info vkid.email")
	aq.Set("state", state)
	aq.Set("code_challenge_method", "S256")
	aq.Set("code_challenge", challenge)
	authU.RawQuery = aq.Encode()
	http.Redirect(w, r, authU.String(), http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) VKCallback(w http.ResponseWriter, r *http.Request) {
	if h.vk == nil {
		writeCatalog(w, apicatalog.GatewayOauthVkNotConfigured)
		return
	}

	stateCookie, err := r.Cookie("vk_oauth_state")
	state := r.URL.Query().Get("state")
	deviceID := r.URL.Query().Get("device_id")
	if err != nil || stateCookie.Value != state {
		h.redirectError(w, r, oauthErrStateMismatch, err)
		return
	}

	// Retrieve and immediately invalidate the PKCE verifier cookie.
	// Clearing on every path (error or success) prevents verifier reuse.
	pkceCookie, err := r.Cookie("vk_pkce_verifier")
	h.clearPKCECookie(w)
	if err != nil || pkceCookie.Value == "" {
		h.redirectError(w, r, oauthErrMissingPKCE, err)
		return
	}
	codeVerifier := pkceCookie.Value

	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectError(w, r, oauthErrDenied, nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// VK ID token exchange via POST form — pass the real PKCE verifier, not state.
	vkTokenResp, err := exchangeVKCode(ctx, h.vkClient, h.vk, code, codeVerifier, deviceID)
	if err != nil {
		h.redirectError(w, r, oauthErrExchangeFailed, err)
		return
	}

	// Get user info from VK ID
	vkUser, err := fetchVKIDUserInfo(ctx, h.vkClient, vkTokenResp.AccessToken)
	if err != nil {
		h.redirectError(w, r, oauthErrUserInfoFailed, err)
		return
	}

	name := vkUser.FirstName
	if vkUser.LastName != "" {
		name += " " + vkUser.LastName
	}

	// VK ID only returns an email when the user granted the vkid.email scope AND
	// the address is confirmed on their side — treat any non-empty email as verified.
	vkEmailVerified := vkUser.Email != ""
	h.oauthLogin(w, r, "vk", fmt.Sprintf("%d", vkUser.UserID), vkUser.Email, vkEmailVerified, name, vkUser.Avatar)
}

type vkTokenResponse struct {
	AccessToken string `json:"access_token"`
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
}

func exchangeVKCode(ctx context.Context, client *http.Client, cfg *oauth2.Config, code, codeVerifier, deviceID string) (*vkTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {cfg.RedirectURL},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"device_id":     {deviceID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.vk.com/oauth2/auth",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp vkTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token from VK")
	}
	return &tokenResp, nil
}

type vkIDUserInfo struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Avatar    string `json:"avatar"`
}

func fetchVKIDUserInfo(ctx context.Context, client *http.Client, accessToken string) (*vkIDUserInfo, error) {
	form := url.Values{
		"access_token": {accessToken},
		"client_id":    {},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.vk.com/oauth2/user_info",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		User vkIDUserInfo `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result.User, nil
}

// --- common ---

// refreshTokenMaxAge is the lifetime of the refresh-token cookie.
// Must stay in sync with the frontend /api/auth/set-refresh route (30 days).
const refreshTokenMaxAge = 60 * 60 * 24 * 30 // 30 days in seconds

func (h *OAuthHandler) oauthLogin(w http.ResponseWriter, r *http.Request, provider, providerID, email string, emailVerified bool, name, avatar string) {
	if email == "" {
		h.redirectError(w, r, oauthErrNoEmail, nil)
		return
	}

	resp, err := h.authClient.OAuthLogin(r.Context(), &authv1.OAuthLoginRequest{
		Provider:      provider,
		ProviderId:    providerID,
		Email:         email,
		Name:          name,
		AvatarUrl:     avatar,
		EmailVerified: emailVerified,
	})
	if err != nil {
		h.redirectError(w, r, oauthErrAuthFailed, err)
		return
	}

	// ── Refresh token → HttpOnly cookie ──────────────────────────────────────
	// The refresh token must never appear in a URL: URLs end up in proxy/CDN
	// logs, browser history, and Referer headers. Set it as an HttpOnly cookie
	// so client-side JavaScript cannot read it.
	//
	// The cookie name "banya_refresh" must stay in sync with:
	//   frontend/src/app/api/auth/set-refresh/route.ts  (REFRESH_COOKIE)
	//   frontend/src/app/api/auth/refresh/route.ts      (REFRESH_COOKIE)
	if resp.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "banya_refresh",
			Value:    resp.RefreshToken,
			Path:     "/",
			MaxAge:   refreshTokenMaxAge,
			HttpOnly: true,
			Secure:   h.secureCookie,
			SameSite: http.SameSiteLaxMode,
		})
	}

	// ── Access token → URL fragment ───────────────────────────────────────────
	// Fragments (#…) are never sent to the server, never appear in proxy logs,
	// and are not included in the Referer header. They are visible in browser
	// history, but that is acceptable for a short-lived access token (vs. a
	// long-lived refresh token which must never appear in any URL).
	//
	// user_id is omitted: the frontend derives it from GET /api/v1/users/me
	// after storing the token, avoiding a redundant URL parameter.
	//
	// url.Parse cannot fail here — h.frontURL was validated at construction.
	callbackU, _ := url.Parse(h.frontURL + "/auth/callback")
	frag := url.Values{}
	frag.Set("access_token", resp.AccessToken)
	callbackU.Fragment = frag.Encode()
	http.Redirect(w, r, callbackU.String(), http.StatusTemporaryRedirect)
}

// redirectError redirects the browser to the frontend login page with an
// opaque error code. The code is safe to expose in the URL — it is an enum
// value that the frontend maps to a localised message.
//
// internalErr, when non-nil, is logged at Warn level so operators can diagnose
// failures without leaking implementation details to the client.
func (h *OAuthHandler) redirectError(w http.ResponseWriter, r *http.Request, code oauthErrorCode, internalErr error) {
	ev := h.log.Warn().Str("oauth_error", string(code))
	if internalErr != nil {
		ev = ev.Err(internalErr)
	}
	ev.Msg("oauth callback error")

	// Same url.Values pattern as oauthLogin: avoids manual QueryEscape.
	// url.Parse cannot fail here — h.frontURL was validated at construction.
	errU, _ := url.Parse(h.frontURL + "/auth/login")
	eq := url.Values{}
	eq.Set("error", string(code))
	errU.RawQuery = eq.Encode()
	http.Redirect(w, r, errU.String(), http.StatusTemporaryRedirect)
}
