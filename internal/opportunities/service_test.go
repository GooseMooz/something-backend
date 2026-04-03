package opportunities

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/goosemooz/something-backend/internal/orgs"
	"github.com/goosemooz/something-backend/internal/testhelper"
)

var (
	svc   *Service
	orgID string // full "orgs:xxx" ID shared across tests
)

func TestMain(m *testing.M) {
	database, err := testhelper.NewDB()
	if err != nil {
		if os.Getenv("CI") != "" {
			fmt.Printf("opportunities integration tests require db in CI: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("skipping opportunities integration tests (no db): %v\n", err)
		os.Exit(0)
	}
	svc = NewService(database)

	// Create one org that all tests in this package share.
	orgSvc := orgs.NewService(database)
	org, err := orgSvc.Create(context.Background(), "Test Org", "hash", "opptest@test.com", "Toronto")
	if err != nil {
		fmt.Printf("failed to create test org: %v\n", err)
		os.Exit(1)
	}
	orgID = org.ID.String()

	os.Exit(m.Run())
}

func seedOpp(t *testing.T, title string) *Opportunity {
	t.Helper()
	opp, err := svc.Create(context.Background(), orgID, Opportunity{
		Title:       title,
		Category:    "environment",
		Description: "Test description",
		Date:        time.Now().Add(24 * time.Hour),
		Duration:    2,
		Location:    "Toronto",
		MaxSpots:    10,
	})
	if err != nil {
		t.Fatalf("seedOpp: Create failed: %v", err)
	}
	return opp
}

func TestCreate(t *testing.T) {
	opp := seedOpp(t, "Park Cleanup")
	if opp.ID.String() == "" {
		t.Error("expected non-empty ID")
	}
	if opp.OrgID.String() != orgID {
		t.Errorf("expected org_id '%s', got '%s'", orgID, opp.OrgID.String())
	}
	if len(opp.Tags) != 0 {
		t.Errorf("expected empty tags slice, got %v", opp.Tags)
	}
	if opp.SpotsLeft != opp.MaxSpots {
		t.Errorf("expected spots_left %d, got %d", opp.MaxSpots, opp.SpotsLeft)
	}
}

func TestList(t *testing.T) {
	seedOpp(t, "List Test Opp")

	opps, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(opps) == 0 {
		t.Error("expected at least one opportunity")
	}
}

func TestGetByID(t *testing.T) {
	created := seedOpp(t, "Get By ID Opp")
	id := bareID(t, created.ID.String())

	found, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected opportunity, got nil")
	}
	if found.Title != "Get By ID Opp" {
		t.Errorf("expected title 'Get By ID Opp', got '%s'", found.Title)
	}
}

func TestListByOrg(t *testing.T) {
	seedOpp(t, "Org Filter Opp")

	orgBareID := bareID(t, orgID)
	opps, err := svc.ListByOrg(context.Background(), orgBareID)
	if err != nil {
		t.Fatalf("ListByOrg failed: %v", err)
	}
	if len(opps) == 0 {
		t.Error("expected at least one opportunity for this org")
	}
	for _, o := range opps {
		if o.OrgID.String() != orgID {
			t.Errorf("ListByOrg returned opp from wrong org: %s", o.OrgID.String())
		}
	}
}

func TestDelete_Owner(t *testing.T) {
	ctx := context.Background()
	created := seedOpp(t, "Delete Me")
	id := bareID(t, created.ID.String())

	if err := svc.Delete(ctx, id, orgID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	found, _ := svc.GetByID(ctx, id)
	if found != nil {
		t.Error("expected nil after deletion")
	}
}

func TestUpdate_Owner(t *testing.T) {
	ctx := context.Background()
	created := seedOpp(t, "Update Me")
	id := bareID(t, created.ID.String())

	updated, err := svc.Update(ctx, id, orgID, map[string]any{
		"title":     "Updated Title",
		"duration":  3.5,
		"max_spots": 12,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("expected updated title, got %q", updated.Title)
	}
	if updated.Duration != 3.5 {
		t.Errorf("expected duration 3.5, got %v", updated.Duration)
	}
	if updated.MaxSpots != 12 || updated.SpotsLeft != 12 {
		t.Errorf("expected max_spots/spots_left 12, got %d/%d", updated.MaxSpots, updated.SpotsLeft)
	}
}

func TestDelete_NonOwner(t *testing.T) {
	created := seedOpp(t, "Should Not Delete")
	id := bareID(t, created.ID.String())

	err := svc.Delete(context.Background(), id, "orgs:doesnotexist")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	err := svc.Delete(context.Background(), "nonexistentid", orgID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// bareID strips the "table:" prefix from a full record ID string.
func bareID(t *testing.T, fullID string) string {
	t.Helper()
	for i, c := range fullID {
		if c == ':' {
			return fullID[i+1:]
		}
	}
	t.Fatalf("could not parse bare ID from %q", fullID)
	return ""
}
