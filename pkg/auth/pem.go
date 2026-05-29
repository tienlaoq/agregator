package auth

import (
	"os"
	"strings"
)

// LoadPEMFromEnv tries fileEnvVar first (path to a PEM file on disk),
// then inlineEnvVar (raw PEM content). Container runtimes often escape
// newlines as \n in env vars; those are unescaped automatically.
// Returns nil if neither variable is set or the file cannot be read.
//
// Usage:
//
//	// auth-service (private key)
//	pem := auth.LoadPEMFromEnv("JWT_EC_PRIVATE_KEY_FILE", "JWT_EC_PRIVATE_KEY")
//
//	// api-gateway (public key)
//	pem := auth.LoadPEMFromEnv("JWT_EC_PUBLIC_KEY_FILE", "JWT_EC_PUBLIC_KEY")
func LoadPEMFromEnv(fileEnvVar, inlineEnvVar string) []byte {
	if path := strings.TrimSpace(os.Getenv(fileEnvVar)); path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
	}
	if raw := strings.TrimSpace(os.Getenv(inlineEnvVar)); raw != "" {
		// Container runtimes often store multiline secrets with literal \n.
		return []byte(strings.ReplaceAll(raw, `\n`, "\n"))
	}
	return nil
}
