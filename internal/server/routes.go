package server

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/applications"
	"github.com/goosemooz/something-backend/internal/auth"
	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/mail"
	"github.com/goosemooz/something-backend/internal/opportunities"
	"github.com/goosemooz/something-backend/internal/orgs"
	"github.com/goosemooz/something-backend/internal/ratelimit"
	"github.com/goosemooz/something-backend/internal/storage"
	"github.com/goosemooz/something-backend/internal/users"
)

func SetupRoutes(r chi.Router, database *db.DB, cfg *config.Config, store *storage.Storage, mailer mail.Mailer) {
	sessionManager := auth.NewSessionManager(database, cfg)
	resetManager := auth.NewPasswordResetManager(database, cfg, mailer, sessionManager)
	authHandler := auth.NewHandler(sessionManager, resetManager)
	userHandler := users.NewHandler(users.NewService(database), cfg, store, sessionManager)
	orgHandler := orgs.NewHandler(orgs.NewService(database), cfg, store, sessionManager)
	appService := applications.NewService(database)
	oppHandler := opportunities.NewHandler(opportunities.NewService(database), appService)
	appHandler := applications.NewHandler(appService)

	// Auth
	r.Route("/auth", func(r chi.Router) {
		r.With(ratelimit.NewIPRateLimiter(5, time.Minute)).Post("/register", userHandler.Register)
		r.With(ratelimit.NewIPRateLimiter(10, time.Minute)).Post("/login", userHandler.Login)
		r.With(ratelimit.NewIPRateLimiter(30, time.Minute)).Post("/refresh", authHandler.Refresh)
		r.With(ratelimit.NewIPRateLimiter(5, time.Minute)).Post("/forgot-password", authHandler.ForgotPassword)
		r.With(ratelimit.NewIPRateLimiter(10, time.Minute)).Post("/reset-password", authHandler.ResetPassword)
		r.Post("/logout", authHandler.Logout)

		r.Route("/org", func(r chi.Router) {
			r.With(ratelimit.NewIPRateLimiter(5, time.Minute)).Post("/register", orgHandler.Register)
			r.With(ratelimit.NewIPRateLimiter(10, time.Minute)).Post("/login", orgHandler.Login)
		})
	})

	// Users
	r.Route("/users", func(r chi.Router) {
		r.Get("/{id}", userHandler.Get)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(cfg))
			r.Use(auth.RequireUserAuth)
			r.Put("/{id}", userHandler.Update)
			r.Put("/{id}/password", userHandler.ChangePassword)
			r.Post("/{id}/pfp", userHandler.UploadPFP)
			r.Post("/{id}/resume", userHandler.UploadResume)
		})
	})

	// Orgs
	r.Route("/orgs", func(r chi.Router) {
		r.Get("/", orgHandler.List)
		r.Get("/{id}", orgHandler.Get)
		r.Get("/{id}/opportunities", oppHandler.ListByOrg)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(cfg))
			r.Use(auth.RequireOrgAuth)
			r.Get("/{id}/applications", appHandler.ListByOrg)
			r.Put("/{id}", orgHandler.Update)
			r.Post("/{id}/pfp", orgHandler.UploadPFP)
		})
	})

	// Opportunities
	r.Route("/opportunities", func(r chi.Router) {
		r.Get("/", oppHandler.List)
		r.Get("/{id}", oppHandler.Get)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(cfg))
			r.Use(auth.RequireOrgAuth)
			r.Post("/", oppHandler.Create)
			r.Put("/{id}", oppHandler.Update)
			r.Delete("/{id}", oppHandler.Delete)
		})
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(cfg))
			r.Use(auth.RequireUserAuth)
			r.Post("/{id}/apply", oppHandler.Apply)
		})
	})

	r.Route("/applications", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(cfg))
			r.Use(auth.RequireUserAuth)
			r.Get("/", appHandler.List)
			r.Delete("/{id}", appHandler.Delete)
		})
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(cfg))
			r.Use(auth.RequireOrgAuth)
			r.Put("/{id}", appHandler.UpdateStatus)
		})
	})
}
