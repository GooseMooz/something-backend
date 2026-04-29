package applications

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/mail"
	"github.com/goosemooz/something-backend/internal/orgs"
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
	orgSvc  *orgs.Service
	mailer  mail.Mailer
	now     func() time.Time
}

func NewService(db *db.DB) *Service {
	return &Service{
		db:      db,
		userSvc: users.NewService(db),
		orgSvc:  orgs.NewService(db),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) WithMailer(mailer mail.Mailer) *Service {
	s.mailer = mailer
	return s
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

func (s *Service) List(ctx context.Context) ([]Application, error) {
	results, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications ORDER BY created_at DESC",
		map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}
	if len(*results) == 0 {
		return []Application{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) ListDetailed(ctx context.Context) ([]OrgApplication, error) {
	apps, err := s.List(ctx)
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

func (s *Service) ListByOpportunity(ctx context.Context, opportunityID string) ([]Application, error) {
	oppRecordID, err := recordIDFor("opportunities", opportunityID)
	if err != nil {
		return nil, fmt.Errorf("invalid opportunity ID: %w", err)
	}

	results, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications WHERE opportunity_id = $opp ORDER BY created_at DESC",
		map[string]any{"opp": oppRecordID})
	if err != nil {
		return nil, fmt.Errorf("failed to list opportunity applications: %w", err)
	}
	if len(*results) == 0 {
		return []Application{}, nil
	}
	return (*results)[0].Result, nil
}

func (s *Service) ListDetailedByOpportunity(ctx context.Context, opportunityID string) ([]OrgApplication, error) {
	apps, err := s.ListByOpportunity(ctx, opportunityID)
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
	return s.updateStatus(ctx, id, orgID, false, status)
}

func (s *Service) UpdateStatusAsAdmin(ctx context.Context, id string, status Status) (*Application, error) {
	return s.updateStatus(ctx, id, "", true, status)
}

func (s *Service) updateStatus(ctx context.Context, id, orgID string, skipOwnershipCheck bool, status Status) (*Application, error) {
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
	if opp == nil {
		if skipOwnershipCheck {
			return nil, ErrNotFound
		}
		return nil, ErrForbidden
	}
	if !skipOwnershipCheck && opp.OrgID.String() != orgID {
		return nil, ErrForbidden
	}
	previousStatus := app.Status
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
	if previousStatus != status {
		s.notifyUserStatusChanged(ctx, *result, *opp, status)
	}
	return result, nil
}

func (s *Service) UpdateStatusDetailed(ctx context.Context, id, orgID string, status Status) (*OrgApplication, error) {
	app, err := s.updateStatus(ctx, id, orgID, false, status)
	if err != nil {
		return nil, err
	}
	item, err := s.enrichOrgApplication(ctx, *app)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) UpdateStatusAsAdminDetailed(ctx context.Context, id string, status Status) (*OrgApplication, error) {
	app, err := s.updateStatus(ctx, id, "", true, status)
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
	var acceptedOpp *opportunityRecord
	if app.Status == StatusAccepted {
		oppID, ok := app.OpportunityID.ID.(string)
		if ok {
			opp, err := s.getOpportunityByID(ctx, oppID)
			if err != nil {
				return err
			}
			acceptedOpp = opp
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
	if acceptedOpp != nil {
		s.notifyOrgAcceptedWithdrawal(ctx, *app, *acceptedOpp)
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

func (s *Service) notifyUserStatusChanged(ctx context.Context, app Application, opp opportunityRecord, status Status) {
	if s.mailer == nil {
		return
	}

	userID, ok := app.UserID.ID.(string)
	if !ok {
		return
	}
	user, err := s.userSvc.GetByID(ctx, userID)
	if err != nil {
		log.Printf("application status notification user lookup failed: %v", err)
		return
	}
	if user == nil {
		return
	}

	switch status {
	case StatusAccepted:
		if !user.NotificationSettings.ApplicationAccepted {
			return
		}
		subject := "You were accepted for " + opp.Title
		body := fmt.Sprintf("Good news, %s.\n\nYou were accepted for %s.\n\nDate: %s\nLocation: %s", user.Name, opp.Title, formatNotificationTime(opp.Date), opp.Location)
		if err := s.mailer.SendNotification(ctx, user.Email, subject, body); err != nil {
			log.Printf("send accepted notification failed for %s: %v", user.Email, err)
		}
	case StatusRejected:
		if !user.NotificationSettings.ApplicationDeclined {
			return
		}
		subject := "Update on your application for " + opp.Title
		body := fmt.Sprintf("Hi %s,\n\nYour application for %s was declined.\n\nThere are more opportunities available on Something Matters.", user.Name, opp.Title)
		if err := s.mailer.SendNotification(ctx, user.Email, subject, body); err != nil {
			log.Printf("send declined notification failed for %s: %v", user.Email, err)
		}
	}
}

func (s *Service) notifyOrgAcceptedWithdrawal(ctx context.Context, app Application, opp opportunityRecord) {
	if s.mailer == nil {
		return
	}

	orgID, ok := opp.OrgID.ID.(string)
	if !ok {
		return
	}
	org, err := s.orgSvc.GetByID(ctx, orgID)
	if err != nil {
		log.Printf("accepted withdrawal org lookup failed: %v", err)
		return
	}
	if org == nil || !org.NotificationSettings.AcceptedWithdrawal {
		return
	}

	userName := "An accepted applicant"
	if userID, ok := app.UserID.ID.(string); ok {
		user, err := s.userSvc.GetByID(ctx, userID)
		if err != nil {
			log.Printf("accepted withdrawal user lookup failed: %v", err)
		} else if user != nil && strings.TrimSpace(user.Name) != "" {
			userName = user.Name
		}
	}

	subject := "Accepted applicant withdrew from " + opp.Title
	body := fmt.Sprintf("%s withdrew their accepted application for %s.\n\nDate: %s\nLocation: %s", userName, opp.Title, formatNotificationTime(opp.Date), opp.Location)
	if err := s.mailer.SendNotification(ctx, org.Email, subject, body); err != nil {
		log.Printf("send accepted withdrawal notification failed for %s: %v", org.Email, err)
	}
}

func (s *Service) NotifyOpportunityCanceled(ctx context.Context, opportunityID string) {
	if s.mailer == nil {
		return
	}

	opp, err := s.getOpportunityByID(ctx, opportunityID)
	if err != nil {
		log.Printf("opportunity cancellation lookup failed: %v", err)
		return
	}
	if opp == nil {
		return
	}

	apps, err := s.ListByOpportunity(ctx, opportunityID)
	if err != nil {
		log.Printf("opportunity cancellation applications lookup failed: %v", err)
		return
	}

	for _, app := range apps {
		if app.Status != StatusAccepted {
			continue
		}
		userID, ok := app.UserID.ID.(string)
		if !ok {
			continue
		}
		user, err := s.userSvc.GetByID(ctx, userID)
		if err != nil {
			log.Printf("opportunity cancellation user lookup failed: %v", err)
			continue
		}
		if user == nil || !user.NotificationSettings.OpportunityCanceled {
			continue
		}
		subject := "Opportunity canceled: " + opp.Title
		body := fmt.Sprintf("Hi %s,\n\n%s has been canceled.\n\nDate: %s\nLocation: %s", user.Name, opp.Title, formatNotificationTime(opp.Date), opp.Location)
		if err := s.mailer.SendNotification(ctx, user.Email, subject, body); err != nil {
			log.Printf("send opportunity cancellation notification failed for %s: %v", user.Email, err)
		}
	}
}

func (s *Service) SendDueOpportunityReminders(ctx context.Context, now time.Time) error {
	if s.mailer == nil {
		return nil
	}
	until := now.Add(24 * time.Hour)
	results, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications WHERE status = 'accepted' AND reminder_sent_at = NONE",
		map[string]any{})
	if err != nil {
		return fmt.Errorf("query reminder applications: %w", err)
	}
	if len(*results) == 0 {
		return nil
	}

	for _, app := range (*results)[0].Result {
		oppID, ok := app.OpportunityID.ID.(string)
		if !ok {
			continue
		}
		opp, err := s.getOpportunityByID(ctx, oppID)
		if err != nil {
			log.Printf("reminder opportunity lookup failed: %v", err)
			continue
		}
		if opp == nil || opp.Date.Before(now) || opp.Date.After(until) {
			continue
		}

		userID, ok := app.UserID.ID.(string)
		if !ok {
			continue
		}
		user, err := s.userSvc.GetByID(ctx, userID)
		if err != nil {
			log.Printf("reminder user lookup failed: %v", err)
			continue
		}
		if user == nil || !user.NotificationSettings.OpportunityReminder {
			continue
		}

		subject := "Reminder: " + opp.Title + " is coming up"
		body := fmt.Sprintf("Hi %s,\n\nThis is a reminder that %s is coming up soon.\n\nDate: %s\nLocation: %s", user.Name, opp.Title, formatNotificationTime(opp.Date), opp.Location)
		if err := s.mailer.SendNotification(ctx, user.Email, subject, body); err != nil {
			log.Printf("send opportunity reminder failed for %s: %v", user.Email, err)
			continue
		}

		appID := strings.TrimPrefix(app.ID.String(), "applications:")
		if _, err := surrealdb.Merge[Application](ctx, s.db.Client, models.NewRecordID("applications", appID), map[string]any{
			"reminder_sent_at": now,
			"updated_at":       now,
		}); err != nil {
			log.Printf("mark opportunity reminder sent failed: %v", err)
		}
	}
	return nil
}

func (s *Service) SendApplicantDigests(ctx context.Context, now time.Time) error {
	if s.mailer == nil {
		return nil
	}

	allOrgs, err := s.orgSvc.List(ctx)
	if err != nil {
		return err
	}

	for _, org := range allOrgs {
		settings := orgs.NormalizeNotificationSettings(org.NotificationSettings)
		if !settings.ApplicantDigest {
			continue
		}

		interval := 24 * time.Hour
		if settings.ApplicantDigestFrequency == "weekly" {
			interval = 7 * 24 * time.Hour
		}
		windowStart := now.Add(-interval)
		if org.ApplicantDigestSentAt != nil {
			if org.ApplicantDigestSentAt.Add(interval).After(now) {
				continue
			}
			windowStart = *org.ApplicantDigestSentAt
		}

		apps, err := s.listNewApplicationsForOrg(ctx, org.ID.String(), windowStart, now)
		if err != nil {
			log.Printf("applicant digest lookup failed for %s: %v", org.ID.String(), err)
			continue
		}
		if len(apps) > 0 {
			subject, body := s.buildApplicantDigest(org, apps, windowStart, now)
			if err := s.mailer.SendNotification(ctx, org.Email, subject, body); err != nil {
				log.Printf("send applicant digest failed for %s: %v", org.Email, err)
				continue
			}
		}

		orgID, ok := org.ID.ID.(string)
		if !ok {
			continue
		}
		if _, err := s.orgSvc.Update(ctx, orgID, map[string]any{"applicant_digest_sent_at": now}); err != nil {
			log.Printf("update applicant digest sent time failed for %s: %v", org.ID.String(), err)
		}
	}
	return nil
}

func (s *Service) listNewApplicationsForOrg(ctx context.Context, orgID string, start, end time.Time) ([]OrgApplication, error) {
	orgRecordID, err := models.ParseRecordID(orgID)
	if err != nil {
		return nil, err
	}
	results, err := surrealdb.Query[[]Application](ctx, s.db.Client,
		"SELECT * FROM applications WHERE opportunity_id.org_id = $org_id AND created_at > $start AND created_at <= $end ORDER BY created_at DESC",
		map[string]any{
			"org_id": orgRecordID,
			"start":  start,
			"end":    end,
		})
	if err != nil {
		return nil, err
	}
	if len(*results) == 0 {
		return []OrgApplication{}, nil
	}

	detailed := make([]OrgApplication, 0, len((*results)[0].Result))
	for _, app := range (*results)[0].Result {
		item, err := s.enrichOrgApplication(ctx, app)
		if err != nil {
			return nil, err
		}
		detailed = append(detailed, item)
	}
	return detailed, nil
}

func (s *Service) buildApplicantDigest(org orgs.Org, apps []OrgApplication, start, end time.Time) (string, string) {
	subject := fmt.Sprintf("%d new applicant", len(apps))
	if len(apps) != 1 {
		subject += "s"
	}
	subject += " for your opportunities"

	lines := []string{
		fmt.Sprintf("Hi %s,", org.Name),
		"",
		fmt.Sprintf("You had %d new applicant(s) between %s and %s.", len(apps), formatNotificationTime(start), formatNotificationTime(end)),
		"",
	}
	for _, app := range apps {
		applicant := "Applicant"
		if app.User != nil && strings.TrimSpace(app.User.Name) != "" {
			applicant = app.User.Name
		}
		oppTitle := "Opportunity"
		if app.Opportunity != nil {
			oppTitle = app.Opportunity.Title
		}
		lines = append(lines, fmt.Sprintf("- %s applied to %s on %s", applicant, oppTitle, formatNotificationTime(app.CreatedAt)))
	}
	return subject, strings.Join(lines, "\n")
}

func formatNotificationTime(t time.Time) string {
	return t.UTC().Format("Jan 2, 2006 at 15:04 UTC")
}

func recordIDFor(table, id string) (*models.RecordID, error) {
	if strings.Contains(id, ":") {
		return models.ParseRecordID(id)
	}
	rid := models.NewRecordID(table, id)
	return &rid, nil
}
