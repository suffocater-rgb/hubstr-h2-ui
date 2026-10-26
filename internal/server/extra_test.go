package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// These tests close the weak spots the first coverage run exposed:
// kick endpoint, settings round trip, logs error path, SPA handler.

func TestKickRequiresAuthAndUser(t *testing.T) {
	ts, tok, _, _ := build(t)

	// no auth
	resp, _ := do(t, "POST", ts.URL+"/api/users/ghost/kick", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("kick unauth: want 401 got %d", resp.StatusCode)
	}

	// unknown user
	resp, _ = do(t, "POST", ts.URL+"/api/users/ghost/kick", tok, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("kick unknown user: want 404 got %d", resp.StatusCode)
	}

	// wrong method
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "k1", "password": "pw1234"})
	resp, _ = do(t, "GET", ts.URL+"/api/users/k1/kick", tok, nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("kick GET: want 405 got %d", resp.StatusCode)
	}

	// HyClient disabled -> 502 (traffic api disabled)
	resp, _ = do(t, "POST", ts.URL+"/api/users/k1/kick", tok, nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("kick without traffic api: want 502 got %d", resp.StatusCode)
	}
}

func TestSettingsValidationAndRoundTrip(t *testing.T) {
	ts, tok, _, _ := build(t)

	// malformed json
	req, _ := http.NewRequest("PUT", ts.URL+"/api/settings", strings.NewReader("{nope"))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("settings bad json: want 400 got %d", resp.StatusCode)
	}

	// round trip
	resp, _ = do(t, "PUT", ts.URL+"/api/settings", tok, map[string]string{"domain": "panel.example.org"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings put: %d", resp.StatusCode)
	}
	resp, raw := do(t, "GET", ts.URL+"/api/settings", tok, nil)
	var out struct {
		Domain string `json:"domain"`
	}
	json.Unmarshal(raw, &out)
	if out.Domain != "panel.example.org" {
		t.Fatalf("settings get: %s", raw)
	}
}

func TestLogsWithoutSystemd(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "GET", ts.URL+"/api/logs?n=5", tok, nil)
	// in the sandbox journalctl exists but has no hysteria unit — either
	// 200 with empty lines or a 502; both are acceptable, panic is not.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("logs: unexpected %d", resp.StatusCode)
	}

	// n=0 must not crash (default applies)
	resp, _ = do(t, "GET", ts.URL+"/api/logs?n=0", tok, nil)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("logs n=0: unexpected %d", resp.StatusCode)
	}
}

func TestSPAFallback(t *testing.T) {
	ts, _, _, _ := build(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index: %d", resp.StatusCode)
	}
	if !strings.Contains(buf.String(), "<div id=\"app\">") {
		t.Fatalf("index body unexpected: %s", buf.String())
	}

	// deep link must also serve the SPA
	resp, err = http.Get(ts.URL + "/users")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spa fallback: %d", resp.StatusCode)
	}
}

func TestConfigPUTRequiresObfsPassword(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "PUT", ts.URL+"/api/config", tok, map[string]any{
		"port": 443, "obfs": true, "obfsPassword": "",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("obfs without password: want 400 got %d", resp.StatusCode)
	}
}

func TestUserPasswordTooShort(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "sh", "password": "abc"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password: want 400 got %d", resp.StatusCode)
	}
}

func TestLinkForUnknownUser(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "GET", ts.URL+"/api/link?user=ghost", tok, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("link ghost: want 404 got %d", resp.StatusCode)
	}
}

func TestUsernameBoundary32(t *testing.T) {
	ts, tok, _, _ := build(t)
	okName := strings.Repeat("x", 32)
	resp, _ := do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": okName, "password": "pw1234"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("32-char name must pass: %d", resp.StatusCode)
	}
	resp, _ = do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": strings.Repeat("y", 33), "password": "pw1234"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("33-char name must fail: %d", resp.StatusCode)
	}
}
