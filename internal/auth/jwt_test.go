package auth

import (
	"testing"
	"time"

	"github.com/goosemooz/something-backend/config"
)

func testConfig() *config.Config {
	return &config.Config{
		JWTSecret:       "test-secret-for-unit-tests",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	}
}

func TestGenerateToken_ReturnsToken(t *testing.T) {
	token, err := GenerateToken("users:abc123", testConfig())
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken returned empty token")
	}
}

func TestValidateToken_ValidToken(t *testing.T) {
	cfg := testConfig()
	token, _ := GenerateToken("users:abc123", cfg)

	claims, err := ValidateToken(token, cfg)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if claims.UserID != "users:abc123" {
		t.Errorf("expected UserID 'users:abc123', got '%s'", claims.UserID)
	}
}

func TestValidateToken_OrgToken(t *testing.T) {
	cfg := testConfig()
	token, _ := GenerateToken("orgs:xyz789", cfg)

	claims, err := ValidateToken(token, cfg)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if claims.UserID != "orgs:xyz789" {
		t.Errorf("expected UserID 'orgs:xyz789', got '%s'", claims.UserID)
	}
}

func TestValidateToken_InvalidToken(t *testing.T) {
	_, err := ValidateToken("not.a.valid.token", testConfig())
	if err == nil {
		t.Error("expected error for malformed token, got nil")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, _ := GenerateToken("users:abc123", testConfig())

	wrongCfg := &config.Config{JWTSecret: "completely-different-secret"}
	_, err := ValidateToken(token, wrongCfg)
	if err == nil {
		t.Error("expected error when validating with wrong secret, got nil")
	}
}
