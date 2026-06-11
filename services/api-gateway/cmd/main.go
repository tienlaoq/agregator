package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/pkg/logger"
	pkgmetrics "github.com/tienlao/agregator/pkg/metrics"
	gwmetrics "github.com/tienlao/agregator/services/api-gateway/internal/metrics"
	"github.com/tienlao/agregator/services/api-gateway/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	log := logger.New("api-gateway")
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// ── Telemetry ──────────────────────────────────────────────────────────
	otelShutdown, err := telemetry.Init(rootCtx, log, "api-gateway")
	if err != nil {
		log.Fatal().Err(err).Msg("OpenTelemetry init failed")
	}

	// ── Config ─────────────────────────────────────────────────────────────
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("configuration error")
	}
	if os.Getenv("JWT_SECRET") != "" {
		log.Warn().Msg("JWT_SECRET is set but no longer used — api-gateway now uses ES256 (JWT_EC_PUBLIC_KEY). Remove JWT_SECRET from your environment.")
	}
	logConfig(log, cfg)

	// ── Dependencies ───────────────────────────────────────────────────────
	d, cleanup, err := buildDeps(rootCtx, log, cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("dependency initialisation failed")
	}
	defer cleanup()

	// ── Router ─────────────────────────────────────────────────────────────
	r, err := buildRouter(rootCtx, log, cfg, d)
	if err != nil {
		log.Fatal().Err(err).Msg("router build failed")
	}

	// ── Internal observability listener (never published on the host) ─────
	go func() {
		addr := pkgmetrics.AddrFromEnv()
		log.Info().Str("addr", addr).Msg("metrics listener starting")
		if err := pkgmetrics.ServeRegistry(rootCtx, addr, gwmetrics.Registry(), nil); err != nil {
			log.Error().Err(err).Msg("metrics listener failed")
		}
	}()

	// ── HTTP server ────────────────────────────────────────────────────────
	var httpHandler http.Handler = r
	if strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" {
		httpHandler = otelhttp.NewHandler(r, "api-gateway",
			// WebSocket-апгрейды идут мимо otelhttp: его ResponseWriter не
			// реализует http.Hijacker, из-за чего gorilla возвращает 500 на
			// каждый /ws. Спан на долгоживущий сокет всё равно бесполезен.
			otelhttp.WithFilter(func(req *http.Request) bool {
				return !strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
			}),
		)
	}

	srv := serverTimeouts(":"+cfg.HTTPPort, httpHandler)

	go func() {
		log.Info().Str("port", cfg.HTTPPort).Msg("starting api-gateway")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	// ── Graceful shutdown ──────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}

	otelCtx, otelCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer otelCancel()
	if err := otelShutdown(otelCtx); err != nil {
		log.Error().Err(err).Msg("OpenTelemetry shutdown")
	}

	log.Info().Msg("server stopped")
}

// logConfig emits the effective configuration at INFO level at startup so
// operators can confirm what was read without digging into env dumps.
func logConfig(log zerolog.Logger, cfg Config) {
	storage := "disk:" + cfg.UploadRoot
	if cfg.MinIOEndpoint != "" {
		storage = "minio:" + cfg.MinIOEndpoint + "/" + cfg.MinIOBucket
	}
	log.Info().
		Str("http_port", cfg.HTTPPort).
		Str("storage", storage).
		Str("base_url", cfg.BaseURL).
		Str("frontend_url", cfg.FrontendURL).
		Str("auth_addr", cfg.AuthAddr).
		Str("user_addr", cfg.UserAddr).
		Str("venue_addr", cfg.VenueAddr).
		Str("booking_addr", cfg.BookingAddr).
		Str("review_addr", cfg.ReviewAddr).
		Str("payment_addr", cfg.PaymentAddr).
		Str("master_addr", cfg.MasterAddr).
		Str("chat_addr", cfg.ChatAddr).
		Str("crm_addr", cfg.CRMAddr).
		Str("notification_addr", cfg.NotificationAddr).
		Bool("redis", cfg.RedisAddr != "").
		Bool("nats", cfg.NATSUrl != "").
		Bool("vk_oauth", cfg.VKClientID != "").
		Bool("yandex_oauth", cfg.YandexClientID != "").
		Msg("config loaded")
}
