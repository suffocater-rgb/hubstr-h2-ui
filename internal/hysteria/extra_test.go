package hysteria

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKickErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	c := NewClient(ts.URL, "")
	if err := c.Kick(context.Background(), []string{"u"}); err == nil {
		t.Fatal("403 must surface an error")
	}
}

func TestOnlineErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	c := NewClient(ts.URL, "bad")
	if _, err := c.Online(context.Background()); err == nil {
		t.Fatal("401 must surface an error")
	}
}

func TestTrafficServerDown(t *testing.T) {
	// nothing listens here
	c := NewClient("http://127.0.0.1:1", "")
	_, err := c.Traffic(context.Background())
	if err == nil {
		t.Fatal("connection refused must surface an error")
	}
	if !strings.Contains(err.Error(), "connection refused") && !strings.Contains(err.Error(), "connect") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestKickNetworkError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "")
	if err := c.Kick(context.Background(), []string{"u"}); err == nil {
		t.Fatal("kick against dead server must error")
	}
}

func TestBaseURLTrailingSlash(t *testing.T) {
	hit := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path == "/traffic"
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	c := NewClient(ts.URL+"/", "") // trailing slash must be trimmed
	if _, err := c.Traffic(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("path mangled: got double slash or none")
	}
}

func TestAuthorizationHeaderSent(t *testing.T) {
	got := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	c := NewClient(ts.URL, "topsecret")
	c.Traffic(context.Background())
	if got != "topsecret" {
		t.Fatalf("secret header: %q", got)
	}
}
