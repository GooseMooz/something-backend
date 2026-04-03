// External test package breaks the import cycle:
//
//	applications_test → opportunities → applications (handler imports it)
package applications_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/goosemooz/something-backend/internal/applications"
	"github.com/goosemooz/something-backend/internal/db"
	"github.com/goosemooz/something-backend/internal/opportunities"
	"github.com/goosemooz/something-backend/internal/orgs"
	"github.com/goosemooz/something-backend/internal/testhelper"
	"github.com/goosemooz/something-backend/internal/users"
)

var (
	svc    *applications.Service
	testDB *db.DB
	userID string // full "users:xxx"
	oppID  string // full "opportunities:xxx"
	orgID  string // full "orgs:xxx"
)

func TestMain(m *testing.M) {
	database, err := testhelper.NewDB()
	if err != nil {
		if os.Getenv("CI") != "" {
			fmt.Printf("applications integration tests require db in CI: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("skipping applications integration tests (no db): %v\n", err)
		os.Exit(0)
	}
	svc = applications.NewService(database)
	testDB = database
	ctx := context.Background()

	user, err := users.NewService(database).Create(ctx, "apptest@test.com", "hash", "App Tester")
	if err != nil {
		fmt.Printf("failed to create test user: %v\n", err)
		os.Exit(1)
	}
	userID = user.ID.String()

	org, err := orgs.NewService(database).Create(ctx, "App Org", "hash", "apporg@test.com", "Toronto")
	if err != nil {
		fmt.Printf("failed to create test org: %v\n", err)
		os.Exit(1)
	}
	orgID = org.ID.String()

	// Shared opportunity used by listing tests.
	opp, err := opportunities.NewService(database).Create(ctx, orgID, opportunities.Opportunity{
		Title:       "Shared Test Opportunity",
		Category:    "environment",
		Description: "For testing applications",
		Date:        time.Now().Add(24 * time.Hour),
		Duration:    2,
		Location:    "Toronto",
		MaxSpots:    10,
	})
	if err != nil {
		fmt.Printf("failed to create test opportunity: %v\n", err)
		os.Exit(1)
	}
	oppID = opp.ID.String()

	os.Exit(m.Run())
}

// freshOpp creates a unique opportunity so duplicate-apply checks don't collide.
func freshOpp(t *testing.T, title string) *opportunities.Opportunity {
	t.Helper()
	opp, err := opportunities.NewService(testDB).Create(context.Background(), orgID, opportunities.Opportunity{
		Title:       title,
		Category:    "env",
		Description: "test",
		Date:        time.Now().Add(24 * time.Hour),
		Duration:    1,
		Location:    "Toronto",
		MaxSpots:    5,
	})
	if err != nil {
		t.Fatalf("freshOpp: %v", err)
	}
	return opp
}

func bareID(fullID string) string {
	for i, c := range fullID {
		if c == ':' {
			return fullID[i+1:]
		}
	}
	return fullID
}

func TestCreate(t *testing.T) {
	opp := freshOpp(t, "Opp for TestCreate")
	app, err := svc.Create(context.Background(), userID, opp.ID.String())
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if app.ID.String() == "" {
		t.Error("expected non-empty ID")
	}
	if app.Status != applications.StatusPending {
		t.Errorf("expected status 'pending', got '%s'", app.Status)
	}
	if app.UserID.String() != userID {
		t.Errorf("expected user_id '%s', got '%s'", userID, app.UserID.String())
	}
}

func TestCreate_DuplicateReturnsError(t *testing.T) {
	ctx := context.Background()
	// First apply to the shared opp (may already exist from a prior test).
	svc.Create(ctx, userID, oppID) //nolint:errcheck
	// Second apply must always be rejected.
	_, err := svc.Create(ctx, userID, oppID)
	if !errors.Is(err, applications.ErrAlreadyApplied) {
		t.Errorf("expected ErrAlreadyApplied on duplicate, got %v", err)
	}
}

func TestListByUser(t *testing.T) {
	svc.Create(context.Background(), userID, freshOpp(t, "Opp for ListByUser").ID.String()) //nolint:errcheck

	apps, err := svc.ListByUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(apps) == 0 {
		t.Error("expected at least one application")
	}
	for _, a := range apps {
		if a.UserID.String() != userID {
			t.Errorf("ListByUser returned application from wrong user: %s", a.UserID.String())
		}
	}
}

func TestListByOrg(t *testing.T) {
	ctx := context.Background()
	opp := freshOpp(t, "Opp for ListByOrg")
	app, err := svc.Create(ctx, userID, opp.ID.String())
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	apps, err := svc.ListByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("ListByOrg failed: %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("expected at least one application")
	}

	found := false
	for _, candidate := range apps {
		if candidate.ID.String() == app.ID.String() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find application %s in org listing", app.ID.String())
	}
}

func TestUpdateStatus_Accept(t *testing.T) {
	ctx := context.Background()
	opp := freshOpp(t, "Opp for Accept")
	app, err := svc.Create(ctx, userID, opp.ID.String())
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated, err := svc.UpdateStatus(ctx, bareID(app.ID.String()), orgID, applications.StatusAccepted)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if updated.Status != applications.StatusAccepted {
		t.Errorf("expected status 'accepted', got '%s'", updated.Status)
	}

	reloadedOpp, err := opportunities.NewService(testDB).GetByID(ctx, bareID(opp.ID.String()))
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if reloadedOpp.SpotsLeft != opp.MaxSpots-1 {
		t.Errorf("expected spots_left %d, got %d", opp.MaxSpots-1, reloadedOpp.SpotsLeft)
	}
}

func TestUpdateStatus_WrongOrg(t *testing.T) {
	ctx := context.Background()
	app, err := svc.Create(ctx, userID, freshOpp(t, "Opp for WrongOrg").ID.String())
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err = svc.UpdateStatus(ctx, bareID(app.ID.String()), "orgs:doesnotexist", applications.StatusAccepted)
	if !errors.Is(err, applications.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestDelete_Withdraw(t *testing.T) {
	ctx := context.Background()
	app, err := svc.Create(ctx, userID, freshOpp(t, "Opp for Withdraw").ID.String())
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.Delete(ctx, bareID(app.ID.String()), userID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestDelete_WrongUser(t *testing.T) {
	ctx := context.Background()
	app, err := svc.Create(ctx, userID, freshOpp(t, "Opp for WrongUser").ID.String())
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = svc.Delete(ctx, bareID(app.ID.String()), "users:doesnotexist")
	if !errors.Is(err, applications.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateStatus_NoSpotsLeft(t *testing.T) {
	ctx := context.Background()
	oppSvc := opportunities.NewService(testDB)
	opp, err := oppSvc.Create(ctx, orgID, opportunities.Opportunity{
		Title:       "Opp With One Spot",
		Category:    "environment",
		Description: "Only one acceptance allowed",
		Date:        time.Now().Add(24 * time.Hour),
		Duration:    1,
		Location:    "Toronto",
		MaxSpots:    1,
	})
	if err != nil {
		t.Fatalf("Create opportunity failed: %v", err)
	}

	firstApp, err := svc.Create(ctx, userID, opp.ID.String())
	if err != nil {
		t.Fatalf("Create first application failed: %v", err)
	}

	secondUser, err := users.NewService(testDB).Create(ctx, "second-applicant@test.com", "hash", "Second Applicant")
	if err != nil {
		t.Fatalf("Create second user failed: %v", err)
	}
	secondApp, err := svc.Create(ctx, secondUser.ID.String(), opp.ID.String())
	if err != nil {
		t.Fatalf("Create second application failed: %v", err)
	}

	if _, err := svc.UpdateStatus(ctx, bareID(firstApp.ID.String()), orgID, applications.StatusAccepted); err != nil {
		t.Fatalf("First acceptance failed: %v", err)
	}

	_, err = svc.UpdateStatus(ctx, bareID(secondApp.ID.String()), orgID, applications.StatusAccepted)
	if !errors.Is(err, applications.ErrNoSpotsLeft) {
		t.Fatalf("expected ErrNoSpotsLeft, got %v", err)
	}
}

func TestListByUser_AwardsXPForPastAcceptedOpportunity(t *testing.T) {
	ctx := context.Background()
	oppSvc := opportunities.NewService(testDB)
	pastOpp, err := oppSvc.Create(ctx, orgID, opportunities.Opportunity{
		Title:       "Past Accepted Opportunity",
		Category:    "environment",
		Difficulty:  opportunities.DifficultyHard,
		Description: "Eligible for XP",
		Date:        time.Now().Add(-2 * time.Hour),
		Duration:    2,
		Location:    "Toronto",
		MaxSpots:    3,
	})
	if err != nil {
		t.Fatalf("Create opportunity failed: %v", err)
	}

	app, err := svc.Create(ctx, userID, pastOpp.ID.String())
	if err != nil {
		t.Fatalf("Create application failed: %v", err)
	}
	if _, err := svc.UpdateStatus(ctx, bareID(app.ID.String()), orgID, applications.StatusAccepted); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	if _, err := svc.ListByUser(ctx, userID); err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}

	user, err := users.NewService(testDB).GetByID(ctx, bareID(userID))
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if user.XP != 300 {
		t.Fatalf("expected XP 300, got %d", user.XP)
	}

	apps, err := svc.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	foundAwarded := false
	for _, candidate := range apps {
		if candidate.ID.String() == app.ID.String() {
			foundAwarded = candidate.XPAwarded
			break
		}
	}
	if !foundAwarded {
		t.Fatal("expected xp_awarded to be true after XP reconciliation")
	}
}
