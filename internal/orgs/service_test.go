package orgs

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/goosemooz/something-backend/internal/testhelper"
)

var svc *Service

func TestMain(m *testing.M) {
	database, err := testhelper.NewDB()
	if err != nil {
		if os.Getenv("CI") != "" {
			fmt.Printf("orgs integration tests require db in CI: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("skipping orgs integration tests (no db): %v\n", err)
		os.Exit(0)
	}
	svc = NewService(database)
	os.Exit(m.Run())
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	org, err := svc.Create(ctx, "Green Leaf", "hash", "gl@test.com", "Toronto")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if org.ID.String() == "" {
		t.Error("expected non-empty ID")
	}
	if org.Email != "gl@test.com" {
		t.Errorf("expected email 'gl@test.com', got '%s'", org.Email)
	}
	if org.Location != "Toronto" {
		t.Errorf("expected location 'Toronto', got '%s'", org.Location)
	}
}

func TestGetByEmail_Found(t *testing.T) {
	ctx := context.Background()
	_, _ = svc.Create(ctx, "River Org", "hash", "river@test.com", "Vancouver")

	org, err := svc.GetByEmail(ctx, "river@test.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	if org == nil {
		t.Fatal("expected org, got nil")
	}
	if org.Name != "River Org" {
		t.Errorf("expected name 'River Org', got '%s'", org.Name)
	}
}

func TestGetByEmail_NotFound(t *testing.T) {
	org, err := svc.GetByEmail(context.Background(), "ghost@nowhere.com")
	if err != nil {
		t.Fatalf("GetByEmail returned unexpected error: %v", err)
	}
	if org != nil {
		t.Errorf("expected nil for missing org, got %+v", org)
	}
}

func TestGetByID(t *testing.T) {
	ctx := context.Background()
	created, _ := svc.Create(ctx, "Forest Org", "hash", "forest@test.com", "Ottawa")

	bareID := created.ID.ID.(string)
	found, err := svc.GetByID(ctx, bareID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected org, got nil")
	}
	if found.Email != "forest@test.com" {
		t.Errorf("expected email 'forest@test.com', got '%s'", found.Email)
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	created, _ := svc.Create(ctx, "Old Name", "hash", "update@test.com", "Calgary")

	bareID := created.ID.ID.(string)
	updated, err := svc.Update(ctx, bareID, map[string]any{
		"description": "We plant trees",
		"categories":  []string{"environment"},
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Description != "We plant trees" {
		t.Errorf("expected description 'We plant trees', got '%s'", updated.Description)
	}
}
