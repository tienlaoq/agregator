package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/config"
	"github.com/tienlao/agregator/pkg/grpcutil"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/services/api-gateway/internal/handler"
	gwmetrics "github.com/tienlao/agregator/services/api-gateway/internal/metrics"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/suspendnotify"
	"github.com/tienlao/agregator/services/api-gateway/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
)

func main() {
	log := logger.New("api-gateway")
	rootCtx := context.Background()

	otelShutdown, err := telemetry.Init(rootCtx, log, "api-gateway")
	if err != nil {
		log.Fatal().Err(err).Msg("OpenTelemetry init failed")
	}

	httpPort := config.GetEnv("HTTP_PORT", "8080")
	uploadRoot := config.GetEnv("UPLOAD_ROOT", "./data/uploads")
	if err := os.MkdirAll(filepath.Join(uploadRoot, "venues"), 0o755); err != nil {
		log.Fatal().Err(err).Msg("failed to create upload directory")
	}
	if err := os.MkdirAll(filepath.Join(uploadRoot, "masters"), 0o755); err != nil {
		log.Fatal().Err(err).Msg("failed to create master upload directory")
	}
	authAddr := config.GetEnv("AUTH_SERVICE_ADDR", "localhost:50051")
	userAddr := config.GetEnv("USER_SERVICE_ADDR", "localhost:50052")
	venueAddr := config.GetEnv("VENUE_SERVICE_ADDR", "localhost:50053")
	bookingAddr := config.GetEnv("BOOKING_SERVICE_ADDR", "localhost:50054")
	reviewAddr := config.GetEnv("REVIEW_SERVICE_ADDR", "localhost:50055")
	paymentAddr := config.GetEnv("PAYMENT_SERVICE_ADDR", "localhost:50056")
	masterAddr := config.GetEnv("MASTER_SERVICE_ADDR", "localhost:50057")

	dialOpts := grpcutil.InsecureDialOptions()
	dialOpts = append(dialOpts, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))

	authConn, err := grpc.NewClient(authAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to auth service")
	}
	defer authConn.Close()

	userConn, err := grpc.NewClient(userAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to user service")
	}
	defer userConn.Close()

	venueConn, err := grpc.NewClient(venueAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to venue service")
	}
	defer venueConn.Close()

	bookingConn, err := grpc.NewClient(bookingAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to booking service")
	}
	defer bookingConn.Close()

	reviewConn, err := grpc.NewClient(reviewAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to review service")
	}
	defer reviewConn.Close()

	paymentConn, err := grpc.NewClient(paymentAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to payment service")
	}
	defer paymentConn.Close()

	masterConn, err := grpc.NewClient(masterAddr, dialOpts...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to master service")
	}
	defer masterConn.Close()

	authClient := authv1.NewAuthServiceClient(authConn)
	userClient := userv1.NewUserServiceClient(userConn)
	venueClient := venuev1.NewVenueServiceClient(venueConn)
	bookingClient := bookingv1.NewBookingServiceClient(bookingConn)
	reviewClient := reviewv1.NewReviewServiceClient(reviewConn)
	paymentClient := paymentv1.NewPaymentServiceClient(paymentConn)
	masterClient := masterv1.NewMasterServiceClient(masterConn)

	baseURL := strings.TrimSpace(config.GetEnv("BASE_URL", "http://localhost:8080"))
	frontendURL := strings.TrimSpace(config.GetEnv("FRONTEND_URL", "http://localhost:3000"))

	authHandler := handler.NewAuthHandler(authClient)
	oauthHandler := handler.NewOAuthHandler(authClient, handler.OAuthConfig{
		GoogleClientID:     config.GetEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: config.GetEnv("GOOGLE_CLIENT_SECRET", ""),
		VKClientID:         config.GetEnv("VK_CLIENT_ID", ""),
		VKClientSecret:     config.GetEnv("VK_CLIENT_SECRET", ""),
		BaseURL:            baseURL,
		FrontendURL:        frontendURL,
	})
	userHandler := handler.NewUserHandler(userClient)
	suspendMail := suspendnotify.NewSender(log, venueClient, userClient)
	venueHandler := handler.NewVenueHandler(venueClient, userClient, uploadRoot, handler.WithSuspendNotifier(suspendMail))
	if suspendMail.Enabled() {
		log.Info().Msg("SMTP configured: при приостановке и возобновлении заведения владельцу и персоналу уйдут письма")
	}
	bookingHandler := handler.NewBookingHandler(bookingClient, venueClient)
	reviewHandler := handler.NewReviewHandler(reviewClient)
	paymentHandler := handler.NewPaymentHandler(paymentClient)
	masterHandler := handler.NewMasterHandler(masterClient, uploadRoot)

	var nc *nats.Conn
	if natsURL := strings.TrimSpace(config.GetEnv("NATS_URL", "")); natsURL != "" {
		var natsErr error
		nc, natsErr = nats.Connect(natsURL)
		if natsErr != nil {
			log.Fatal().Err(natsErr).Str("nats_url", natsURL).Msg("failed to connect to NATS")
		}
		defer nc.Close()
		log.Info().Str("nats_url", natsURL).Msg("NATS connected (analytics publish)")
	}
	analyticsHandler := handler.NewAnalyticsHandler(log, nc)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(gwmetrics.HTTPMiddleware)
	r.Use(middleware.Logging(log))
	r.Use(middleware.SecurityHeaders)
	corsOrigins := middleware.CORSAllowedOrigins()
	log.Info().Strs("cors_origins", corsOrigins).Msg("CORS allowlist (set CORS_ALLOWED_ORIGINS or FRONTEND_URL for production)")
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))

	// Public routes
	r.Get("/healthz", handler.HealthCheck)
	r.Handle("/metrics", promhttp.HandlerFor(gwmetrics.Registry(), promhttp.HandlerOpts{
		EnableOpenMetrics: false,
	}))

	r.Route("/api/v1", func(api chi.Router) {
		// Static venue uploads (must live on this router: /api/v1 is a mount catch-all).
		api.Handle("/uploads/*", handler.ServeVenueUploads(uploadRoot))

		// Auth (public)
		api.Post("/auth/register", authHandler.Register)
		api.Post("/auth/login", authHandler.Login)
		api.Post("/auth/refresh", authHandler.RefreshToken)
		api.Post("/auth/logout", authHandler.Logout)
		api.With(middleware.ForgotPasswordRateLimit(log)).Post("/auth/forgot-password", authHandler.ForgotPassword)
		api.Post("/auth/reset-password", authHandler.ResetPassword)

		// OAuth (public)
		api.Get("/auth/google", oauthHandler.GoogleRedirect)
		api.Get("/auth/google/callback", oauthHandler.GoogleCallback)
		api.Get("/auth/vk", oauthHandler.VKRedirect)
		api.Get("/auth/vk/callback", oauthHandler.VKCallback)

		// Venues (public read)
		api.Get("/venues", venueHandler.List)
		api.Get("/venues/search", venueHandler.Search)
		api.Group(func(pubSlug chi.Router) {
			pubSlug.Use(middleware.AuthOptional(authClient))
			pubSlug.Get("/venues/{slug}/availability", venueHandler.AvailabilityBySlug)
			pubSlug.Get("/venues/{slug}", venueHandler.GetBySlug)
		})

		// Reviews (public read)
		api.Get("/venues/{venueId}/reviews", reviewHandler.ListByVenue)

		api.Get("/masters", masterHandler.ListPublic)
		api.Get("/masters/{slug}", masterHandler.GetPublic)

		// Payment webhook (public, called by YooKassa)
		api.Post("/payments/webhook", paymentHandler.Webhook)

		// Product analytics (public; no PII in props by contract)
		api.Post("/analytics/events", analyticsHandler.CollectEvent)

		// Protected routes
		api.Group(func(protected chi.Router) {
			protected.Use(middleware.Auth(authClient))

			// User profile
			protected.Get("/users/me", userHandler.GetMe)
			protected.Patch("/users/me", userHandler.UpdateMe)

			// Venues (write, venue_owner only)
			protected.With(middleware.RequireRole("venue_owner", "master")).Post("/venues", venueHandler.Create)
			protected.With(middleware.RequireRole("venue_owner", "master")).Post("/venues/{id}/submit-for-review", venueHandler.SubmitForReview)
			protected.With(middleware.RequireRole("venue_owner", "master")).Patch("/venues/{id}", venueHandler.Update)
			protected.With(middleware.RequireRole("venue_owner", "master")).Post("/venues/{id}/photos", venueHandler.UploadVenuePhoto)
			protected.With(middleware.RequireRole("venue_owner", "master")).Delete("/venues/{id}/photos/{photoId}", venueHandler.DeleteVenuePhoto)
			protected.With(middleware.RequireRole("venue_owner", "master")).Post("/venues/{id}/photos/{photoId}/cover", venueHandler.SetVenueCoverPhoto)
			protected.With(middleware.RequireRole("venue_owner", "master")).Post("/venues/{id}/halls/{hallId}/photos", venueHandler.UploadVenueHallPhoto)
			protected.With(middleware.RequireRole("venue_owner", "master")).Delete("/venues/{id}/halls/{hallId}/photos/{photoId}", venueHandler.DeleteVenueHallPhoto)
			protected.With(middleware.RequireRole("venue_owner", "master")).Post("/venues/{id}/halls/{hallId}/photos/{photoId}/cover", venueHandler.SetVenueHallCoverPhoto)

			// Owner / CRM views (any authenticated user; venue-service enforces access)
			ownerCabinet := middleware.RequireRole("user", "venue_owner", "master")
			protected.With(ownerCabinet).Get("/owner/venues", venueHandler.ListOwnerVenues)
			protected.With(ownerCabinet).Get("/owner/venues/{venueId}/slot-blocks", venueHandler.ListOwnerSlotBlocks)
			protected.With(ownerCabinet).Post("/owner/venues/{venueId}/slot-blocks", venueHandler.CreateOwnerSlotBlock)
			protected.With(ownerCabinet).Delete("/owner/venues/{venueId}/slot-blocks/{blockId}", venueHandler.DeleteOwnerSlotBlock)
			protected.With(ownerCabinet).Get("/owner/venues/{venueId}/bookings", bookingHandler.ListVenueBookings)
			protected.With(ownerCabinet).Get("/owner/venues/{venueId}/staff", venueHandler.ListVenueStaff)
			protected.With(ownerCabinet).Post("/owner/venues/{venueId}/staff", venueHandler.AddVenueStaffByEmail)
			protected.With(ownerCabinet).Delete("/owner/venues/{venueId}/staff/{userId}", venueHandler.RemoveVenueStaff)
			protected.With(ownerCabinet).Get("/owner/venues/{venueId}/crm/tasks", venueHandler.ListVenueCRMTasks)
			protected.With(ownerCabinet).Post("/owner/venues/{venueId}/crm/tasks", venueHandler.CreateVenueCRMTask)
			protected.With(ownerCabinet).Post("/owner/venues/{venueId}/crm/tasks/{taskId}/complete", venueHandler.CompleteVenueCRMTask)
			protected.With(ownerCabinet).Get("/owner/venues/{venueId}/bookings/{bookingId}/staff-notes", bookingHandler.ListBookingStaffNotes)
			protected.With(ownerCabinet).Post("/owner/venues/{venueId}/bookings/{bookingId}/staff-notes", bookingHandler.AddBookingStaffNote)

			// Кабинет мастера: подроутер + сначала более длинные пути (Chi /profile vs /profile/submit-for-review).
			protected.Route("/owner/master", func(om chi.Router) {
				om.Use(middleware.RequireRole("master"))
				om.Post("/profile/submit-for-review", masterHandler.SubmitForReview)
				om.Post("/profile/photos/{photoId}/cover", masterHandler.SetMasterCoverPhoto)
				om.Delete("/profile/photos/{photoId}", masterHandler.DeleteMasterPhoto)
				om.Post("/profile/photos", masterHandler.UploadMasterPhoto)
				om.Get("/profile", masterHandler.GetMyProfile)
				om.Post("/profile", masterHandler.CreateMyProfile)
				om.Patch("/profile", masterHandler.PatchMyProfile)
				om.Get("/bookings", masterHandler.ListMyBookings)
			})

			protected.With(middleware.RequireRole("user", "venue_owner", "master", "admin")).Post("/masters/{slug}/bookings", masterHandler.CreateBooking)

			// Bookings
			protected.Post("/bookings", bookingHandler.Create)
			protected.Get("/bookings/my", bookingHandler.ListMy)
			protected.Get("/bookings/{id}", bookingHandler.Get)
			protected.Post("/bookings/{id}/cancel", bookingHandler.Cancel)

			// Reviews (write)
			protected.Post("/reviews", reviewHandler.Create)
			protected.Post("/venues/{venueId}/reviews", reviewHandler.CreateForVenue)

			// Admin routes
			protected.With(middleware.RequireRole("admin")).Get("/admin/venues", venueHandler.ListPending)
			protected.With(middleware.RequireRole("admin")).Post("/admin/venues/{id}/moderate", venueHandler.Moderate)

			protected.With(middleware.RequireRole("admin")).Get("/admin/masters", masterHandler.ListForModeration)
			protected.With(middleware.RequireRole("admin")).Post("/admin/masters/{id}/moderate", masterHandler.Moderate)
			protected.With(middleware.RequireRole("admin")).Get("/admin/masters/{id}/moderation-history", masterHandler.ModerationHistory)
		})
	})

	httpHandler := http.Handler(r)
	if strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" {
		httpHandler = otelhttp.NewHandler(r, "api-gateway",
			otelhttp.WithFilter(func(req *http.Request) bool {
				return req.URL.Path != "/metrics"
			}),
		)
	}

	srv := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      httpHandler,
		ReadTimeout:  90 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info().Str("port", httpPort).Msg("starting api-gateway")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
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
