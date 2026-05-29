package grpcutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

func init() {
	// Unary retries from service config require this env var in grpc-go.
	if os.Getenv("GRPC_GO_RETRY") == "" {
		_ = os.Setenv("GRPC_GO_RETRY", "on")
	}
}

// defaultRetryServiceConfig enables limited unary retries on UNAVAILABLE for
// all methods. grpc service config requires {"service": "pkg.Svc", "method": "M"}
// or {} (catch-all) — method-only entries without a service are invalid in
// grpc-go and cause dial to fail. We use a single catch-all with 4 attempts;
// idempotency is enforced at the application level (CreateBooking, SendMessage
// use client-generated IDs for deduplication).
//
// See https://github.com/grpc/grpc/blob/master/doc/service_config.md
const defaultRetryServiceConfig = `{
  "methodConfig": [
    {
      "name": [{}],
      "retryPolicy": {
        "maxAttempts": 4,
        "initialBackoff": "0.1s",
        "maxBackoff": "2s",
        "backoffMultiplier": 2,
        "retryableStatusCodes": ["UNAVAILABLE"]
      }
    }
  ]
}`

// ClientDialOptions returns production-oriented gRPC dial defaults (keepalive + retry).
// Pass grpc.WithTransportCredentials and service-specific interceptors before or after as needed.
func ClientDialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(defaultRetryServiceConfig),
	}
}

// InsecureDialOptions is grpc.WithTransportCredentials(insecure) plus ClientDialOptions.
// Deprecated: use DialOptions which respects GRPC_TLS env. Kept for internal use.
func InsecureDialOptions() []grpc.DialOption {
	out := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	out = append(out, ClientDialOptions()...)
	return out
}

// TLSDialOptions returns dial options with TLS enabled.
//
// If GRPC_TLS_CA is set, the PEM file at that path is appended to the system
// CA pool — useful for internal CAs (cert-manager, step-ca, etc.).
// Otherwise the system roots are used (works with Let's Encrypt / public CAs).
func TLSDialOptions() ([]grpc.DialOption, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if caFile := strings.TrimSpace(os.Getenv("GRPC_TLS_CA")); caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("grpcutil: read GRPC_TLS_CA %q: %w", caFile, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("grpcutil: no valid certificates in GRPC_TLS_CA %q", caFile)
		}
		tlsCfg.RootCAs = pool
	}

	creds := credentials.NewTLS(tlsCfg)
	out := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	out = append(out, ClientDialOptions()...)
	return out, nil
}

// DialOptions returns TLS or insecure dial options based on env:
//
//   - GRPC_TLS=true  → TLS (reads optional GRPC_TLS_CA for custom CA bundle)
//   - GRPC_TLS=false (default) → plaintext insecure
//
// In production (ENV=production) plaintext is a fatal misconfiguration — the
// caller should treat the returned error as fatal.
//
// Typical usage:
//
//	dialOpts, err := grpcutil.DialOptions()
//	if err != nil { log.Fatal()... }
func DialOptions() ([]grpc.DialOption, error) {
	grpcTLS := strings.EqualFold(strings.TrimSpace(os.Getenv("GRPC_TLS")), "true")
	isProduction := strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")

	if !grpcTLS {
		if isProduction {
			return nil, fmt.Errorf("grpcutil: GRPC_TLS=true is required in production (ENV=production) — " +
				"running gRPC in plaintext exposes auth tokens and payment data")
		}
		return InsecureDialOptions(), nil
	}
	return TLSDialOptions()
}
