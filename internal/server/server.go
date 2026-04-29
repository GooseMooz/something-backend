package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/applications"
	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/mail"
	"github.com/goosemooz/something-backend/internal/storage"
)

type Server struct {
	router *chi.Mux
	cfg    *config.Config
	db     *db.DB
	store  *storage.Storage
	mailer mail.Mailer
}

func New(cfg *config.Config, database *db.DB, store *storage.Storage, mailer mail.Mailer) *Server {
	s := &Server{
		router: chi.NewRouter(),
		cfg:    cfg,
		db:     database,
		store:  store,
		mailer: mailer,
	}
	s.setupMiddleware()
	SetupRoutes(s.router, database, cfg, store, mailer)
	s.startNotificationScheduler()
	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)   // attaches a unique ID to every request
	s.router.Use(middleware.RealIP)      // reads X-Forwarded-For so you get the real client IP
	s.router.Use(middleware.Logger)      // logs method, path, status, latency
	s.router.Use(middleware.Recoverer)   // catches panics and returns 500 instead of crashing
	s.router.Use(middleware.Compress(5)) // gzip responses at compression level 5
	s.router.Use(middleware.Timeout(30 * time.Second))

	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) startNotificationScheduler() {
	if s.mailer == nil {
		return
	}

	appService := applications.NewService(s.db).WithMailer(s.mailer)
	go func() {
		run := func() {
			now := time.Now().UTC()
			if err := appService.SendDueOpportunityReminders(context.Background(), now); err != nil {
				log.Printf("opportunity reminder scheduler failed: %v", err)
			}
			if err := appService.SendApplicantDigests(context.Background(), now); err != nil {
				log.Printf("applicant digest scheduler failed: %v", err)
			}
		}

		run()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}
