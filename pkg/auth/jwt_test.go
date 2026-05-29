package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKeyPair generates a fresh P-256 key pair for each test run.
// It uses a helper so individual tests do not need to repeat the setup.
func testKeyPair(t *testing.T) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return priv, &priv.PublicKey
}

func TestGenerateAndValidate_RoundTrip(t *testing.T) {
	priv, pub := testKeyPair(t)

	tok, err := GenerateAccessToken(priv, "user-123", "test@example.com", "admin", 15*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)

	claims, err := ValidateAccessToken(pub, tok)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "admin", claims.Role)
	assert.True(t, claims.Exp > time.Now().Unix())
	assert.True(t, claims.Iat <= time.Now().Unix())
}

func TestGenerateAndValidate_NbfSet(t *testing.T) {
	priv, pub := testKeyPair(t)

	tok, err := GenerateAccessToken(priv, "uid", "e@x.com", "user", time.Hour)
	require.NoError(t, err)

	claims, err := ValidateAccessToken(pub, tok)
	require.NoError(t, err)
	assert.True(t, claims.Nbf > 0, "nbf should be set")
	assert.True(t, claims.Nbf <= time.Now().Unix(), "nbf should be <= now")
}

func TestValidateAccessToken_NotYetValid(t *testing.T) {
	priv, pub := testKeyPair(t)

	// Manually craft a token where nbf is in the future.
	now := time.Now().Unix()
	claims := Claims{
		UserID: "uid",
		Email:  "e@x.com",
		Role:   "user",
		Exp:    now + 3600,
		Iat:    now,
		Nbf:    now + 3600, // valid only in the future
	}
	tok, err := signES256(priv, claims)
	require.NoError(t, err)

	_, err = ValidateAccessToken(pub, tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateAccessToken_Expired(t *testing.T) {
	priv, pub := testKeyPair(t)

	tok, err := GenerateAccessToken(priv, "uid", "e@x.com", "user", -time.Second)
	require.NoError(t, err)

	_, err = ValidateAccessToken(pub, tok)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

func TestValidateAccessToken_WrongKey(t *testing.T) {
	priv, _ := testKeyPair(t)
	_, wrongPub := testKeyPair(t) // different key pair

	tok, err := GenerateAccessToken(priv, "uid", "e@x.com", "user", time.Hour)
	require.NoError(t, err)

	_, err = ValidateAccessToken(wrongPub, tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateAccessToken_Malformed(t *testing.T) {
	_, pub := testKeyPair(t)

	cases := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"single part", "abc"},
		{"two parts", "abc.def"},
		{"four parts", "a.b.c.d"},
		{"invalid base64 header", "!!!.e30.sig"},
		{"invalid base64 payload", "eyJhbGciOiJFUzI1NiJ9.!!!.sig"},
		// alg-confusion attacks — any algorithm other than ES256 must be rejected
		{"wrong alg HS256", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.sig"},
		{"wrong alg none", "eyJhbGciOiJub25lIn0.e30."},   // {"alg":"none"}
		{"wrong alg ES384", "eyJhbGciOiJFUzM4NCJ9.e30.sig"}, // {"alg":"ES384"}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateAccessToken(pub, tc.token)
			assert.ErrorIs(t, err, ErrInvalidToken, "token: %q", tc.token)
		})
	}
}

func TestValidateAccessToken_TamperedPayload(t *testing.T) {
	priv, pub := testKeyPair(t)

	tok, err := GenerateAccessToken(priv, "uid", "e@x.com", "user", time.Hour)
	require.NoError(t, err)

	// Replace payload with a different one (admin role) while keeping signature.
	parts := splitThree(tok)
	require.Len(t, parts, 3)
	import_b64 := "eyJzdWIiOiJ1aWQiLCJlbWFpbCI6ImVAeC5jb20iLCJyb2xlIjoiYWRtaW4iLCJleHAiOjk5OTk5OTk5OTksImlhdCI6MH0"
	tampered := parts[0] + "." + import_b64 + "." + parts[2]

	_, err = ValidateAccessToken(pub, tampered)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// ── PEM parsing tests ─────────────────────────────────────────────────────────

func TestParseECPrivateKey_PKCS8(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	parsed, err := ParseECPrivateKey(pemBytes)
	require.NoError(t, err)
	assert.Equal(t, priv.D, parsed.D)
}

func TestParseECPrivateKey_SEC1(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})

	parsed, err := ParseECPrivateKey(pemBytes)
	require.NoError(t, err)
	assert.Equal(t, priv.D, parsed.D)
}

func TestParseECPublicKey_PKIX(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	parsed, err := ParseECPublicKey(pemBytes)
	require.NoError(t, err)
	assert.Equal(t, priv.PublicKey.X, parsed.X)
	assert.Equal(t, priv.PublicKey.Y, parsed.Y)
}

func TestParseECPrivateKey_Empty(t *testing.T) {
	_, err := ParseECPrivateKey([]byte("not a pem"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no PEM block")
}

func TestParseECPublicKey_WrongType(t *testing.T) {
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("data")})
	_, err := ParseECPublicKey(pemBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected PEM block type")
}

// TestSignVerify_MultipleTokensUnique checks that two tokens for the same
// claims differ (ES256 is randomised unlike HS256).
func TestSignVerify_MultipleTokensUnique(t *testing.T) {
	priv, pub := testKeyPair(t)

	tok1, err := GenerateAccessToken(priv, "u1", "e@x.com", "user", time.Hour)
	require.NoError(t, err)
	tok2, err := GenerateAccessToken(priv, "u1", "e@x.com", "user", time.Hour)
	require.NoError(t, err)

	// Tokens differ because ECDSA signing uses a random nonce.
	assert.NotEqual(t, tok1, tok2)

	// But both validate correctly.
	c1, err := ValidateAccessToken(pub, tok1)
	require.NoError(t, err)
	c2, err := ValidateAccessToken(pub, tok2)
	require.NoError(t, err)
	assert.Equal(t, c1.UserID, c2.UserID)
}

// splitThree splits a JWT string into its three dot-separated parts.
func splitThree(tok string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(tok); i++ {
		if tok[i] == '.' {
			parts = append(parts, tok[start:i])
			start = i + 1
		}
	}
	parts = append(parts, tok[start:])
	return parts
}
