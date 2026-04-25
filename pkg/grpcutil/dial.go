package grpcutil

import (
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// DefaultRetryServiceConfig enables limited unary retries on transient failures.
// See https://github.com/grpc/grpc/blob/master/doc/service_config.md
func init() {
	// Unary retries from service config require this in grpc-go.
	if os.Getenv("GRPC_GO_RETRY") == "" {
		_ = os.Setenv("GRPC_GO_RETRY", "on")
	}
}

const defaultRetryServiceConfig = `{
  "methodConfig": [{
    "name": [{}],
    "retryPolicy": {
      "maxAttempts": 4,
      "initialBackoff": "0.1s",
      "maxBackoff": "2s",
      "backoffMultiplier": 2,
      "retryableStatusCodes": ["UNAVAILABLE"]
    }
  }]
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
func InsecureDialOptions() []grpc.DialOption {
	out := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	out = append(out, ClientDialOptions()...)
	return out
}
