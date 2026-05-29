// Package auth provides JWT generation (ES256) and validation for the
// agregator platform.
//
// # Key separation
//
// Only auth-service holds the ECDSA private key and can issue tokens.
// All other services (api-gateway, etc.) receive only the public key and
// can verify signatures but cannot forge them.
//
// Previously HS256 was used with a shared secret. HS256 is symmetric: any
// service that can verify a token can also sign one, so a compromised
// api-gateway could mint arbitrary tokens. ES256 (ECDSA P-256) is
// asymmetric: the private key never leaves auth-service.
//
// # Configuration
//
//	auth-service:   JWT_EC_PRIVATE_KEY  — PEM-encoded EC private key (PKCS8 or SEC1)
//	api-gateway:    JWT_EC_PUBLIC_KEY   — PEM-encoded EC public key  (PKIX)
//
// Generate a key pair (run once, store in a secrets manager):
//
//	openssl ecparam -name prime256v1 -genkey -noout | \
//	    openssl pkcs8 -topk8 -nocrypt -out ec_private.pem
//	openssl ec -in ec_private.pem -pubout -out ec_public.pem
package auth

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// Claims carries the standard JWT fields used across the platform.
type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Exp    int64  `json:"exp"`
	Iat    int64  `json:"iat"`
	Nbf    int64  `json:"nbf"` // not-before: token is invalid before this Unix timestamp
}

// GenerateAccessToken signs a JWT with ES256 using the given ECDSA private key.
// Only auth-service should call this; all other services verify with the
// corresponding public key via ValidateAccessToken.
func GenerateAccessToken(privateKey *ecdsa.PrivateKey, userID, email, role string, ttl time.Duration) (string, error) {
	now := time.Now().Unix()
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		Exp:    time.Now().Add(ttl).Unix(),
		Iat:    now,
		Nbf:    now,
	}
	return signES256(privateKey, claims)
}

// ValidateAccessToken verifies the ES256 signature and expiry of a JWT using
// the given ECDSA public key. Returns parsed claims or a sentinel error.
func ValidateAccessToken(publicKey *ecdsa.PublicKey, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Verify header declares ES256.
	// Use subtle.ConstantTimeCompare for the alg field to prevent alg-confusion
	// attacks (e.g. "none", "HS256") from being distinguishable via timing.
	// Without constant-time comparison an attacker could craft tokens with
	// different alg values and observe response-time differences to infer
	// whether the check short-circuited on the first differing byte.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return nil, ErrInvalidToken
	}
	if subtle.ConstantTimeCompare([]byte(hdr.Alg), []byte("ES256")) != 1 {
		return nil, ErrInvalidToken
	}

	// Decode signature before payload (fail fast on bad sig).
	// ecdsa.Verify itself does not require constant-time wrapping: it returns a
	// boolean whose value does not leak secret material — the private key is
	// never present on the verification path.  hmac.Equal is used here only to
	// future-proof the call site against copy-paste of this pattern into an HMAC
	// context; for ECDSA the plain boolean suffices and is what the stdlib uses.
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !verifyES256(publicKey, parts[0]+"."+parts[1], sigBytes) {
		return nil, ErrInvalidToken
	}

	// Decode claims.
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	now := time.Now().Unix()
	if now > claims.Exp {
		return nil, ErrExpiredToken
	}
	// nbf=0 means the field was absent (old tokens or tests) — skip check.
	if claims.Nbf != 0 && now < claims.Nbf {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

// ParseECPrivateKey decodes a PEM-encoded ECDSA private key (PKCS8 or SEC1).
// auth-service calls this at startup to load JWT_EC_PRIVATE_KEY.
func ParseECPrivateKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("auth: no PEM block found in private key")
	}
	switch block.Type {
	case "PRIVATE KEY": // PKCS8
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parse PKCS8 private key: %w", err)
		}
		ec, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("auth: PKCS8 key is not ECDSA (got %T)", key)
		}
		return ec, nil
	case "EC PRIVATE KEY": // SEC1 / legacy openssl format
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parse SEC1 EC private key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("auth: unexpected PEM block type %q (want PRIVATE KEY or EC PRIVATE KEY)", block.Type)
	}
}

// ParseECPublicKey decodes a PEM-encoded ECDSA public key (PKIX SubjectPublicKeyInfo).
// api-gateway calls this at startup to load JWT_EC_PUBLIC_KEY.
func ParseECPublicKey(pemBytes []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("auth: no PEM block found in public key")
	}
	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("auth: unexpected PEM block type %q (want PUBLIC KEY)", block.Type)
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("auth: parse PKIX public key: %w", err)
	}
	ec, ok := key.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("auth: public key is not ECDSA (got %T)", key)
	}
	return ec, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

func signES256(key *ecdsa.PrivateKey, claims Claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256","typ":"JWT"}`))

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payload

	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("ecdsa sign: %w", err)
	}

	sig := encodeRS(r, s)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func verifyES256(key *ecdsa.PublicKey, signingInput string, sigBytes []byte) bool {
	// ES256 signature for P-256 is a 64-byte fixed-width r||s encoding
	// (IEEE P1363), where r and s are each exactly 32 bytes (zero-padded).
	if len(sigBytes) != 64 {
		return false
	}
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])
	digest := sha256.Sum256([]byte(signingInput))
	if !ecdsa.Verify(key, digest[:], r, s) {
		return false
	}
	// Re-encode the verified (r, s) pair and compare against the input bytes
	// with hmac.Equal (constant-time) to prevent timing side-channels on the
	// byte-level comparison.  ecdsa.Verify itself is not timing-sensitive for
	// the secret (no private key on this path), but constant-time comparison
	// here is defence-in-depth and prevents any future refactor from
	// accidentally introducing a variable-time byte comparison.
	expected := encodeRS(r, s)
	return hmac.Equal(expected, sigBytes)
}

// encodeRS produces the 64-byte IEEE P1363 encoding of an ECDSA signature:
// r and s each zero-padded to 32 bytes (the byte-length of P-256's order).
func encodeRS(r, s *big.Int) []byte {
	out := make([]byte, 64)
	rb := r.Bytes()
	sb := s.Bytes()
	// right-align within each 32-byte half
	copy(out[32-len(rb):32], rb)
	copy(out[64-len(sb):64], sb)
	return out
}
