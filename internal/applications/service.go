package applications

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/types"
	"github.com/goosemooz/something-backend/internal/users"
	surrealdb "github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

var (
	ErrAlreadyApplied = errors.New("already applied")
	ErrForbidden      = errors.New("forbidden")
	ErrNotFound       = errors.New("not found")
	ErrNoSpotsLeft    = errors.New("no spots left")
)

type Service struct {
	db      *db.DB
	userSvc *users.Service
}

func NewService(db *db.DB) *Service {
	return &Service{
		db:      db,
		userSvc: users.NewService(db),
	}
}

type opportunityRecord struct {
	ID         types.RecordID `json:"id"`
	OrgID      types.RecordID `json:"org_id"`
	Title      string         `json:"title"`
	Category   string         `json:"category"`
	Difficulty int            `json:"difficulty"`
	Date       time.Time      `json:"date"`
	Duration   float64        `json:"duration"`
	Location   string         `json:"location"`
	MaxSpots   int            `json:"max_spots"`
	SpotsLeft  int            `json:"spots_left"`
}

func (o opportunityRecord) summary() *OpportunitySummary {
	return &OpportunitySummary{
		ID:         o.ID,
		OrgID:      o.OrgID,
		Title:      o.Title,
		Category:   o.Category,
		Difficulty: o.Difficulty,
		Date:       o.Date,
		Duration:   o.Duration,
		Location:   o.Location,
		MaxSpots:   o.MaxSpots,
		SpotsLeft:  o.SpotsLeft,
	}
}

func (s *Service) Create(ctx context.Context, userID, opportunityID string) (*Application, error) {
	userRecordID, err := models.ParseRecordID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}
	if !strings.HasPrefix(userID, "users:") {
		return nil, ErrForbidden
	}
	oppRecordID, err := models.ParseRecordID(opportunityID)
	if err != nil {
		return nil, fmt.Errorf("invalid opportunity ID: %w", err)
	}

	user, err := surrealdb.Select[map[string]any](ctx, s.db.Client, *userRecordID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}
	if user == nil {
		return nil, ErrForbidden
	}

	opp, err := surrealdb.Select[map[string]any](ctx, s.db.Client, *oppRecordID)
	if err != nil {
		return nil, fmt.Errorf("failed to query opportunity: %w", err)
	}
	if opp == nil {
		return nil, ErrNotFound
	}

	// Check for duplicate application
	existing, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications WHERE user_id = $uid AND opportunity_id = $opp LIMIT 1",
		map[string]any{"uid": userRecordID, "opp": oppRecordID})
	if err != nil {
		return nil, fmt.Errorf("failed to check existing application: %w", err)
	}
	if len(*existing) > 0 && len((*existing)[0].Result) > 0 {
		return nil, ErrAlreadyApplied
	}

	now := time.Now().UTC()
	result, err := surrealdb.Create[Application](ctx, s.db.Client, "applications", map[string]any{
		"user_id":        userRecordID,
		"opportunity_id": oppRecordID,
		"status":         StatusPending,
		"xp_awarded":     false,
		"created_at":     now,
		"updated_at":     now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}
	return result, nil
}

func (s *Service) ListByUser(ctx context.Context, userID string) ([]Application, error) {
	if err := s.awardEarnedXP(ctx, userID); err != nil {
		return nil, err
	}

	userRecordID, err := models.ParseRecordID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	results, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications WHERE user_id = $uid",
		map[string]any{"uid": userRecordID})
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}
	if len(*results) == 0 {
		return []Application{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) ListByOrg(ctx context.Context, orgID string) ([]Application, error) {
	orgRecordID, err := models.ParseRecordID(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid org ID: %w", err)
	}

	results, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications WHERE opportunity_id.org_id = $org_id ORDER BY created_at DESC",
		map[string]any{"org_id": orgRecordID})
	if err != nil {
		return nil, fmt.Errorf("failed to list org applications: %w", err)
	}
	if len(*results) == 0 {
		return []Application{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) ListDetailedByOrg(ctx context.Context, orgID string) ([]OrgApplication, error) {
	apps, err := s.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}

	detailed := make([]OrgApplication, 0, len(apps))
	for _, app := range apps {
		item, err := s.enrichOrgApplication(ctx, app)
		if err != nil {
			return nil, err
		}
		detailed = append(detailed, item)
	}
	return detailed, nil
}

// UpdateStatus updates the status of an application, but only if the given orgID
// owns the opportunity the application belongs to.
func (s *Service) UpdateStatus(ctx context.Context, id, orgID string, status Status) (*Application, error) {
	appRID := models.NewRecordID("applications", id)
	app, err := surrealdb.Select[Application](ctx, s.db.Client, appRID)
	if err != nil {
		return nil, fmt.Errorf("failed to query application: %w", err)
	}
	if app == nil {
		return nil, ErrNotFound
	}

	oppID, ok := app.OpportunityID.ID.(string)
	if !ok {
		return nil, fmt.Errorf("invalid opportunity ID")
	}
	opp, err := s.getOpportunityByID(ctx, oppID)
	if err != nil {
		return nil, err
	}
	if opp == nil || opp.OrgID.String() != orgID {
		return nil, ErrForbidden
	}
	if status == StatusAccepted && app.Status != StatusAccepted {
		if opp.SpotsLeft <= 0 {
			return nil, ErrNoSpotsLeft
		}
		if _, err := s.updateOpportunitySpots(ctx, oppID, opp.SpotsLeft-1); err != nil {
			return nil, err
		}
	}
	if app.Status == StatusAccepted && status != StatusAccepted && opp.SpotsLeft < opp.MaxSpots {
		if _, err := s.updateOpportunitySpots(ctx, oppID, opp.SpotsLeft+1); err != nil {
			return nil, err
		}
	}

	result, err := surrealdb.Merge[Application](ctx, s.db.Client, appRID, map[string]any{
		"status":     status,
		"updated_at": time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update application: %w", err)
	}
	return result, nil
}

func (s *Service) UpdateStatusDetailed(ctx context.Context, id, orgID string, status Status) (*OrgApplication, error) {
	app, err := s.UpdateStatus(ctx, id, orgID, status)
	if err != nil {
		return nil, err
	}
	item, err := s.enrichOrgApplication(ctx, *app)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Delete withdraws an application. Only the user who created it can delete it.
func (s *Service) Delete(ctx context.Context, id, userID string) error {
	rid := models.NewRecordID("applications", id)
	app, err := surrealdb.Select[Application](ctx, s.db.Client, rid)
	if err != nil {
		return fmt.Errorf("failed to query application: %w", err)
	}
	if app == nil {
		return ErrNotFound
	}
	if app.UserID.String() != userID {
		return ErrForbidden
	}
	if app.Status == StatusAccepted {
		oppID, ok := app.OpportunityID.ID.(string)
		if ok {
			opp, err := s.getOpportunityByID(ctx, oppID)
			if err != nil {
				return err
			}
			if opp != nil && opp.SpotsLeft < opp.MaxSpots {
				if _, err := s.updateOpportunitySpots(ctx, oppID, opp.SpotsLeft+1); err != nil {
					return err
				}
			}
		}
	}
	if _, err := surrealdb.Delete[Application](ctx, s.db.Client, rid); err != nil {
		return fmt.Errorf("failed to delete application: %w", err)
	}
	return nil
}

func (s *Service) updateOpportunitySpots(ctx context.Context, oppID string, spotsLeft int) (*opportunityRecord, error) {
	rid := models.NewRecordID("opportunities", oppID)
	result, err := surrealdb.Merge[opportunityRecord](ctx, s.db.Client, rid, map[string]any{
		"spots_left": spotsLeft,
		"updated_at": time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update opportunity spots: %w", err)
	}
	return result, nil
}

func (s *Service) awardEarnedXP(ctx context.Context, userID string) error {
	userRecordID, err := models.ParseRecordID(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	results, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications WHERE user_id = $uid AND status = 'accepted' AND xp_awarded = false",
		map[string]any{"uid": userRecordID})
	if err != nil {
		return fmt.Errorf("failed to query accepted applications: %w", err)
	}
	if len(*results) == 0 {
		return nil
	}

	now := time.Now().UTC()
	for _, app := range (*results)[0].Result {
		oppID, ok := app.OpportunityID.ID.(string)
		if !ok {
			continue
		}
		opp, err := s.getOpportunityByID(ctx, oppID)
		if err != nil {
			return err
		}
		if opp == nil || now.Before(opp.Date) {
			continue
		}

		xp := calculateOpportunityXP(*opp)
		if _, err := s.userSvc.AddXP(ctx, strings.TrimPrefix(userID, "users:"), xp); err != nil {
			return err
		}

		appRID := models.NewRecordID("applications", strings.TrimPrefix(app.ID.String(), "applications:"))
		if _, err := surrealdb.Merge[Application](ctx, s.db.Client, appRID, map[string]any{
			"xp_awarded": true,
			"updated_at": time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("failed to mark application xp_awarded: %w", err)
		}
	}

	return nil
}

func calculateOpportunityXP(opp opportunityRecord) int {
	if opp.Duration <= 0 {
		return 0
	}

	multiplier := 50.0
	switch opp.Difficulty {
	case 1:
		multiplier = 100.0
	case 2:
		multiplier = 150.0
	}

	return int(math.Round(opp.Duration * multiplier))
}

func (s *Service) getOpportunityByID(ctx context.Context, id string) (*opportunityRecord, error) {
	rid := models.NewRecordID("opportunities", id)
	result, err := surrealdb.Select[opportunityRecord](ctx, s.db.Client, rid)
	if err != nil {
		return nil, fmt.Errorf("failed to query opportunity: %w", err)
	}
	return result, nil
}

func (s *Service) enrichOrgApplication(ctx context.Context, app Application) (OrgApplication, error) {
	item := OrgApplication{Application: app}

	userID, ok := app.UserID.ID.(string)
	if ok {
		user, err := s.userSvc.GetByID(ctx, userID)
		if err != nil {
			return OrgApplication{}, err
		}
		item.User = user
	}

	oppID, ok := app.OpportunityID.ID.(string)
	if ok {
		opp, err := s.getOpportunityByID(ctx, oppID)
		if err != nil {
			return OrgApplication{}, err
		}
		if opp != nil {
			item.Opportunity = opp.summary()
		}
	}

	return item, nil
}
