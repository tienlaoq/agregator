package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
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
	authClient authv1.AuthServiceClient
	google     *oauth2.Config
	vk         *oauth2.Config
	frontURL   string
}

func NewOAuthHandler(authClient authv1.AuthServiceClient, cfg OAuthConfig) *OAuthHandler {
	h := &OAuthHandler{
		authClient: authClient,
		frontURL:   cfg.FrontendURL,
	}

	if cfg.GoogleClientID != "" {
		h.google = &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.BaseURL + "/api/v1/auth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		}
	}

	if cfg.VKClientID != "" {
		h.vk = &oauth2.Config{
			ClientID:     cfg.VKClientID,
			ClientSecret: cfg.VKClientSecret,
			RedirectURL:  cfg.BaseURL + "/api/v1/auth/vk/callback",
			Scopes:       []string{"vkid.personal_info", "vkid.email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://id.vk.com/authorize",
				TokenURL: "https://id.vk.com/oauth2/auth",
			},
		}
	}

	return h
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// --- Google ---

func (h *OAuthHandler) GoogleRedirect(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Google OAuth not configured"})
		return
	}
	state := generateState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.google.AuthCodeURL(state), http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Google OAuth not configured"})
		return
	}

	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value != r.URL.Query().Get("state") {
		h.redirectError(w, r, "invalid state parameter")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectError(w, r, "authorization denied")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	token, err := h.google.Exchange(ctx, code)
	if err != nil {
		h.redirectError(w, r, "failed to exchange code")
		return
	}

	userInfo, err := fetchGoogleUserInfo(ctx, h.google, token)
	if err != nil {
		h.redirectError(w, r, "failed to get user info")
		return
	}

	h.oauthLogin(w, r, "google", userInfo.ID, userInfo.Email, userInfo.Name, userInfo.Picture)
}

type googleUserInfo struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
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
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "VK OAuth not configured"})
		return
	}
	state := generateState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	authURL := fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s&code_challenge_method=plain&code_challenge=%s",
		h.vk.Endpoint.AuthURL,
		h.vk.ClientID,
		url.QueryEscape(h.vk.RedirectURL),
		url.QueryEscape("vkid.personal_info vkid.email"),
		state,
		state,
	)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) VKCallback(w http.ResponseWriter, r *http.Request) {
	if h.vk == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "VK OAuth not configured"})
		return
	}

	cookie, err := r.Cookie("oauth_state")
	state := r.URL.Query().Get("state")
	deviceID := r.URL.Query().Get("device_id")
	if err != nil || cookie.Value != state {
		h.redirectError(w, r, "invalid state parameter")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectError(w, r, "authorization denied")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// VK ID token exchange via POST form
	vkTokenResp, err := exchangeVKCode(ctx, h.vk, code, state, deviceID)
	if err != nil {
		h.redirectError(w, r, "failed to exchange code")
		return
	}

	// Get user info from VK ID
	vkUser, err := fetchVKIDUserInfo(ctx, vkTokenResp.AccessToken)
	if err != nil {
		h.redirectError(w, r, "failed to get user info")
		return
	}

	name := vkUser.FirstName
	if vkUser.LastName != "" {
		name += " " + vkUser.LastName
	}

	h.oauthLogin(w, r, "vk", fmt.Sprintf("%d", vkUser.UserID), vkUser.Email, name, vkUser.Avatar)
}

type vkTokenResponse struct {
	AccessToken string `json:"access_token"`
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
}

func exchangeVKCode(ctx context.Context, cfg *oauth2.Config, code, state, deviceID string) (*vkTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {cfg.RedirectURL},
		"code":          {code},
		"code_verifier": {state},
		"device_id":     {deviceID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.vk.com/oauth2/auth",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
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

func fetchVKIDUserInfo(ctx context.Context, accessToken string) (*vkIDUserInfo, error) {
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

	resp, err := http.DefaultClient.Do(req)
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

func (h *OAuthHandler) oauthLogin(w http.ResponseWriter, r *http.Request, provider, providerID, email, name, avatar string) {
	if email == "" {
		h.redirectError(w, r, "email not provided by "+provider)
		return
	}

	resp, err := h.authClient.OAuthLogin(r.Context(), &authv1.OAuthLoginRequest{
		Provider:   provider,
		ProviderId: providerID,
		Email:      email,
		Name:       name,
		AvatarUrl:  avatar,
	})
	if err != nil {
		h.redirectError(w, r, "authentication failed")
		return
	}

	callbackURL := fmt.Sprintf("%s/auth/callback?access_token=%s&refresh_token=%s&user_id=%s",
		h.frontURL,
		url.QueryEscape(resp.AccessToken),
		url.QueryEscape(resp.RefreshToken),
		url.QueryEscape(resp.UserId),
	)
	http.Redirect(w, r, callbackURL, http.StatusTemporaryRedirect)
}

func (h *OAuthHandler) redirectError(w http.ResponseWriter, r *http.Request, msg string) {
	errURL := fmt.Sprintf("%s/auth/login?error=%s", h.frontURL, url.QueryEscape(msg))
	http.Redirect(w, r, errURL, http.StatusTemporaryRedirect)
}
