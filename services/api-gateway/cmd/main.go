package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/pkg/config"
	"github.com/tienlao/agregator/pkg/logger"
	"github.com/tienlao/agregator/services/api-gateway/internal/handler"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	log := logger.New("api-gateway")

	httpPort := config.GetEnv("HTTP_PORT", "8080")
	authAddr := config.GetEnv("AUTH_SERVICE_ADDR", "localhost:50051")
	userAddr := config.GetEnv("USER_SERVICE_ADDR", "localhost:50052")
	venueAddr := config.GetEnv("VENUE_SERVICE_ADDR", "localhost:50053")
	bookingAddr := config.GetEnv("BOOKING_SERVICE_ADDR", "localhost:50054")
	reviewAddr := config.GetEnv("REVIEW_SERVICE_ADDR", "localhost:50055")
	paymentAddr := config.GetEnv("PAYMENT_SERVICE_ADDR", "localhost:50056")

	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

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

	authClient := authv1.NewAuthServiceClient(authConn)
	userClient := userv1.NewUserServiceClient(userConn)
	venueClient := venuev1.NewVenueServiceClient(venueConn)
	bookingClient := bookingv1.NewBookingServiceClient(bookingConn)
	reviewClient := reviewv1.NewReviewServiceClient(reviewConn)
	paymentClient := paymentv1.NewPaymentServiceClient(paymentConn)

	authHandler := handler.NewAuthHandler(authClient)
	oauthHandler := handler.NewOAuthHandler(authClient, handler.OAuthConfig{
		GoogleClientID:     config.GetEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: config.GetEnv("GOOGLE_CLIENT_SECRET", ""),
		VKClientID:         config.GetEnv("VK_CLIENT_ID", ""),
		VKClientSecret:     config.GetEnv("VK_CLIENT_SECRET", ""),
		BaseURL:            config.GetEnv("BASE_URL", "http://localhost:8080"),
		FrontendURL:        config.GetEnv("FRONTEND_URL", "http://localhost:3000"),
	})
	userHandler := handler.NewUserHandler(userClient)
	venueHandler := handler.NewVenueHandler(venueClient)
	bookingHandler := handler.NewBookingHandler(bookingClient, venueClient)
	reviewHandler := handler.NewReviewHandler(reviewClient)
	paymentHandler := handler.NewPaymentHandler(paymentClient)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(chimw.Recoverer)

	// Public routes
	r.Get("/healthz", handler.HealthCheck)

	r.Route("/api/v1", func(api chi.Router) {
		// Auth (public)
		api.Post("/auth/register", authHandler.Register)
		api.Post("/auth/login", authHandler.Login)
		api.Post("/auth/refresh", authHandler.RefreshToken)
		api.Post("/auth/logout", authHandler.Logout)

		// OAuth (public)
		api.Get("/auth/google", oauthHandler.GoogleRedirect)
		api.Get("/auth/google/callback", oauthHandler.GoogleCallback)
		api.Get("/auth/vk", oauthHandler.VKRedirect)
		api.Get("/auth/vk/callback", oauthHandler.VKCallback)

		// Venues (public read)
		api.Get("/venues", venueHandler.List)
		api.Get("/venues/search", venueHandler.Search)
		api.Get("/venues/{slug}", venueHandler.GetBySlug)

		// Reviews (public read)
		api.Get("/venues/{venueId}/reviews", reviewHandler.ListByVenue)

		// Payment webhook (public, called by YooKassa)
		api.Post("/payments/webhook", paymentHandler.Webhook)

		// Protected routes
		api.Group(func(protected chi.Router) {
			protected.Use(middleware.Auth(authClient))

			// User profile
			protected.Get("/users/me", userHandler.GetMe)
			protected.Patch("/users/me", userHandler.UpdateMe)

			// Venues (write, venue_owner only)
			protected.With(middleware.RequireRole("venue_owner", "master")).Post("/venues", venueHandler.Create)
			protected.With(middleware.RequireRole("venue_owner", "master")).Patch("/venues/{id}", venueHandler.Update)

			// Owner views
			protected.With(middleware.RequireRole("venue_owner", "master")).Get("/owner/venues", venueHandler.ListOwnerVenues)
			protected.With(middleware.RequireRole("venue_owner", "master")).Get("/owner/venues/{venueId}/bookings", bookingHandler.ListVenueBookings)

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
		})
	})

	srv := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server forced to shutdown")
	}
	log.Info().Msg("server stopped")
}
