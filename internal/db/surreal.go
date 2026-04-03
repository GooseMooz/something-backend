package db

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/goosemooz/something-backend/config"
	"github.com/goosemooz/something-backend/internal/types"
	surrealdb "github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

//go:embed schema.surql
var schemaSQL string

type DB struct {
	Client *surrealdb.DB
}

var Instance *DB

func Connect(ctx context.Context, cfg *config.Config) (*DB, error) {
	client, err := surrealdb.FromEndpointURLString(ctx, cfg.SurrealURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	if _, err := client.SignIn(ctx, &surrealdb.Auth{
		Username: cfg.SurrealUser,
		Password: cfg.SurrealPassword,
	}); err != nil {
		return nil, fmt.Errorf("failed to sign in: %w", err)
	}

	if err := client.Use(ctx, cfg.SurrealNS, cfg.SurrealDB); err != nil {
		return nil, fmt.Errorf("failed to select ns/db: %w", err)
	}

	Instance = &DB{Client: client}
	return Instance, nil
}

func (d *DB) ApplySchema(ctx context.Context) error {
	if _, err := surrealdb.Query[any](ctx, d.Client, schemaSQL, map[string]any{}); err != nil {
		return fmt.Errorf("failed to apply schema: %w", err)
	}
	if err := d.migrateOpportunityData(ctx); err != nil {
		return fmt.Errorf("failed to migrate opportunity data: %w", err)
	}
	return nil
}

type rawOpportunity struct {
	ID        types.RecordID `json:"id"`
	Duration  any            `json:"duration"`
	MaxSpots  int            `json:"max_spots"`
	SpotsLeft *int           `json:"spots_left"`
}

type countRow struct {
	Count int `json:"count"`
}

var durationPattern = regexp.MustCompile(`\d+(\.\d+)?`)

func (d *DB) migrateOpportunityData(ctx context.Context) error {
	results, err := surrealdb.Query[[]rawOpportunity](ctx, d.Client,
		"SELECT id, duration, max_spots, spots_left FROM opportunities",
		map[string]any{})
	if err != nil {
		return nil
	}
	if len(*results) == 0 {
		return nil
	}

	for _, opp := range (*results)[0].Result {
		updates := map[string]any{}

		duration, changed := normalizeDuration(opp.Duration)
		if changed {
			updates["duration"] = duration
		}

		if opp.SpotsLeft == nil {
			acceptedCount, err := d.acceptedApplicationsCount(ctx, opp.ID.String())
			if err != nil {
				return err
			}
			spotsLeft := maxInt(0, opp.MaxSpots-acceptedCount)
			updates["spots_left"] = spotsLeft
		}

		if len(updates) == 0 {
			continue
		}

		updates["updated_at"] = time.Now().UTC()
		if _, err := surrealdb.Merge[map[string]any](ctx, d.Client, opp.ID.RecordID, updates); err != nil {
			return fmt.Errorf("migrate opportunity %s: %w", opp.ID.String(), err)
		}
	}

	return nil
}

func (d *DB) acceptedApplicationsCount(ctx context.Context, opportunityID string) (int, error) {
	oppRID, err := models.ParseRecordID(opportunityID)
	if err != nil {
		return 0, fmt.Errorf("parse opportunity id %q: %w", opportunityID, err)
	}

	results, err := surrealdb.Query[[]countRow](ctx, d.Client,
		"SELECT count() AS count FROM applications WHERE opportunity_id = $opp AND status = 'accepted' GROUP ALL",
		map[string]any{"opp": oppRID})
	if err != nil {
		return 0, fmt.Errorf("count accepted applications: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return 0, nil
	}
	return (*results)[0].Result[0].Count, nil
}

func normalizeDuration(v any) (float64, bool) {
	switch typed := v.(type) {
	case float64:
		return typed, false
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		match := durationPattern.FindString(typed)
		if match == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(match, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func maxInt(a, b int) int {
	return int(math.Max(float64(a), float64(b)))
}
