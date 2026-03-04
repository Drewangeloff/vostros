package auth

import (
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("mypassword123")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}

	ok, err := VerifyPassword("mypassword123", hash)
	if err != nil {
		t.Fatalf("VerifyPassword error: %v", err)
	}
	if !ok {
		t.Fatal("password should verify")
	}

	ok, err = VerifyPassword("wrongpassword", hash)
	if err != nil {
		t.Fatalf("VerifyPassword error: %v", err)
	}
	if ok {
		t.Fatal("wrong password should not verify")
	}
}

func TestCreateAndValidateToken(t *testing.T) {
	svc := NewService("test-secret-key")

	token, err := svc.CreateAccessToken("user123", "alice", "user")
	if err != nil {
		t.Fatalf("CreateAccessToken error: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}
	if claims.UserID != "user123" {
		t.Fatalf("expected user123, got %s", claims.UserID)
	}
	if claims.Username != "alice" {
		t.Fatalf("expected alice, got %s", claims.Username)
	}
	if claims.Role != "user" {
		t.Fatalf("expected user, got %s", claims.Role)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	svc1 := NewService("secret-1")
	svc2 := NewService("secret-2")

	token, _ := svc1.CreateAccessToken("user1", "bob", "user")
	_, err := svc2.ValidateToken(token)
	if err == nil {
		t.Fatal("token signed with different secret should fail validation")
	}
}

func TestValidateToken_GarbageInput(t *testing.T) {
	svc := NewService("secret")
	_, err := svc.ValidateToken("not-a-jwt")
	if err == nil {
		t.Fatal("garbage token should fail validation")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	t1, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}
	t2, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}
	if t1 == t2 {
		t.Fatal("refresh tokens should be unique")
	}
}

func TestNewULID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewULID()
		if ids[id] {
			t.Fatalf("duplicate ULID: %s", id)
		}
		ids[id] = true
	}
}
