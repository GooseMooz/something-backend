package users

import (
	"context"
	"fmt"
	"time"

	"github.com/goosemooz/something-backend/internal/db"
	surrealdb "github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

type Service struct {
	db *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{db: db}
}

// Create inserts a new user and returns it. Email uniqueness is enforced by the DB schema.
func (s *Service) Create(ctx context.Context, email, passwordHash, name string) (*User, error) {
	now := time.Now().UTC()
	_, err := surrealdb.Create[User](ctx, s.db.Client, "users", map[string]any{
		"email":                 email,
		"password_hash":         passwordHash,
		"name":                  name,
		"skills":                []string{},
		"bio":                   "",
		"categories":            []string{},
		"intensity":             "low",
		"xp":                    0,
		"s3_pfp":                "",
		"s3_pdf":                "",
		"badges":                []string{},
		"notification_settings": DefaultNotificationSettings(),
		"created_at":            now,
		"updated_at":            now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	// SELECT queries return IDs as plain strings, so we fetch back
	// the created record to get a properly populated ID field.
	return s.GetByEmail(ctx, email)
}

// GetByEmail returns the user with the given email, or nil if not found.
func (s *Service) GetByEmail(ctx context.Context, email string) (*User, error) {
	results, err := surrealdb.Query[[]User](ctx, s.db.Client,
		"SELECT * FROM users WHERE email = $email LIMIT 1",
		map[string]any{"email": email})
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*User, error) {
	rid := models.NewRecordID("users", id)
	result, err := surrealdb.Select[User](ctx, s.db.Client, rid)
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}
	return result, nil
}

func (s *Service) List(ctx context.Context) ([]User, error) {
	results, err := surrealdb.Query[[]User](ctx, s.db.Client,
		"SELECT * FROM users ORDER BY created_at DESC",
		map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	if len(*results) == 0 {
		return []User{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) Update(ctx context.Context, id string, updates map[string]any) (*User, error) {
	updates["updated_at"] = time.Now().UTC()
	rid := models.NewRecordID("users", id)
	result, err := surrealdb.Merge[User](ctx, s.db.Client, rid, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	return result, nil
}

func (s *Service) AddXP(ctx context.Context, id string, amount int) (*User, error) {
	if amount <= 0 {
		return s.GetByID(ctx, id)
	}

	user, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}

	return s.Update(ctx, id, map[string]any{"xp": user.XP + amount})
}
