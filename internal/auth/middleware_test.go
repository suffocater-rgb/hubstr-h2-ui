package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Middleware is exercised end-to-end through the httptest server so coverage
// counts it without needing a running panel.
func TestMiddlewareRejectsMissingHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	h := Middleware("secret", next)

	req := httptest.NewRequest("GET", "/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without header, got %d", rr.Code)
	}
}

func TestMiddlewareRejectsBadScheme(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := Middleware("secret", next)

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Basic abc")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for Basic scheme, got %d", rr.Code)
	}
}

func TestMiddlewareRejectsBadToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := Middleware("secret", next)

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer old.bad.token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for bad token, got %d", rr.Code)
	}
}

func TestMiddlewarePassesValidToken(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(204)
	})
	h := Middleware("secret", next)

	tok, err := IssueToken("secret", "admin", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent || !called {
		t.Fatalf("valid token must call next, code=%d called=%v", rr.Code, called)
	}
}
