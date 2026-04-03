package users

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
			fmt.Printf("users integration tests require db in CI: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("skipping users integration tests (no db): %v\n", err)
		os.Exit(0)
	}
	svc = NewService(database)
	os.Exit(m.Run())
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	user, err := svc.Create(ctx, "create@test.com", "hash", "Alice")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if user.ID.String() == "" {
		t.Error("expected non-empty ID")
	}
	if user.Email != "create@test.com" {
		t.Errorf("expected email 'create@test.com', got '%s'", user.Email)
	}
	if user.Name != "Alice" {
		t.Errorf("expected name 'Alice', got '%s'", user.Name)
	}
}

func TestGetByEmail_Found(t *testing.T) {
	ctx := context.Background()
	_, _ = svc.Create(ctx, "getbyemail@test.com", "hash", "Bob")

	user, err := svc.GetByEmail(ctx, "getbyemail@test.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Email != "getbyemail@test.com" {
		t.Errorf("expected email 'getbyemail@test.com', got '%s'", user.Email)
	}
}

func TestGetByEmail_NotFound(t *testing.T) {
	user, err := svc.GetByEmail(context.Background(), "nobody@nowhere.com")
	if err != nil {
		t.Fatalf("GetByEmail returned unexpected error: %v", err)
	}
	if user != nil {
		t.Errorf("expected nil for missing user, got %+v", user)
	}
}

func TestGetByID(t *testing.T) {
	ctx := context.Background()
	created, _ := svc.Create(ctx, "getbyid@test.com", "hash", "Carol")

	bareID := created.ID.ID.(string)
	found, err := svc.GetByID(ctx, bareID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected user, got nil")
	}
	if found.Email != "getbyid@test.com" {
		t.Errorf("expected email 'getbyid@test.com', got '%s'", found.Email)
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	created, _ := svc.Create(ctx, "update@test.com", "hash", "Dave")

	bareID := created.ID.ID.(string)
	updated, err := svc.Update(ctx, bareID, map[string]any{"name": "David"})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "David" {
		t.Errorf("expected name 'David', got '%s'", updated.Name)
	}
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Error("expected updated_at to stay the same or advance after Update")
	}
}
