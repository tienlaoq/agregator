package grpcutil

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

const defaultMaxMsgSize = 16 << 20 // 16 MiB

// ServerOptions returns keepalive and message size limits for internal gRPC servers.
func ServerOptions() []grpc.ServerOption {
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
