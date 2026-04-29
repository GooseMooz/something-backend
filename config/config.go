package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port string

	SurrealURL      string
	SurrealUser     string
	SurrealPassword string
	SurrealNS       string
	SurrealDB       string

	JWTSecret           string
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	PasswordResetTTL    time.Duration
	RefreshCookieSecure bool

	S3PDFBucket string
	S3PFPBucket string
	S3Region    string
	S3AccessKey string
	S3SecretKey string

	SMTPHost       string
	SMTPPort       int
	SMTPUsername   string
	SMTPPassword   string
	SMTPFrom       string
	AppBaseURL     string
	CampaignAPIKey string

	AdminEmail    string
	AdminPassword string
	AdminName     string
}

func Load() *Config {
	cfg := &Config{
		Port: mustGetEnv("PORT"),

		SurrealURL:      mustGetEnv("SURREAL_URL"),
		SurrealUser:     mustGetEnv("SURREAL_USER"),
		SurrealPassword: mustGetEnv("SURREAL_PASSWORD"),
		SurrealNS:       mustGetEnv("SURREAL_NS"),
		SurrealDB:       mustGetEnv("SURREAL_DB"),

		JWTSecret:           mustGetEnv("JWT_SECRET"),
		AccessTokenTTL:      getDurationEnv("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:     getDurationEnv("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		PasswordResetTTL:    getDurationEnv("PASSWORD_RESET_TTL", time.Hour),
		RefreshCookieSecure: getBoolEnv("REFRESH_COOKIE_SECURE", false),

		S3PDFBucket: mustGetEnv("S3_PDF_BUCKET"),
		S3PFPBucket: mustGetEnv("S3_PFP_BUCKET"),
		S3Region:    mustGetEnv("S3_REGION"),
		S3AccessKey: mustGetEnv("S3_ACCESS_KEY"),
		S3SecretKey: mustGetEnv("S3_SECRET_KEY"),

		SMTPHost:       os.Getenv("SMTP_HOST"),
		SMTPPort:       getIntEnv("SMTP_PORT", 587),
		SMTPUsername:   os.Getenv("SMTP_USERNAME"),
		SMTPPassword:   os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:       os.Getenv("SMTP_FROM"),
		AppBaseURL:     os.Getenv("APP_BASE_URL"),
		CampaignAPIKey: os.Getenv("CAMPAIGN_API_KEY"),

		AdminEmail:    os.Getenv("ADMIN_EMAIL"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		AdminName:     getEnv("ADMIN_NAME", "Admin"),
	}
	return cfg
}

func mustGetEnv(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		panic(fmt.Sprintf("Required env var %q is not set!", key))
	}
	return v
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("Invalid duration env var %q: %v", key, err))
	}
	return parsed
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		panic(fmt.Sprintf("Invalid bool env var %q: %v", key, err))
	}
	return parsed
}

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("Invalid int env var %q: %v", key, err))
	}
	return parsed
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
