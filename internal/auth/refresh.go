package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/types"
	surrealdb "github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

const refreshCookieName = "refresh_token"

var (
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	ErrRefreshTokenExpired  = errors.New("refresh token expired")
	ErrRefreshTokenRevoked  = errors.New("refresh token revoked")
)

type RefreshToken struct {
	ID             types.RecordID `json:"id"`
	TokenHash      string         `json:"token_hash"`
	UserID         string         `json:"user_id"`
	ExpiresAt      time.Time      `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
	RevokedAt      *time.Time     `json:"revoked_at,omitempty"`
	ReplacedByHash string         `json:"replaced_by_hash,omitempty"`
}

type SessionManager struct {
	db  *db.DB
	cfg *config.Config
	now func() time.Time
}

func NewSessionManager(database *db.DB, cfg *config.Config) *SessionManager {
	return &SessionManager{
		db:  database,
		cfg: cfg,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (m *SessionManager) IssueSession(ctx context.Context, w http.ResponseWriter, userID string) (string, error) {
	accessToken, err := GenerateToken(userID, m.cfg)
	if err != nil {
		return "", err
	}

	refreshToken, hash, err := generateRefreshToken()
	if err != nil {
		return "", err
	}

	now := m.now()
	if _, err := surrealdb.Create[RefreshToken](ctx, m.db.Client, "refresh_tokens", map[string]any{
		"token_hash":       hash,
		"user_id":          userID,
		"expires_at":       now.Add(m.cfg.RefreshTokenTTL),
		"created_at":       now,
		"replaced_by_hash": "",
	}); err != nil {
		return "", fmt.Errorf("create refresh token: %w", err)
	}

	setRefreshCookie(w, refreshToken, m.cfg)
	return accessToken, nil
}

func (m *SessionManager) RefreshSession(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, error) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		return "", ErrRefreshTokenNotFound
	}

	record, err := m.getRefreshToken(ctx, hashRefreshToken(cookie.Value))
	if err != nil {
		return "", err
	}

	newRaw, newHash, err := generateRefreshToken()
	if err != nil {
		return "", err
	}

	now := m.now()
	rid := models.NewRecordID("refresh_tokens", strings.TrimPrefix(record.ID.String(), "refresh_tokens:"))
	if _, err := surrealdb.Merge[RefreshToken](ctx, m.db.Client, rid, map[string]any{
		"revoked_at":       now,
		"replaced_by_hash": newHash,
	}); err != nil {
		return "", fmt.Errorf("rotate refresh token: %w", err)
	}

	if _, err := surrealdb.Create[RefreshToken](ctx, m.db.Client, "refresh_tokens", map[string]any{
		"token_hash":       newHash,
		"user_id":          record.UserID,
		"expires_at":       now.Add(m.cfg.RefreshTokenTTL),
		"created_at":       now,
		"replaced_by_hash": "",
	}); err != nil {
		return "", fmt.Errorf("create rotated refresh token: %w", err)
	}

	accessToken, err := GenerateToken(record.UserID, m.cfg)
	if err != nil {
		return "", err
	}
	setRefreshCookie(w, newRaw, m.cfg)
	return accessToken, nil
}

func (m *SessionManager) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie(refreshCookieName)
	if err == nil && cookie.Value != "" {
		if record, lookupErr := m.getRefreshToken(ctx, hashRefreshToken(cookie.Value)); lookupErr == nil {
			now := m.now()
			rid := models.NewRecordID("refresh_tokens", strings.TrimPrefix(record.ID.String(), "refresh_tokens:"))
			if _, err := surrealdb.Merge[RefreshToken](ctx, m.db.Client, rid, map[string]any{
				"revoked_at": now,
			}); err != nil {
				return fmt.Errorf("revoke refresh token: %w", err)
			}
		}
	}

	clearRefreshCookie(w, m.cfg)
	return nil
}

func (m *SessionManager) getRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	results, err := surrealdb.Query[[]RefreshToken](ctx, m.db.Client,
		"SELECT * FROM refresh_tokens WHERE token_hash = $token_hash LIMIT 1",
		map[string]any{"token_hash": tokenHash})
	if err != nil {
		return nil, fmt.Errorf("query refresh token: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, ErrRefreshTokenNotFound
	}

	token := (*results)[0].Result[0]
	now := m.now()
	if token.RevokedAt != nil {
		return nil, ErrRefreshTokenRevoked
	}
	if !token.ExpiresAt.After(now) {
		return nil, ErrRefreshTokenExpired
	}
	return &token, nil
}

func generateRefreshToken() (raw string, hashed string, err error) {
	raw, err = randomHex(32)
	if err != nil {
		return "", "", err
	}
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func setRefreshCookie(w http.ResponseWriter, token string, cfg *config.Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/auth",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.RefreshCookieSecure,
		MaxAge:   int(cfg.RefreshTokenTTL.Seconds()),
	})
}

func clearRefreshCookie(w http.ResponseWriter, cfg *config.Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.RefreshCookieSecure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}
