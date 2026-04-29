package server

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/admins"
	"github.com/goosemooz/something-backend/internal/applications"
	"github.com/goosemooz/something-backend/internal/auth"
	"github.com/goosemooz/something-backend/internal/campaigns"
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
	userService := users.NewService(database)
	orgService := orgs.NewService(database)
	appService := applications.NewService(database).WithMailer(mailer)
	oppService := opportunities.NewService(database)
	userHandler := users.NewHandler(userService, cfg, store, sessionManager)
	orgHandler := orgs.NewHandler(orgService, cfg, store, sessionManager)
	oppHandler := opportunities.NewHandler(oppService, appService)
	appHandler := applications.NewHandler(appService)
	campaignHandler := campaigns.NewHandler(cfg, mailer)
	adminHandler := admins.NewHandler(admins.NewService(database), userService, orgService, oppService, appService, mailer, sessionManager)

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
		r.Route("/admin", func(r chi.Router) {
			r.With(ratelimit.NewIPRateLimiter(10, time.Minute)).Post("/login", adminHandler.Login)
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
			r.Get("/{id}/notification-settings", userHandler.GetNotificationSettings)
			r.Put("/{id}/notification-settings", userHandler.UpdateNotificationSettings)
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
			r.Put("/{id}/password", orgHandler.ChangePassword)
			r.Get("/{id}/notification-settings", orgHandler.GetNotificationSettings)
			r.Put("/{id}/notification-settings", orgHandler.UpdateNotificationSettings)
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

	r.Route("/campaigns", func(r chi.Router) {
		r.With(ratelimit.NewIPRateLimiter(2, time.Minute)).Post("/launch", campaignHandler.SendLaunchNotification)
	})

	r.Route("/admin", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireAdminAuth)

		r.Get("/users", adminHandler.ListUsers)
		r.Get("/orgs", adminHandler.ListOrgs)
		r.Put("/orgs/{id}/verification", adminHandler.SetOrgVerification)
		r.Get("/opportunities", adminHandler.ListOpportunities)
		r.Put("/opportunities/{id}", adminHandler.UpdateOpportunity)
		r.Get("/opportunities/{id}/applications", adminHandler.ListOpportunityApplications)
		r.Get("/applications", adminHandler.ListApplications)
		r.Put("/applications/{id}", adminHandler.UpdateApplicationStatus)
		r.Post("/campaigns", adminHandler.SendCampaign)
	})
}
