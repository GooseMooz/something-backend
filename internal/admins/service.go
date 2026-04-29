package admins

import (
	"context"
	"fmt"
	"strings"
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

func (s *Service) Create(ctx context.Context, email, passwordHash, name string) (*Admin, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(name) == "" {
		name = "Admin"
	}

	_, err := surrealdb.Create[Admin](ctx, s.db.Client, "admins", map[string]any{
		"email":         email,
		"password_hash": passwordHash,
		"name":          name,
		"created_at":    now,
		"updated_at":    now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create admin: %w", err)
	}
	return s.GetByEmail(ctx, email)
}

func (s *Service) EnsureDefault(ctx context.Context, email, passwordHash, name string) (*Admin, error) {
	existing, err := s.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return s.Create(ctx, email, passwordHash, name)
	}

	updates := map[string]any{"password_hash": passwordHash}
	if strings.TrimSpace(name) != "" {
		updates["name"] = name
	}
	return s.Update(ctx, existing.ID.ID.(string), updates)
}

func (s *Service) GetByEmail(ctx context.Context, email string) (*Admin, error) {
	results, err := surrealdb.Query[[]Admin](ctx, s.db.Client,
		"SELECT * FROM admins WHERE email = $email LIMIT 1",
		map[string]any{"email": email})
	if err != nil {
		return nil, fmt.Errorf("failed to query admin: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Admin, error) {
	rid := models.NewRecordID("admins", id)
	result, err := surrealdb.Select[Admin](ctx, s.db.Client, rid)
	if err != nil {
		return nil, fmt.Errorf("failed to query admin: %w", err)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, id string, updates map[string]any) (*Admin, error) {
	updates["updated_at"] = time.Now().UTC()
	rid := models.NewRecordID("admins", id)
	result, err := surrealdb.Merge[Admin](ctx, s.db.Client, rid, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update admin: %w", err)
	}
	return result, nil
}
