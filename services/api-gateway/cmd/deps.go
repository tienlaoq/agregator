package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	notificationv1 "github.com/tienlao/agregator/gen/go/notification/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/config"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/natsutil"
	"github.com/tienlao/agregator/pkg/storage"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/ratelimit"
	"github.com/tienlao/agregator/services/api-gateway/internal/supportstore"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// deps holds every live connection and client the gateway needs. All fields
// are optional where the underlying infrastructure is optional (Redis, NATS,
// Postgres). buildDeps is the single place that opens sockets; the returned
// cleanup func closes them in reverse order.
type deps struct {
	// gRPC clients
	Auth         authv1.AuthServiceClient
	User         userv1.UserServiceClient
	Venue        venuev1.VenueServiceClient
	Booking      bookingv1.BookingServiceClient
	Review       reviewv1.ReviewServiceClient
	Payment      paymentv1.PaymentServiceClient
	Master       masterv1.MasterServiceClient
	Chat         chatv1.ChatServiceClient
	CRM          crmv1.CRMServiceClient
	Notification notificationv1.NotificationServiceClient

	// File storage for user-uploaded photos (venues, masters).
	// MinioUploader when MINIO_ENDPOINT is set; DiskUploader otherwise.
	Storage storage.Uploader

	// Optional infrastructure
	Redis       *redis.Client       // nil when REDIS_ADDR is unset
	RateLimiter ratelimit.Limiter   // in-memory fallback when Redis is nil
	NATS        *nats.Conn          // nil when NATS_URL is unset
	TicketStore *supportstore.Store // nil when Postgres is unconfigured
}

// buildDeps opens all connections described by cfg. It returns the deps bundle
// and a cleanup func that must be called (typically via defer) to close every
// connection that was successfully opened. Connections that succeed before a
// later failure are still closed by cleanup, so there are no leaks on partial
// initialisation.
func buildDeps(ctx context.Context, log zerolog.Logger, cfg Config) (*deps, func(), error) {
	var closers []func()
	cleanup := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	// ── gRPC dial options ──────────────────────────────────────────────────
	baseOpts, err := grpcutil.DialOptions()
	if err != nil {
		return nil, cleanup, fmt.Errorf("gRPC transport config: %w", err)
	}
	baseOpts = append(baseOpts, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))

	// mustDial opens one gRPC connection and registers its Close in the
	// cleanup stack. On error the caller returns immediately; cleanup still
	// closes any previously opened connections.
	mustDial := func(addr, name string, extra ...grpc.DialOption) (*grpc.ClientConn, error) {
		opts := append(append([]grpc.DialOption{}, baseOpts...), extra...)
		conn, err := grpc.NewClient(addr, opts...)
		if err != nil {
			return nil, fmt.Errorf("dial %s (%s): %w", name, addr, err)
		}
		closers = append(closers, func() { conn.Close() })
		log.Info().Str("service", name).Str("addr", addr).Msg("gRPC client created")
		return conn, nil
	}

	authConn, err := mustDial(cfg.AuthAddr, "auth-service")
	if err != nil {
		return nil, cleanup, err
	}
	userConn, err := mustDial(cfg.UserAddr, "user-service")
	if err != nil {
		return nil, cleanup, err
	}
	venueConn, err := mustDial(cfg.VenueAddr, "venue-service")
	if err != nil {
		return nil, cleanup, err
	}
	// CallerIDClientInterceptor propagates the gateway-verified user_id (from
	// JWT claims stored in HTTP context) into gRPC metadata as "x-caller-id".
	// booking-service reads it via CallerIDServerInterceptor + CallerIDFromCtx
	// for authz, instead of trusting proto payload fields (owner_id, requester_user_id).
	bookingConn, err := mustDial(cfg.BookingAddr, "booking-service",
		grpc.WithUnaryInterceptor(grpcutil.CallerIDClientInterceptor(middleware.UserIDFromCtx)),
	)
	if err != nil {
		return nil, cleanup, err
	}
	reviewConn, err := mustDial(cfg.ReviewAddr, "review-service")
	if err != nil {
		return nil, cleanup, err
	}
	// payment-service and chat-service both enforce ServiceTokenServerInterceptor:
	// every call must carry x-service-token or it is rejected with Unauthenticated.
	// Validate the token once here so a missing value surfaces at startup with a
	// clear message instead of as a stream of Unauthenticated errors at runtime.
	if cfg.InternalServiceToken == "" {
		if isProductionEnv(cfg.ChatAddr) {
			return nil, cleanup, fmt.Errorf(
				"INTERNAL_SERVICE_TOKEN must be set in production " +
					"(required by payment-service and chat-service)",
			)
		}
		log.Warn().Msg("INTERNAL_SERVICE_TOKEN not set — service-to-service auth to payment-service and chat-service disabled (dev/CI only)")
	}

	paymentConn, err := mustDial(cfg.PaymentAddr, "payment-service",
		grpc.WithUnaryInterceptor(grpcutil.ServiceTokenClientInterceptor(cfg.InternalServiceToken)),
	)
	if err != nil {
		return nil, cleanup, err
	}
	// CallerIDClientInterceptor propagates the gateway-verified user_id (from
	// JWT claims stored in HTTP context) into gRPC metadata as "x-caller-id".
	// master-service reads it via CallerIDServerInterceptor + CallerIDFromCtx
	// instead of trusting the user_id proto field in the request payload.
	masterConn, err := mustDial(cfg.MasterAddr, "master-service",
		grpc.WithUnaryInterceptor(grpcutil.CallerIDClientInterceptor(middleware.UserIDFromCtx)),
	)
	if err != nil {
		return nil, cleanup, err
	}

	chatConn, err := mustDial(cfg.ChatAddr, "chat-service",
		grpc.WithUnaryInterceptor(grpcutil.ServiceTokenClientInterceptor(cfg.InternalServiceToken)),
	)
	if err != nil {
		return nil, cleanup, err
	}

	crmConn, err := mustDial(cfg.CRMAddr, "crm-service")
	if err != nil {
		return nil, cleanup, err
	}

	// notification-service enforces ServiceTokenServerInterceptor (like chat /
	// payment), so the gateway must present x-service-token on every call.
	notificationConn, err := mustDial(cfg.NotificationAddr, "notification-service",
		grpc.WithUnaryInterceptor(grpcutil.ServiceTokenClientInterceptor(cfg.InternalServiceToken)),
	)
	if err != nil {
		return nil, cleanup, err
	}

	d := &deps{
		Auth:         authv1.NewAuthServiceClient(authConn),
		User:         userv1.NewUserServiceClient(userConn),
		Venue:        venuev1.NewVenueServiceClient(venueConn),
		Booking:      bookingv1.NewBookingServiceClient(bookingConn),
		Review:       reviewv1.NewReviewServiceClient(reviewConn),
		Payment:      paymentv1.NewPaymentServiceClient(paymentConn),
		Master:       masterv1.NewMasterServiceClient(masterConn),
		Chat:         chatv1.NewChatServiceClient(chatConn),
		CRM:          crmv1.NewCRMServiceClient(crmConn),
		Notification: notificationv1.NewNotificationServiceClient(notificationConn),
	}

	// ── File storage ──────────────────────────────────────────────────────
	if cfg.MinIOEndpoint != "" {
		minioUploader, err := storage.NewMinioUploader(storage.MinioConfig{
			Endpoint:      cfg.MinIOEndpoint,
			AccessKey:     cfg.MinIOAccessKey,
			SecretKey:     cfg.MinIOSecretKey,
			Bucket:        cfg.MinIOBucket,
			PublicBaseURL: cfg.MinIOPublicBaseURL,
			UseSSL:        cfg.MinIOUseSSL,
		})
		if err != nil {
			return nil, cleanup, fmt.Errorf("storage: minio init: %w", err)
		}
		d.Storage = minioUploader
		log.Info().
			Str("endpoint", cfg.MinIOEndpoint).
			Str("bucket", cfg.MinIOBucket).
			Msg("storage: MinIO uploader active")
	} else {
		// ── Production gate: object storage required ─────────────────────────
		// DiskUploader writes to the container's local filesystem, which does
		// not survive restarts and is not shared across replicas — uploaded
		// photos would be lost or unreachable behind a load balancer. Fail fast
		// in production rather than silently serving a fallback that drops
		// files. See TECH_DEBT [STORAGE-01].
		if isProductionEnv(cfg.ChatAddr) {
			return nil, cleanup, fmt.Errorf(
				"MINIO_ENDPOINT is required in production: the local-disk uploader " +
					"is single-replica and loses files on container restart; set " +
					"MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY and MINIO_BUCKET",
			)
		}
		diskUploader, err := storage.NewDiskUploader(cfg.UploadRoot, "/api/v1/uploads")
		if err != nil {
			return nil, cleanup, fmt.Errorf("storage: disk init: %w", err)
		}
		d.Storage = diskUploader
		log.Warn().
			Str("root", cfg.UploadRoot).
			Msg("storage: MINIO_ENDPOINT not set — using local disk (single-replica only, see TECH_DEBT [STORAGE-01])")
	}

	// ── Redis (optional) ───────────────────────────────────────────────────
	if cfg.RedisAddr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Warn().Err(err).Str("redis_addr", cfg.RedisAddr).Msg("redis unavailable, fallback to in-memory rate limit")
			_ = rdb.Close()
		} else {
			d.Redis = rdb
			d.RateLimiter = ratelimit.NewRedisLimiter(rdb)
			closers = append(closers, func() { rdb.Close() })
			log.Info().Str("redis_addr", cfg.RedisAddr).Msg("Redis connected")
		}
	}

	// ── Production gate: Redis required for distributed rate limiting ─────
	// Without Redis each gateway replica maintains its own per-user counter,
	// so the effective send-rate limit becomes sendRateMax × replica-count.
	// Fail fast in production rather than silently allow excess traffic.
	if d.Redis == nil && isProductionEnv(cfg.ChatAddr) {
		return nil, cleanup, fmt.Errorf(
			"REDIS_ADDR is required in production: " +
				"without Redis the chat rate limiter is per-replica " +
				"(effective limit = sendRateMax × replica count); set REDIS_ADDR",
		)
	}

	// ── NATS (optional) ────────────────────────────────────────────────────
	if cfg.NATSUrl != "" {
		nc, err := nats.Connect(cfg.NATSUrl)
		if err != nil {
			// NATS failure is fatal: analytics and chat fanout depend on it.
			return nil, cleanup, fmt.Errorf("NATS connect %q: %w", cfg.NATSUrl, err)
		}
		d.NATS = nc
		closers = append(closers, nc.Close)
		log.Info().Str("nats_url", cfg.NATSUrl).Msg("NATS connected")

		// ANALYTICS captures core-NATS publishes of analytics.web into
		// JetStream: без стрима события живут только пока их кто-то слушает.
		// MaxAge 30d — стрим лишь буфер, архив строит analytics-service в
		// Postgres. Не фатально: гейтвей работает и без аналитики.
		if js, jsErr := nc.JetStream(); jsErr != nil {
			log.Warn().Err(jsErr).Msg("jetstream unavailable — analytics.web events will not be persisted")
		} else if err := natsutil.EnsureStreamMaxAge(js, "ANALYTICS", []string{"analytics.>"}, 30*24*time.Hour); err != nil {
			log.Warn().Err(err).Msg("ensure ANALYTICS stream failed — analytics.web events will not be persisted")
		}
	}

	// ── Postgres for support tickets (optional) ────────────────────────────
	pgSupport := config.NewPostgresConfig("PG")
	pgSupport.DBName = cfg.SupportPGDBName
	if err := pgSupport.Validate(); err != nil {
		log.Warn().Err(err).Msg("support tickets Postgres disabled — set PG_* / migrate support_db for moderator inbox")
	} else {
		pool, err := pgxpool.New(ctx, pgSupport.DSN())
		if err != nil {
			log.Warn().Err(err).Msg("support tickets: postgres pool failed")
		} else if pingErr := pool.Ping(ctx); pingErr != nil {
			log.Warn().Err(pingErr).Msg("support tickets: postgres ping failed")
			pool.Close()
		} else {
			d.TicketStore = supportstore.New(pool)
			closers = append(closers, pool.Close)
			log.Info().Str("pg_db", pgSupport.DBName).Msg("support tickets persistence enabled")
		}
	}

	return d, cleanup, nil
}
