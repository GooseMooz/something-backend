package auth

import "testing"

func TestHashPassword_ProducesHash(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty hash")
	}
	if hash == "secret123" {
		t.Fatal("hash must not equal the plain-text password")
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	hash, _ := HashPassword("secret123")
	if !CheckPassword(hash, "secret123") {
		t.Error("CheckPassword returned false for correct password")
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("secret123")
	if CheckPassword(hash, "wrongpassword") {
		t.Error("CheckPassword returned true for wrong password")
	}
}

func TestHashPassword_UniqueHashes(t *testing.T) {
	hash1, _ := HashPassword("same-password")
	hash2, _ := HashPassword("same-password")
	if hash1 == hash2 {
		t.Error("two hashes of the same password should differ (bcrypt uses random salt)")
	}
}
