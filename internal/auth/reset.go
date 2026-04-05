package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/mail"
	"github.com/goosemooz/something-backend/internal/types"
	surrealdb "github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

var (
	ErrPasswordResetNotFound = errors.New("password reset token not found")
	ErrPasswordResetExpired  = errors.New("password reset token expired")
	ErrPasswordResetUsed     = errors.New("password reset token already used")
	ErrPasswordResetDisabled = errors.New("password reset email is not configured")
)

type passwordResetToken struct {
	ID        types.RecordID `json:"id"`
	TokenHash string         `json:"token_hash"`
	UserID    string         `json:"user_id"`
	ExpiresAt time.Time      `json:"expires_at"`
	CreatedAt time.Time      `json:"created_at"`
	UsedAt    *time.Time     `json:"used_at,omitempty"`
}

type passwordResetUser struct {
	ID    types.RecordID `json:"id"`
	Email string         `json:"email"`
}

type PasswordResetManager struct {
	db       *db.DB
	cfg      *config.Config
	mailer   mail.Mailer
	sessions *SessionManager
	now      func() time.Time
}

func NewPasswordResetManager(database *db.DB, cfg *config.Config, mailer mail.Mailer, sessions *SessionManager) *PasswordResetManager {
	return &PasswordResetManager{
		db:       database,
		cfg:      cfg,
		mailer:   mailer,
		sessions: sessions,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (m *PasswordResetManager) SendReset(ctx context.Context, email string) error {
	if m.mailer == nil || strings.TrimSpace(m.cfg.AppBaseURL) == "" {
		return ErrPasswordResetDisabled
	}

	user, err := m.getUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}

	rawToken, hashedToken, err := generatePasswordResetToken()
	if err != nil {
		return err
	}

	now := m.now()
	if _, err := surrealdb.Create[passwordResetToken](ctx, m.db.Client, "password_resets", map[string]any{
		"token_hash": hashedToken,
		"user_id":    user.ID.String(),
		"expires_at": now.Add(m.cfg.PasswordResetTTL),
		"created_at": now,
	}); err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}

	resetURL := strings.TrimRight(m.cfg.AppBaseURL, "/") + "/reset-password?token=" + url.QueryEscape(rawToken)
	if err := m.mailer.SendPasswordReset(ctx, user.Email, resetURL); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	return nil
}

func (m *PasswordResetManager) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	record, err := m.getPasswordResetToken(ctx, hashPasswordResetToken(rawToken))
	if err != nil {
		return err
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	userID := strings.TrimPrefix(record.UserID, "users:")
	userRID := models.NewRecordID("users", userID)
	if _, err := surrealdb.Merge[map[string]any](ctx, m.db.Client, userRID, map[string]any{
		"password_hash": hash,
		"updated_at":    m.now(),
	}); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	now := m.now()
	resetRID := models.NewRecordID("password_resets", strings.TrimPrefix(record.ID.String(), "password_resets:"))
	if _, err := surrealdb.Merge[passwordResetToken](ctx, m.db.Client, resetRID, map[string]any{
		"used_at": now,
	}); err != nil {
		return fmt.Errorf("mark password reset used: %w", err)
	}

	if err := m.sessions.RevokeAllForUser(ctx, record.UserID); err != nil {
		return err
	}
	return nil
}

func (m *PasswordResetManager) getUserByEmail(ctx context.Context, email string) (*passwordResetUser, error) {
	results, err := surrealdb.Query[[]passwordResetUser](ctx, m.db.Client,
		"SELECT * FROM users WHERE email = $email LIMIT 1",
		map[string]any{"email": email})
	if err != nil {
		return nil, fmt.Errorf("query user by email: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}

	user := (*results)[0].Result[0]
	return &user, nil
}

func (m *PasswordResetManager) getPasswordResetToken(ctx context.Context, tokenHash string) (*passwordResetToken, error) {
	results, err := surrealdb.Query[[]passwordResetToken](ctx, m.db.Client,
		"SELECT * FROM password_resets WHERE token_hash = $token_hash LIMIT 1",
		map[string]any{"token_hash": tokenHash})
	if err != nil {
		return nil, fmt.Errorf("query password reset token: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, ErrPasswordResetNotFound
	}

	record := (*results)[0].Result[0]
	now := m.now()
	if record.UsedAt != nil {
		return nil, ErrPasswordResetUsed
	}
	if !record.ExpiresAt.After(now) {
		return nil, ErrPasswordResetExpired
	}
	return &record, nil
}

func generatePasswordResetToken() (raw string, hashed string, err error) {
	raw, err = randomHex(32)
	if err != nil {
		return "", "", err
	}
	return raw, hashPasswordResetToken(raw), nil
}

func hashPasswordResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
