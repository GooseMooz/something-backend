package opportunities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/goosemooz/something-backend/internal/db"
	surrealdb "github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
)

type Service struct {
	db *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{db: db}
}

func (s *Service) List(ctx context.Context) ([]Opportunity, error) {
	results, err := surrealdb.Query[[]Opportunity](ctx, s.db.Client,
		"SELECT * FROM opportunities",
		map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to list opportunities: %w", err)
	}
	if len(*results) == 0 {
		return []Opportunity{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Opportunity, error) {
	rid := models.NewRecordID("opportunities", id)
	result, err := surrealdb.Select[Opportunity](ctx, s.db.Client, rid)
	if err != nil {
		return nil, fmt.Errorf("failed to query opportunity: %w", err)
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, orgID string, opp Opportunity) (*Opportunity, error) {
	orgRecordID, err := models.ParseRecordID(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid org ID: %w", err)
	}

	if opp.Tags == nil {
		opp.Tags = []string{}
	}

	now := time.Now().UTC()
	result, err := surrealdb.Create[Opportunity](ctx, s.db.Client, "opportunities", map[string]any{
		"org_id":         orgRecordID,
		"title":          opp.Title,
		"category":       opp.Category,
		"difficulty":     opp.Difficulty,
		"description":    opp.Description,
		"date":           opp.Date,
		"duration":       opp.Duration,
		"location":       opp.Location,
		"max_spots":      opp.MaxSpots,
		"spots_left":     opp.MaxSpots,
		"recurring":      opp.Recurring,
		"drop_in":        opp.DropIn,
		"event_link":     opp.EventLink,
		"resources_link": opp.ResourcesLink,
		"tags":           opp.Tags,
		"created_at":     now,
		"updated_at":     now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create opportunity: %w", err)
	}
	return result, nil
}

func (s *Service) ListByOrg(ctx context.Context, orgID string) ([]Opportunity, error) {
	orgRID := models.NewRecordID("orgs", orgID)
	results, err := surrealdb.Query[[]Opportunity](ctx, s.db.Client,
		"SELECT * FROM opportunities WHERE org_id = $org_id",
		map[string]any{"org_id": orgRID})
	if err != nil {
		return nil, fmt.Errorf("failed to list opportunities: %w", err)
	}
	if len(*results) == 0 {
		return []Opportunity{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) Delete(ctx context.Context, id, orgID string) error {
	rid := models.NewRecordID("opportunities", id)
	opp, err := surrealdb.Select[Opportunity](ctx, s.db.Client, rid)
	if err != nil {
		return fmt.Errorf("failed to query opportunity: %w", err)
	}
	if opp == nil {
		return ErrNotFound
	}
	if opp.OrgID.String() != orgID {
		return ErrForbidden
	}
	if _, err := surrealdb.Delete[Opportunity](ctx, s.db.Client, rid); err != nil {
		return fmt.Errorf("failed to delete opportunity: %w", err)
	}
	return nil
}

func (s *Service) Update(ctx context.Context, id, orgID string, updates map[string]any) (*Opportunity, error) {
	return s.update(ctx, id, orgID, false, updates)
}

func (s *Service) UpdateAsAdmin(ctx context.Context, id string, updates map[string]any) (*Opportunity, error) {
	return s.update(ctx, id, "", true, updates)
}

func (s *Service) update(ctx context.Context, id, orgID string, skipOwnershipCheck bool, updates map[string]any) (*Opportunity, error) {
	rid := models.NewRecordID("opportunities", id)
	opp, err := surrealdb.Select[Opportunity](ctx, s.db.Client, rid)
	if err != nil {
		return nil, fmt.Errorf("failed to query opportunity: %w", err)
	}
	if opp == nil {
		return nil, ErrNotFound
	}
	if !skipOwnershipCheck && opp.OrgID.String() != orgID {
		return nil, ErrForbidden
	}

	if newMaxRaw, ok := updates["max_spots"]; ok {
		newMax, ok := newMaxRaw.(int)
		if !ok {
			return nil, fmt.Errorf("invalid max_spots type")
		}
		acceptedCount := opp.MaxSpots - opp.SpotsLeft
		if newMax < acceptedCount {
			return nil, fmt.Errorf("max_spots cannot be less than accepted applications")
		}
		updates["spots_left"] = newMax - acceptedCount
	}

	updates["updated_at"] = time.Now().UTC()
	result, err := surrealdb.Merge[Opportunity](ctx, s.db.Client, rid, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update opportunity: %w", err)
	}
	return result, nil
}
