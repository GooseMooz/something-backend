// Package testhelper provides shared utilities for integration tests.
package testhelper

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/db"
)

// NewDB connects to SurrealDB using env vars (or sensible local defaults) and
// applies the schema into a unique database name so every test package runs in
// complete isolation — no cleanup needed between runs.
func NewDB() (*db.DB, error) {
	cfg := &config.Config{
		SurrealURL:      getEnv("SURREAL_URL", "ws://localhost:8000/rpc"),
		SurrealUser:     getEnv("SURREAL_USER", "root"),
		SurrealPassword: getEnv("SURREAL_PASSWORD", "root"),
		SurrealNS:       "test",
		// Unique DB per process — prevents any cross-package interference.
		SurrealDB: fmt.Sprintf("testdb_%d", time.Now().UnixNano()),
	}

	database, err := db.Connect(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := database.ApplySchema(context.Background()); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return database, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
