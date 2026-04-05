package main

import (
	"context"
	"log"

	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/mail"
	"github.com/goosemooz/something-backend/internal/server"
	"github.com/goosemooz/something-backend/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	database, err := db.Connect(context.Background(), cfg)
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}
	log.Println("Connected to SurrealDB")

	if err := database.ApplySchema(context.Background()); err != nil {
		log.Fatalf("Schema setup failed: %v", err)
	}
	log.Println("Schema applied")

	store, err := storage.New(cfg)
	if err != nil {
		log.Fatalf("S3 init failed: %v", err)
	}

	var mailer mail.Mailer
	if cfg.SMTPHost != "" && cfg.SMTPUsername != "" && cfg.SMTPPassword != "" && cfg.SMTPFrom != "" && cfg.AppBaseURL != "" {
		smtpMailer, err := mail.NewSMTPMailer(cfg)
		if err != nil {
			log.Fatalf("SMTP init failed: %v", err)
		}
		mailer = smtpMailer
	}

	srv := server.New(cfg, database, store, mailer)
	log.Printf("Server starting on port %s", cfg.Port)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
