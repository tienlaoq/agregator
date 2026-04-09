package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-key-for-unit-tests"

func TestGenerateAccessToken(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, "user-123", "test@example.com", "admin", 15*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := ValidateAccessToken(testSecret, token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.Equal(t, "admin", claims.Role)
	assert.True(t, claims.Exp > time.Now().Unix())
	assert.True(t, claims.Iat <= time.Now().Unix())
}

func TestValidateAccessToken_Valid(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, "uid-456", "u@x.com", "user", time.Hour)
	require.NoError(t, err)

	claims, err := ValidateAccessToken(testSecret, token)
	require.NoError(t, err)
	assert.Equal(t, "uid-456", claims.UserID)
	assert.Equal(t, "u@x.com", claims.Email)
	assert.Equal(t, "user", claims.Role)
}

func TestValidateAccessToken_Expired(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, "uid", "e@x.com", "user", -1*time.Second)
	require.NoError(t, err)

	_, err = ValidateAccessToken(testSecret, token)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

func TestValidateAccessToken_InvalidSignature(t *testing.T) {
	token, err := GenerateAccessToken(testSecret, "uid", "e@x.com", "user", time.Hour)
	require.NoError(t, err)

	_, err = ValidateAccessToken("wrong-secret", token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateAccessToken_Malformed(t *testing.T) {
	testCases := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"single part", "abc"},
		{"two parts", "abc.def"},
		{"four parts", "a.b.c.d"},
		{"invalid base64 payload", "eyJhbGciOiJIUzI1NiJ9.!!!invalid!!!.sig"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateAccessToken(testSecret, tc.token)
			assert.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}
