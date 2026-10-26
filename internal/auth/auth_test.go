package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "s3cret") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(h, "wrong") {
		t.Fatal("wrong password accepted")
	}
	if CheckPassword(h, "") {
		t.Fatal("empty password accepted")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	secret := RandomSecret(32)
	tok, err := IssueToken(secret, "admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := ValidateToken(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if sub != "admin" {
		t.Fatalf("want admin, got %q", sub)
	}
}

func TestTokenRejectsWrongSecret(t *testing.T) {
	tok, _ := IssueToken(RandomSecret(32), "admin", time.Hour)
	if _, err := ValidateToken(RandomSecret(32), tok); err != ErrBadToken {
		t.Fatalf("forged token must fail, got %v", err)
	}
}

func TestTokenRejectsTamperedPayload(t *testing.T) {
	secret := RandomSecret(32)
	tok, _ := IssueToken(secret, "admin", time.Hour)
	// flip a byte in the payload part
	parts := strings.Split(tok, ".")
	p := []byte(parts[1])
	if p[2] == 'a' {
		p[2] = 'b'
	} else {
		p[2] = 'a'
	}
	parts[1] = string(p)
	if _, err := ValidateToken(secret, strings.Join(parts, ".")); err != ErrBadToken {
		t.Fatalf("tampered token must fail, got %v", err)
	}
}

func TestTokenExpires(t *testing.T) {
	secret := RandomSecret(32)
	tok, _ := IssueToken(secret, "admin", -time.Hour) // already expired
	if _, err := ValidateToken(secret, tok); err != ErrBadToken {
		t.Fatalf("expired token must fail, got %v", err)
	}
}

func TestTokenMalformed(t *testing.T) {
	secret := RandomSecret(32)
	for _, tok := range []string{"", "a", "a.b", "a.b.c.d", "...."} {
		if _, err := ValidateToken(secret, tok); err == nil {
			t.Fatalf("malformed token %q must fail", tok)
		}
	}
}

func TestLoginSuccessAndFailure(t *testing.T) {
	hash, _ := HashPassword("pw1234")
	secret := RandomSecret(32)
	if tok, ok := Login("admin", hash, secret, "admin", "pw1234", time.Hour); !ok || tok == "" {
		t.Fatal("valid login rejected")
	}
	if _, ok := Login("admin", hash, secret, "admin", "bad", time.Hour); ok {
		t.Fatal("bad password accepted")
	}
	if _, ok := Login("admin", hash, secret, "root", "pw1234", time.Hour); ok {
		t.Fatal("bad username accepted")
	}
}

func TestRandomSecretUnique(t *testing.T) {
	a, b := RandomSecret(16), RandomSecret(16)
	if a == b {
		t.Fatal("secrets must differ")
	}
	if len(a) != 32 {
		t.Fatalf("16 bytes hex = 32 chars, got %d", len(a))
	}
}
