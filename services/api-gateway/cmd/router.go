package main

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/pkg/roles"
	"github.com/tienlao/agregator/pkg/storage"
	"github.com/tienlao/agregator/services/api-gateway/internal/handler"
	gwmetrics "github.com/tienlao/agregator/services/api-gateway/internal/metrics"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/suspendnotify"
)

// buildRouter wires handlers and middleware into a chi.Mux. It has no side
// effects beyond in-memory setup and can be called in tests without touching
// the network. ctx is forwarded to handlers that start background goroutines
// (e.g. chat in-memory limiter prune loop).
func buildRouter(ctx context.Context, log zerolog.Logger, cfg Config, d *deps) (*chi.Mux, error) {
	// ── Handlers ───────────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(d.Auth)

	oauthHandler, err := handler.NewOAuthHandler(log, d.Auth, handler.OAuthConfig{
		VKClientID:         cfg.VKClientID,
		VKClientSecret:     cfg.VKClientSecret,
		YandexClientID:     cfg.YandexClientID,
		YandexClientSecret: cfg.YandexClientSecret,
		BaseURL:            cfg.BaseURL,
		FrontendURL:        cfg.FrontendURL,
	})
	if err != nil {
		return nil, err
	}

	userHandler := handler.NewUserHandler(d.User, d.Auth, d.Venue, d.Master)

	suspendMail := suspendnotify.NewSender(log, d.CRM, d.User)

	// Notification handler must be built before the venue handler so it can be
	// wired as the staff-invite notifier (bell notification on AddStaff).
	notificationHandler := handler.NewNotificationHandler(log, d.Notification, d.Redis, d.NATS)

	venueHandler := handler.NewVenueHandler(d.Venue, d.User, d.CRM, d.Storage,
		handler.WithSuspendNotifier(suspendMail),
		handler.WithStaffInviteNotifier(notificationHandler),
		handler.WithCRMTaskNotifier(notificationHandler),
		handler.WithVenueModerationNotifier(notificationHandler),
	)
	if suspendMail.Enabled() {
		log.Info().Msg("SMTP configured: при приостановке и возобновлении заведения владельцу и персоналу уйдут письма")
	}

	bookingHandler := handler.NewBookingHandler(d.Booking, d.Venue, d.User,
		handler.WithBookingOwnerNotifier(notificationHandler),
	)
	reviewHandler := handler.NewReviewHandler(d.Review, d.User,
		handler.WithReviewOwnerNotifier(d.Venue, notificationHandler),
		handler.WithReviewMasterNotifier(d.Master, notificationHandler),
	)
	paymentHandler := handler.NewPaymentHandler(log, d.Payment,
		handler.ParseWebhookSecurityConfig(cfg.PaymentWebhookSecret, cfg.PaymentWebhookIPAllowlist))
	masterHandler := handler.NewMasterHandler(d.Master, d.Storage,
		handler.WithMasterOwnerNotifier(notificationHandler),
		handler.WithMasterUserClient(d.User),
	)
	payoutHandler := handler.NewPayoutHandler(log, d.Payment, d.Venue, d.Master)
	analyticsHandler := handler.NewAnalyticsHandler(log, d.NATS)

	chatHandler := handler.NewChatHandler(
		ctx, log, d.Chat, d.Booking, d.Venue, d.Master, d.User, d.CRM,
		d.RateLimiter, d.Redis, d.NATS,
	)

	if d.NATS != nil {
		if _, err := d.NATS.Subscribe("chat.fanout", func(msg *nats.Msg) {
			chatHandler.HandleFanoutMessage(msg.Data)
		}); err != nil {
			log.Warn().Err(err).Msg("chat NATS fanout subscribe failed")
		}
		if _, err := d.NATS.Subscribe("notification.fanout", func(msg *nats.Msg) {
			notificationHandler.HandleFanoutMessage(msg.Data)
		}); err != nil {
			log.Warn().Err(err).Msg("notification NATS fanout subscribe failed")
		}
	}

	var ticketRepo handler.SupportTicketsPersistence
	if d.TicketStore != nil {
		ticketRepo = d.TicketStore
	}
	supportHandler := handler.NewSupportHandler(log,
		cfg.SupportWebhookURL, cfg.SupportWebhookToken, cfg.SupportModeratorEmails,
		ticketRepo,
	)

	// ── Rate-limit middleware ──────────────────────────────────────────────
	loginRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:login:",
		Max:       cfg.RateLimitLoginMax,
		Window:    cfg.RateLimitLoginWindow,
		FailOpen:  false,
		Mode:      middleware.KeyModeIP,
	})
	registerRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:register:",
		Max:       cfg.RateLimitRegisterMax,
		Window:    cfg.RateLimitRegisterWindow,
		FailOpen:  true,
		Mode:      middleware.KeyModeIP,
	})
	analyticsRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:analytics:",
		Max:       cfg.RateLimitAnalyticsMax,
		Window:    cfg.RateLimitAnalyticsWindow,
		FailOpen:  true,
		Mode:      middleware.KeyModeIP,
	})
	supportRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:support:",
		Max:       cfg.RateLimitSupportMax,
		Window:    cfg.RateLimitSupportWindow,
		FailOpen:  true,
		Mode:      middleware.KeyModeUser,
	})
	webhookRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:webhook:",
		Max:       cfg.RateLimitWebhookMax,
		Window:    cfg.RateLimitWebhookWindow,
		FailOpen:  true,
		Mode:      middleware.KeyModeIP,
	})
	forgotPasswordRL := middleware.ForgotPasswordRateLimit(log, d.RateLimiter)
	// reset-password consumes a one-time token produced by forgot-password.
	// Brute-forcing the token (SHA-256 of 32 random bytes) is computationally
	// infeasible, but rate-limiting still prevents amplification attacks where
	// an attacker floods the endpoint to trigger argon2id on every attempt.
	// Limits mirror forgot-password: 10 req / 15 min per IP, fail-closed.
	resetPasswordRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:reset-password:",
		Max:       cfg.RateLimitResetPasswordMax,
		Window:    cfg.RateLimitResetPasswordWindow,
		FailOpen:  false,
		Mode:      middleware.KeyModeIP,
	})
	// resend-verification sends an email on every hit, so it is rate-limited
	// like forgot-password (10 req / 15 min per IP, fail-closed) to prevent an
	// attacker from using it as an email-amplification vector. verify-email
	// consumes a SHA-256(32-byte) token and is infeasible to brute-force, but is
	// limited too to cap argon2id-free DB churn under a flood.
	resendVerificationRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:resend-verification:",
		Max:       cfg.RateLimitResetPasswordMax,
		Window:    cfg.RateLimitResetPasswordWindow,
		FailOpen:  false,
		Mode:      middleware.KeyModeIP,
	})
	verifyEmailRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:verify-email:",
		Max:       cfg.RateLimitResetPasswordMax,
		Window:    cfg.RateLimitResetPasswordWindow,
		FailOpen:  false,
		Mode:      middleware.KeyModeIP,
	})
	adminRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:admin:",
		Max:       cfg.RateLimitAdminMax,
		Window:    cfg.RateLimitAdminWindow,
		FailOpen:  false,
		Mode:      middleware.KeyModeUser,
	})
	// masterPublicRL guards GET /masters and GET /masters/{slug} against
	// enumeration and scraping. Keyed by IP so anonymous crawlers are throttled
	// independently of authenticated users. FailOpen: Redis outage should not
	// take down the public catalogue for legitimate visitors.
	masterPublicRL := middleware.RateLimit(log, d.RateLimiter, middleware.RateLimitConfig{
		KeyPrefix: "rl:master-public:",
		Max:       cfg.RateLimitMasterPublicMax,
		Window:    cfg.RateLimitMasterPublicWindow,
		FailOpen:  true,
		Mode:      middleware.KeyModeIP,
	})

	// ── Router ────────────────────────────────────────────────────────────
	r := chi.NewRouter()

	r.Use(middleware.RealIP(log)) // must be first: rewrites r.RemoteAddr from trusted XFF
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
	r.Use(middleware.Recoverer(log))
	// Compress is NOT global: static uploads (images) are already compressed
	// and gzip-ing them wastes CPU with no size benefit. Level 3 is the
	// sweet spot for JSON — vs level 5 it saves ~5% extra but costs 2-3× CPU.
	// Compression is applied per-route group below (API routes only).

	// ── Public ────────────────────────────────────────────────────────────
	r.Get("/healthz", handler.HealthCheck)
	// /metrics intentionally NOT mounted here: the public listener is proxied
	// wholesale by Caddy ({$API_DOMAIN} → api-gateway:8080), which would expose
	// route patterns and traffic volumes. Prometheus scrapes the internal
	// METRICS_ADDR listener started in main.go instead.

	r.Route("/api/v1", func(api chi.Router) {
		// /uploads/* is only mounted when using DiskUploader (dev/single-replica).
		// In production (MinIO), files are served directly from MinIO/CDN.
		if disk, ok := d.Storage.(*storage.DiskUploader); ok {
			api.Handle("/uploads/*", disk)
		}

		// All other API routes get gzip at level 3 and a hard write timeout.
		// Level 3 vs 5: ~5% larger output, but 2-3× cheaper on CPU for JSON payloads.
		// WebSocket upgrade routes are mounted outside this group — see below.
		api.Group(func(r chi.Router) {
			r.Use(chimw.Compress(3))
			r.Use(middleware.RequestTimeout(cfg.RESTHandlerTimeout))

			// Auth (public)
			r.With(registerRL).Post("/auth/register", authHandler.Register)
			r.With(loginRL).Post("/auth/login", authHandler.Login)
			r.Post("/auth/refresh", authHandler.RefreshToken)
			r.Post("/auth/logout", authHandler.Logout)
			r.With(forgotPasswordRL).Post("/auth/forgot-password", authHandler.ForgotPassword)
			r.With(resetPasswordRL).Post("/auth/reset-password", authHandler.ResetPassword)
			r.With(verifyEmailRL).Post("/auth/verify-email", authHandler.VerifyEmail)
			r.With(resendVerificationRL).Post("/auth/resend-verification", authHandler.ResendVerification)

			// OAuth (public)
			r.Get("/auth/vk", oauthHandler.VKRedirect)
			r.Get("/auth/vk/callback", oauthHandler.VKCallback)
			r.Get("/auth/yandex", oauthHandler.YandexRedirect)
			r.Get("/auth/yandex/callback", oauthHandler.YandexCallback)

			// Venues (public read)
			r.Get("/venues", venueHandler.List)
			r.Get("/venues/search", venueHandler.Search)
			r.Group(func(r chi.Router) {
				r.Use(middleware.AuthOptional(log, cfg.JWTECPublicKey))
				r.Get("/venues/{slug}/availability", venueHandler.AvailabilityBySlug)
				r.Get("/venues/{slug}", venueHandler.GetBySlug)
			})

			// Reviews (public read)
			r.Get("/venues/{venueId}/reviews", reviewHandler.ListByVenue)
			r.Get("/masters/{masterId}/reviews", reviewHandler.ListByMaster)
			r.With(masterPublicRL).Get("/masters", masterHandler.ListPublic)
			r.With(masterPublicRL).Get("/masters/{slug}", masterHandler.GetPublic)

			// Payment webhook (public, called by YooKassa)
			r.With(webhookRL).Post("/payments/webhook", paymentHandler.Webhook)

			// Analytics (public)
			r.With(analyticsRL).Post("/analytics/events", analyticsHandler.CollectEvent)

			// ── Protected ─────────────────────────────────────────────────
			r.Group(func(r chi.Router) {
				r.Use(middleware.Auth(log, cfg.JWTECPublicKey, d.Redis))

				// Email verification status — polled by the partner flow to
				// advance past the "check your inbox" screen once the user clicks
				// the link. Reads live from auth-service, not the (possibly stale) JWT.
				r.Get("/auth/email-verification", authHandler.GetEmailVerificationStatus)

				// User profile
				r.Get("/users/me", userHandler.GetMe)
				r.Patch("/users/me", userHandler.UpdateMe)
				r.Delete("/users/me", userHandler.DeleteMe)

				// Venues (write)
				// emailVerified gates the two actions that create moderation load —
				// venue creation here and master-profile creation below — so an
				// unverified throwaway email cannot flood the moderation queue.
				//
				// Temporarily bypassable via EMAIL_VERIFICATION_GATE_ENABLED=false:
				// the gate becomes a pass-through so unverified users can publish.
				// The startup warning makes the relaxed state visible in logs.
				var emailVerified func(http.Handler) http.Handler
				if cfg.EmailVerificationGateEnabled {
					emailVerified = middleware.RequireEmailVerified(log, d.Auth)
				} else {
					log.Warn().Msg("email verification gate DISABLED (EMAIL_VERIFICATION_GATE_ENABLED=false) — unverified users can create venues and master profiles")
					emailVerified = func(next http.Handler) http.Handler { return next }
				}
				ownerOrMaster := middleware.RequireRole(roles.RoleVenueOwner, roles.RoleMaster)
				r.With(ownerOrMaster, emailVerified).Post("/venues", venueHandler.Create)
				r.With(ownerOrMaster).Post("/venues/{id}/submit-for-review", venueHandler.SubmitForReview)
				r.With(ownerOrMaster).Patch("/venues/{id}", venueHandler.Update)
				r.With(ownerOrMaster).Post("/venues/{id}/photos", venueHandler.UploadVenuePhoto)
				r.With(ownerOrMaster).Delete("/venues/{id}/photos/{photoId}", venueHandler.DeleteVenuePhoto)
				r.With(ownerOrMaster).Post("/venues/{id}/photos/{photoId}/cover", venueHandler.SetVenueCoverPhoto)
				r.With(ownerOrMaster).Post("/venues/{id}/halls/{hallId}/photos", venueHandler.UploadVenueHallPhoto)
				r.With(ownerOrMaster).Delete("/venues/{id}/halls/{hallId}/photos/{photoId}", venueHandler.DeleteVenueHallPhoto)
				r.With(ownerOrMaster).Post("/venues/{id}/halls/{hallId}/photos/{photoId}/cover", venueHandler.SetVenueHallCoverPhoto)

				// Owner cabinet
				ownerCabinet := middleware.RequireRole(roles.RoleUser, roles.RoleVenueOwner, roles.RoleMaster)
				r.With(ownerCabinet).Get("/owner/venues", venueHandler.ListOwnerVenues)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/slot-blocks", venueHandler.ListOwnerSlotBlocks)
				r.With(ownerCabinet).Post("/owner/venues/{venueId}/slot-blocks", venueHandler.CreateOwnerSlotBlock)
				r.With(ownerCabinet).Delete("/owner/venues/{venueId}/slot-blocks/{blockId}", venueHandler.DeleteOwnerSlotBlock)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/bookings", bookingHandler.ListVenueBookings)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/staff", venueHandler.ListVenueStaff)
				r.With(ownerCabinet).Post("/owner/venues/{venueId}/staff", venueHandler.AddVenueStaffByEmail)
				r.With(ownerCabinet).Delete("/owner/venues/{venueId}/staff/{userId}", venueHandler.RemoveVenueStaff)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/crm/tasks", venueHandler.ListVenueCRMTasks)
				r.With(ownerCabinet).Post("/owner/venues/{venueId}/crm/tasks", venueHandler.CreateVenueCRMTask)
				r.With(ownerCabinet).Patch("/owner/venues/{venueId}/crm/tasks/{taskId}", venueHandler.UpdateVenueCRMTask)
				r.With(ownerCabinet).Post("/owner/venues/{venueId}/crm/tasks/{taskId}/complete", venueHandler.CompleteVenueCRMTask)
				r.With(ownerCabinet).Post("/owner/venues/{venueId}/crm/tasks/{taskId}/reopen", venueHandler.ReopenVenueCRMTask)
				r.With(ownerCabinet).Delete("/owner/venues/{venueId}/crm/tasks/{taskId}", venueHandler.CancelVenueCRMTask)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/crm/guests", venueHandler.ListVenueGuests)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/crm/guests/{userId}", venueHandler.GetVenueGuest)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/bookings/{bookingId}/staff-notes", bookingHandler.ListBookingStaffNotes)
				r.With(ownerCabinet).Post("/owner/venues/{venueId}/bookings/{bookingId}/staff-notes", bookingHandler.AddBookingStaffNote)

				// Venue payout cabinet (escrow). The handler enforces the real
				// gate — caller must be the venue's legal owner (OwnerId match) —
				// so the role middleware here only narrows the candidate set.
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/payout-method", payoutHandler.GetVenuePayoutMethod)
				r.With(ownerCabinet).Put("/owner/venues/{venueId}/payout-method", payoutHandler.SetVenuePayoutMethod)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/balance", payoutHandler.GetVenueBalance)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/ledger", payoutHandler.ListVenueLedger)
				r.With(ownerCabinet).Get("/owner/venues/{venueId}/payouts", payoutHandler.ListVenuePayouts)

				// Chat v1 REST (non-WS) — covered by RequestTimeout above.
				chatCabinet := middleware.RequireRole(roles.RoleUser, roles.RoleVenueOwner, roles.RoleMaster, roles.RoleAdmin)
				r.With(chatCabinet).Post("/chat/threads", chatHandler.EnsureThread)
				r.With(chatCabinet).Get("/chat/threads", chatHandler.ListThreads)
				r.With(chatCabinet).Get("/chat/threads/{threadId}/messages", chatHandler.ListMessages)
				r.With(chatCabinet).Post("/chat/threads/{threadId}/messages", chatHandler.SendMessage)
				r.With(chatCabinet).Post("/chat/threads/{threadId}/read", chatHandler.MarkRead)

				// Support
				allAuth := middleware.RequireRole(roles.RoleUser, roles.RoleVenueOwner, roles.RoleMaster, roles.RoleAdmin)
				r.With(allAuth).With(supportRL).Post("/support/contact", supportHandler.Contact)

				// Master cabinet
				r.Route("/owner/master", func(r chi.Router) {
					r.Use(middleware.RequireRole(roles.RoleMaster))
					r.Post("/profile/submit-for-review", masterHandler.SubmitForReview)
					r.Post("/profile/photos/{photoId}/cover", masterHandler.SetMasterCoverPhoto)
					r.Delete("/profile/photos/{photoId}", masterHandler.DeleteMasterPhoto)
					r.Post("/profile/photos", masterHandler.UploadMasterPhoto)
					r.Get("/profile", masterHandler.GetMyProfile)
					// emailVerified (defined above in this protected group) gates
					// master-profile creation — the second moderation-load action.
					r.With(emailVerified).Post("/profile", masterHandler.CreateMyProfile)
					r.Patch("/profile", masterHandler.PatchMyProfile)
					r.Get("/bookings", masterHandler.ListMyBookings)

					// Master payout cabinet (escrow). partner_id is resolved from
					// the caller's own master profile, so no extra ownership check
					// is needed beyond the RoleMaster gate on this group.
					r.Get("/payout-method", payoutHandler.GetMasterPayoutMethod)
					r.Put("/payout-method", payoutHandler.SetMasterPayoutMethod)
					r.Get("/balance", payoutHandler.GetMasterBalance)
					r.Get("/ledger", payoutHandler.ListMasterLedger)
					r.Get("/payouts", payoutHandler.ListMasterPayouts)
				})

				r.With(allAuth).Post("/masters/{slug}/bookings", masterHandler.CreateBooking)
				r.With(allAuth).Get("/my/master-bookings", masterHandler.ListMyClientBookings)

				// Bookings
				r.Post("/bookings", bookingHandler.Create)
				r.Get("/bookings/my", bookingHandler.ListMy)
				r.Get("/bookings/{id}", bookingHandler.Get)
				r.Post("/bookings/{id}/cancel", bookingHandler.Cancel)

				// Reviews (write)
				r.Post("/reviews", reviewHandler.Create)
				r.Post("/venues/{venueId}/reviews", reviewHandler.CreateForVenue)
				r.Post("/masters/{masterId}/reviews", reviewHandler.CreateForMaster)

				// Admin
				admin := middleware.RequireRole(roles.RoleAdmin)
				r.With(admin, adminRL).Get("/admin/venues", venueHandler.ListPending)
				r.With(admin, adminRL).Post("/admin/venues/{id}/moderate", venueHandler.Moderate)
				r.With(admin, adminRL).Get("/admin/masters", masterHandler.ListForModeration)
				r.With(admin, adminRL).Post("/admin/masters/{id}/moderate", masterHandler.Moderate)
				r.With(admin, adminRL).Get("/admin/masters/{id}/moderation-history", masterHandler.ModerationHistory)
				r.With(admin, adminRL).Get("/admin/support/tickets", supportHandler.AdminListTickets)
				r.With(admin, adminRL).Get("/admin/support/tickets/{requestID}", supportHandler.AdminGetTicket)
				r.With(admin, adminRL).Post("/admin/support/reply", supportHandler.AdminReply)
				// Prometheus exposition for the /admin/metrics frontend page.
				// The unauthenticated /metrics endpoint moved to the internal
				// METRICS_ADDR listener; this is the only authenticated way to
				// read gateway metrics from outside the docker network.
				r.With(admin, adminRL).Get("/admin/metrics",
					promhttp.HandlerFor(gwmetrics.Registry(), promhttp.HandlerOpts{}).ServeHTTP)
			})
		})

		// ── WebSocket upgrade route (v1) — NO RequestTimeout ─────────────────
		// http.TimeoutHandler closes the connection after the deadline, which
		// would terminate long-lived WebSocket sessions. These routes are
		// intentionally outside the timed group above.
		// Auth middleware is still applied; only the write timeout is absent.
		api.Group(func(r chi.Router) {
			r.Use(middleware.Auth(log, cfg.JWTECPublicKey, d.Redis))
			chatCabinet := middleware.RequireRole(roles.RoleUser, roles.RoleVenueOwner, roles.RoleMaster, roles.RoleAdmin)
			r.With(chatCabinet).Get("/chat/ws", chatHandler.WS)
		})
	})

	// ── API v2 ────────────────────────────────────────────────────────────
	// v2 lives at /api/v2/… — a separate prefix from /api/v1 so the two
	// versions are independently addressable and cannot be confused.
	//
	// Current v2 surface: chat only. The v1 chat endpoints remain for
	// backward compatibility; new clients should target v2.
	//
	// Differences from v1 chat:
	//   • URL style: resource:action  (e.g. POST /threads:ensure, POST /threads/{id}:read)
	//   • WebSocket auth uses a one-time ticket (POST /chat/ws-ticket → GET /chat/ws?ticket=…)
	//     instead of the Authorization header on the upgrade request.
	//   • WS fanout events use the "event" key  (e.g. "chat.message.created")
	//     instead of the legacy "type" key ("message_new").
	r.Route("/api/v2", func(api chi.Router) {
		api.Group(func(r chi.Router) {
			r.Use(chimw.Compress(3))
			r.Use(middleware.RequestTimeout(cfg.RESTHandlerTimeout))
			r.Use(middleware.Auth(log, cfg.JWTECPublicKey, d.Redis))

			chatCabinet := middleware.RequireRole(roles.RoleUser, roles.RoleVenueOwner, roles.RoleMaster, roles.RoleAdmin)
			r.With(chatCabinet).Post("/chat/threads:ensure", chatHandler.EnsureThread)
			r.With(chatCabinet).Get("/chat/threads", chatHandler.ListThreads)
			r.With(chatCabinet).Get("/chat/threads/{threadId}/messages", chatHandler.ListMessages)
			r.With(chatCabinet).Post("/chat/threads/{threadId}/messages", chatHandler.SendMessage)
			r.With(chatCabinet).Post("/chat/threads/{threadId}:read", chatHandler.MarkRead)
			r.With(chatCabinet).Post("/chat/ws-ticket", chatHandler.IssueWSTicket)

			// Notifications ("колокольчик") — per-user in-app inbox.
			r.With(chatCabinet).Get("/notifications", notificationHandler.List)
			r.With(chatCabinet).Get("/notifications/unread-count", notificationHandler.UnreadCount)
			r.With(chatCabinet).Post("/notifications/read-all", notificationHandler.MarkAllRead)
			r.With(chatCabinet).Post("/notifications/{notificationId}/read", notificationHandler.MarkRead)
			r.With(chatCabinet).Post("/notifications/ws-ticket", notificationHandler.IssueWSTicket)
		})

		// WebSocket upgrade — no RequestTimeout (same rationale as v1).
		api.Group(func(r chi.Router) {
			r.Use(middleware.Auth(log, cfg.JWTECPublicKey, d.Redis))
			chatCabinet := middleware.RequireRole(roles.RoleUser, roles.RoleVenueOwner, roles.RoleMaster, roles.RoleAdmin)
			r.With(chatCabinet).Get("/chat/ws", chatHandler.WS)
			r.With(chatCabinet).Get("/notifications/ws", notificationHandler.WS)
		})
	})

	return r, nil
}

// serverTimeouts wraps the router in an http.Server with production-safe timeouts.
//
// Timeout rationale:
//   - ReadHeaderTimeout: 10s — защита от Slowloris; клиент обязан прислать заголовки быстро.
//   - ReadTimeout: 30s — достаточно для чтения тела любого REST-запроса (макс. 5 MiB фото).
//   - WriteTimeout: 0 — отключён на уровне сервера намеренно: WebSocket-соединения живут
//     произвольно долго и были бы убиты глобальным дедлайном. Защита REST-хендлеров
//     обеспечивается на уровне роутера через middleware.RequestTimeout (http.TimeoutHandler),
//     который применяется ко всем маршрутам кроме WS-апгрейдов. Значение таймаута
//     задаётся через REST_HANDLER_TIMEOUT (по умолчанию 60s).
//   - IdleTimeout: 120s — keep-alive соединение без активности закрывается.
func serverTimeouts(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // REST protected by middleware.RequestTimeout; WS intentionally unbounded
		IdleTimeout:       120 * time.Second,
	}
}
