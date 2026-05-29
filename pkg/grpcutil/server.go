package grpcutil

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

const defaultMaxMsgSize = 16 << 20 // 16 MiB

// baseServerOptions returns keepalive and message size limits for internal gRPC servers.
func baseServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.MaxRecvMsgSize(defaultMaxMsgSize),
		grpc.MaxSendMsgSize(defaultMaxMsgSize),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 15 * time.Minute,
			Time:              5 * time.Minute,
			Timeout:           20 * time.Second,
		}),
	}
}

// ServerOptions returns keepalive and message size limits for internal gRPC servers.
// Deprecated: use ServerOptionsFromEnv which respects GRPC_TLS. Kept for backward compat.
func ServerOptions() []grpc.ServerOption {
	return baseServerOptions()
}

// TLSServerOptions returns server options with TLS enabled using the given
// certificate and key PEM files.
//
// certFile — path to the server's TLS certificate (PEM, may include chain).
// keyFile  — path to the corresponding private key (PEM).
func TLSServerOptions(certFile, keyFile string) ([]grpc.ServerOption, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("grpcutil: load TLS keypair (cert=%q key=%q): %w", certFile, keyFile, err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	opts := []grpc.ServerOption{grpc.Creds(credentials.NewTLS(tlsCfg))}
	opts = append(opts, baseServerOptions()...)
	return opts, nil
}

// ServerOptionsFromEnv returns TLS or plaintext server options based on env:
//
//   - GRPC_TLS=true  → TLS; reads GRPC_TLS_CERT and GRPC_TLS_KEY (both required)
//   - GRPC_TLS=false (default) → plaintext
//
// In production (ENV=production) plaintext is a fatal misconfiguration — the
// caller should treat the returned error as fatal.
//
// Typical usage:
//
//	srvOpts, err := grpcutil.ServerOptionsFromEnv()
//	if err != nil { log.Fatal()... }
func ServerOptionsFromEnv() ([]grpc.ServerOption, error) {
	grpcTLS := strings.EqualFold(strings.TrimSpace(os.Getenv("GRPC_TLS")), "true")
	isProduction := strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")

	if !grpcTLS {
		if isProduction {
			return nil, fmt.Errorf("grpcutil: GRPC_TLS=true is required in production (ENV=production) — " +
				"running gRPC in plaintext exposes auth tokens and payment data")
		}
		return baseServerOptions(), nil
	}

	certFile := strings.TrimSpace(os.Getenv("GRPC_TLS_CERT"))
	keyFile := strings.TrimSpace(os.Getenv("GRPC_TLS_KEY"))
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("grpcutil: GRPC_TLS=true but GRPC_TLS_CERT or GRPC_TLS_KEY is not set")
	}
	return TLSServerOptions(certFile, keyFile)
}
