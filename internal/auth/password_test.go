package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("expected valid password to verify")
	}
	if VerifyPassword(encoded, "wrong password") {
		t.Fatal("expected invalid password to fail")
	}
}

func TestPasswordLength(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected short password to fail")
	}
}
