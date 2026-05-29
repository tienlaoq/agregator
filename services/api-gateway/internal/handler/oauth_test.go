package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generatePKCE tests ---------------------------------------------------

func TestGeneratePKCE_verifierLength(t *testing.T) {
	verifier, _, err := generatePKCE()
	require.NoError(t, err)
	// RFC 7636: verifier must be 43-128 chars of base64url alphabet.
	assert.GreaterOrEqual(t, len(verifier), 43)
	assert.LessOrEqual(t, len(verifier), 128)
}

func TestGeneratePKCE_challengeIsS256(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	require.NoError(t, err)
	// Independently compute S256 challenge and compare.
	sum := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	assert.Equal(t, expected, challenge, "challenge must be BASE64URL(SHA256(verifier))")
}

func TestGeneratePKCE_verifierAndChallengeAreDifferent(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	require.NoError(t, err)
	assert.NotEqual(t, verifier, challenge, "verifier and challenge must differ (S256, not plain)")
}

func TestGeneratePKCE_uniqueEachCall(t *testing.T) {
	v1, c1, err1 := generatePKCE()
	v2, c2, err2 := generatePKCE()
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotEqual(t, v1, v2, "verifiers must be unique across calls")
	assert.NotEqual(t, c1, c2, "challenges must be unique across calls")
}

func TestGeneratePKCE_base64urlAlphabetOnly(t *testing.T) {
	for i := 0; i < 20; i++ {
		verifier, challenge, err := generatePKCE()
		require.NoError(t, err)
		_, err = base64.RawURLEncoding.DecodeString(verifier)
		assert.NoError(t, err, "verifier must be valid base64url: %s", verifier)
		_, err = base64.RawURLEncoding.DecodeString(challenge)
		assert.NoError(t, err, "challenge must be valid base64url: %s", challenge)
	}
}

// validateOAuthURL tests -----------------------------------------------

func TestValidateOAuthURL_valid(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"http localhost", "http://localhost:8080"},
		{"https prod", "https://api.example.com"},
		{"https with path", "https://api.example.com/prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := validateOAuthURL(tc.url, "TEST_URL")
			require.NoError(t, err)
			assert.NotNil(t, u)
		})
	}
}

func TestValidateOAuthURL_empty(t *testing.T) {
	_, err := validateOAuthURL("", "BASE_URL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestValidateOAuthURL_noScheme(t *testing.T) {
	_, err := validateOAuthURL("example.com/path", "BASE_URL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheme and host")
}

func TestValidateOAuthURL_invalidURL(t *testing.T) {
	_, err := validateOAuthURL("://broken", "BASE_URL")
	require.Error(t, err)
}

func TestValidateOAuthURL_httpForbiddenInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	_, err := validateOAuthURL("http://api.example.com", "BASE_URL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}

func TestValidateOAuthURL_httpsAllowedInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	u, err := validateOAuthURL("https://api.example.com", "BASE_URL")
	require.NoError(t, err)
	assert.Equal(t, "https", u.Scheme)
}

func TestValidateOAuthURL_httpAllowedOutsideProduction(t *testing.T) {
	t.Setenv("ENV", "development")
	_, err := validateOAuthURL("http://localhost:8080", "BASE_URL")
	require.NoError(t, err)
}

// NewOAuthHandler construction tests -----------------------------------

func TestNewOAuthHandler_invalidBaseURL(t *testing.T) {
	_, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:     "not-a-url",
		FrontendURL: "http://localhost:3000",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BASE_URL")
}

func TestNewOAuthHandler_invalidFrontendURL(t *testing.T) {
	_, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:     "http://localhost:8080",
		FrontendURL: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FRONTEND_URL")
}

func TestNewOAuthHandler_httpBaseURL_secureCookieFalse(t *testing.T) {
	h, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:     "http://localhost:8080",
		FrontendURL: "http://localhost:3000",
	})
	require.NoError(t, err)
	assert.False(t, h.secureCookie, "http BASE_URL must not set Secure on cookie")
}

func TestNewOAuthHandler_httpsBaseURL_secureCookieTrue(t *testing.T) {
	h, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:     "https://api.example.com",
		FrontendURL: "https://app.example.com",
	})
	require.NoError(t, err)
	assert.True(t, h.secureCookie, "https BASE_URL must set Secure on cookie")
}

func TestNewOAuthHandler_trailingSlashStripped(t *testing.T) {
	h, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:        "https://api.example.com/",
		FrontendURL:    "https://app.example.com/",
		GoogleClientID: "gid",
		GoogleClientSecret: "gsecret",
	})
	require.NoError(t, err)
	require.NotNil(t, h.google)
	assert.Equal(t, "https://api.example.com/api/v1/auth/google/callback", h.google.RedirectURL,
		"trailing slash in BaseURL must not produce double slash in redirect URI")
}

func TestNewOAuthHandler_productionRequiresHTTPS(t *testing.T) {
	t.Setenv("ENV", "production")
	_, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:     "http://api.example.com",
		FrontendURL: "https://app.example.com",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}

// setStateCookie tests -------------------------------------------------

func TestSetStateCookie_insecureWhenHTTP(t *testing.T) {
	h, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:     "http://localhost:8080",
		FrontendURL: "http://localhost:3000",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	h.setStateCookie(w, "google_oauth_state", "teststate")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	assert.Equal(t, "google_oauth_state", c.Name)
	assert.Equal(t, "teststate", c.Value)
	assert.False(t, c.Secure, "Secure must be false when BASE_URL is http")
	assert.True(t, c.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, 600, c.MaxAge)
}

func TestSetStateCookie_secureWhenHTTPS(t *testing.T) {
	h, err := NewOAuthHandler(zerolog.Nop(), nil, OAuthConfig{
		BaseURL:     "https://api.example.com",
		FrontendURL: "https://app.example.com",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	h.setStateCookie(w, "google_oauth_state", "securestate")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	assert.True(t, c.Secure, "Secure must be true when BASE_URL is https")
	assert.True(t, c.HttpOnly)
}
