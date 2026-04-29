package orgs

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

func (s *Service) Create(ctx context.Context, name, passwordHash, email, location string) (*Org, error) {
	now := time.Now().UTC()
	_, err := surrealdb.Create[Org](ctx, s.db.Client, "orgs", map[string]any{
		"name":                  name,
		"password_hash":         passwordHash,
		"email":                 email,
		"location":              location,
		"categories":            []string{},
		"description":           "",
		"s3_pfp":                "",
		"verified":              false,
		"notification_settings": DefaultNotificationSettings(),
		"created_at":            now,
		"updated_at":            now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create org: %w", err)
	}
	return s.GetByEmail(ctx, email)
}

// GetByEmail returns the org with the given email, or nil if not found.
func (s *Service) GetByEmail(ctx context.Context, email string) (*Org, error) {
	results, err := surrealdb.Query[[]Org](ctx, s.db.Client,
		"SELECT * FROM orgs WHERE email = $email LIMIT 1",
		map[string]any{"email": email})
	if err != nil {
		return nil, fmt.Errorf("failed to query org: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Org, error) {
	rid := models.NewRecordID("orgs", id)
	result, err := surrealdb.Select[Org](ctx, s.db.Client, rid)
	if err != nil {
		return nil, fmt.Errorf("failed to query org: %w", err)
	}
	return result, nil
}

func (s *Service) List(ctx context.Context) ([]Org, error) {
	results, err := surrealdb.Query[[]Org](ctx, s.db.Client,
		"SELECT * FROM orgs",
		map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to list orgs: %w", err)
	}
	if len(*results) == 0 {
		return []Org{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) Update(ctx context.Context, id string, updates map[string]any) (*Org, error) {
	updates["updated_at"] = time.Now().UTC()
	rid := models.NewRecordID("orgs", id)
	result, err := surrealdb.Merge[Org](ctx, s.db.Client, rid, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update org: %w", err)
	}
	return result, nil
}
