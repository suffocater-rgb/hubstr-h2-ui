// Package auth provides bcrypt password hashing, a minimal HMAC-SHA256 JWT
// (HS256) implementation and the login/middleware logic for the panel.
//
// We hand-roll the JWT to keep the binary dependency-free: header.payload.sig
// with exp/iat claims is all we need for a session token.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword bcrypts a password.
func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

// CheckPassword verifies a bcrypt hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// RandomSecret returns n random bytes hex-encoded.
func RandomSecret(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// --- minimal HS256 JWT -------------------------------------------------

type claims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

var b64 = base64.RawURLEncoding

// IssueToken signs a token for subject valid for ttl.
func IssueToken(secret, subject string, ttl time.Duration) (string, error) {
	head := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	c := claims{Sub: subject, Iat: time.Now().Unix(), Exp: time.Now().Add(ttl).Unix()}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	body := head + "." + b64.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return body + "." + b64.EncodeToString(mac.Sum(nil)), nil
}

// ErrBadToken is returned for malformed/forged/expired tokens.
var ErrBadToken = errors.New("invalid token")

// ValidateToken checks signature and expiry; returns the subject.
func ValidateToken(secret, token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrBadToken
	}
	body := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	expected := b64.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return "", ErrBadToken
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return "", ErrBadToken
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return "", ErrBadToken
	}
	if time.Now().Unix() > c.Exp {
		return "", ErrBadToken
	}
	if c.Sub == "" {
		return "", ErrBadToken
	}
	return c.Sub, nil
}

// Middleware protects a handler with the Bearer token.
func Middleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if _, err := ValidateToken(secret, strings.TrimPrefix(h, "Bearer ")); err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Login validates user+password and issues a token on success.
// Returns "" and false on failure (no oracle about which part failed).
func Login(storeUser, storedHash, jwtSecret, user, password string, ttl time.Duration) (string, bool) {
	if user != storeUser || !CheckPassword(storedHash, password) {
		return "", false
	}
	tok, err := IssueToken(jwtSecret, user, ttl)
	if err != nil {
		panic(fmt.Errorf("issue token: %w", err)) // cannot fail on random data
	}
	return tok, true
}
