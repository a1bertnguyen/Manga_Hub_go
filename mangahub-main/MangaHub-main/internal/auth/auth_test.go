package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func TestHashPasswordAndCheckPassword(t *testing.T) {
	password := "superSecret123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" || hash == password {
		t.Fatalf("HashPassword returned invalid hash: %s", hash)
	}

	// Correct password must match
	if !CheckPassword(password, hash) {
		t.Errorf("CheckPassword returned false for correct password")
	}

	// Wrong password must fail
	if CheckPassword("wrongPassword", hash) {
		t.Errorf("CheckPassword returned true for wrong password")
	}
}

func TestGenerateAndValidateToken(t *testing.T) {
	userID := "usr_test123"
	username := "alice"

	tokenStr, err := GenerateToken(userID, username)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("GenerateToken returned empty token string")
	}

	claims, err := ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("Expected UserID %q, got %q", userID, claims.UserID)
	}
	if claims.Username != username {
		t.Errorf("Expected Username %q, got %q", username, claims.Username)
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Before(time.Now()) {
		t.Errorf("Token has already expired or missing expiration")
	}
}

func TestValidateInvalidToken(t *testing.T) {
	// Completely malformed token
	if _, err := ValidateToken("not.a.valid.jwt"); err == nil {
		t.Error("Expected error for malformed token, got nil")
	}

	// Tampered token signed with different secret
	tamperedClaims := Claims{
		UserID:   "usr_hacker",
		Username: "evil",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	foreignToken := jwt.NewWithClaims(jwt.SigningMethodHS256, tamperedClaims)
	foreignStr, err := foreignToken.SignedString([]byte("wrong-foreign-secret-key-12345"))
	if err != nil {
		t.Fatalf("Failed to sign foreign token: %v", err)
	}

	if _, err := ValidateToken(foreignStr); err == nil {
		t.Error("Expected validation failure for token signed with wrong secret")
	}
}

func TestGenerateID(t *testing.T) {
	prefix := "usr"
	id1 := GenerateID(prefix)
	id2 := GenerateID(prefix)

	if !strings.HasPrefix(id1, "usr_") {
		t.Errorf("Expected prefix 'usr_', got %s", id1)
	}
	if id1 == id2 {
		t.Errorf("GenerateID produced duplicate IDs: %s == %s", id1, id2)
	}
}

