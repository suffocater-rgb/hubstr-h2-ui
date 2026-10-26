package server

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// Close the remaining coverage holes on handlers — pure HTTP-level tests
// against the fully wired test panel.

func TestMiddlewareEnforced(t *testing.T) {
	ts, tok, _, _ := build(t)

	// no Authorization header
	resp, err := http.Get(ts.URL + "/api/users")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no header: want 401 got %d", resp.StatusCode)
	}

	// garbage token
	req, _ := http.NewRequest("GET", ts.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer garbage.token.here")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("garbage token: want 401 got %d", resp.StatusCode)
	}

	// valid token
	req, _ = http.NewRequest("GET", ts.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid token: want 200 got %d", resp.StatusCode)
	}
}

func TestConfigGetOnCorruptFile(t *testing.T) {
	ts, tok, _, hyConf := build(t)
	if err := os.WriteFile(hyConf, []byte(": : :"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, _ := do(t, "GET", ts.URL+"/api/config", tok, nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("broken yaml: want 500 got %d", resp.StatusCode)
	}
}

func TestServiceAllActions(t *testing.T) {
	ts, tok, _, _ := build(t)
	for _, a := range []string{"start", "stop", "restart"} {
		resp, _ := do(t, "POST", ts.URL+"/api/service", tok, map[string]string{"action": a})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("action %s: want 200 got %d", a, resp.StatusCode)
		}
	}
}

func TestGetSingleUser(t *testing.T) {
	ts, tok, _, _ := build(t)
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "solo", "password": "pw1234"})
	resp, raw := do(t, "GET", ts.URL+"/api/users/solo", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get single: %d", resp.StatusCode)
	}
	if !strings.Contains(string(raw), `"solo"`) {
		t.Fatalf("get single: %s", raw)
	}
}

func TestSyncAuthSurvivesBrokenConfig(t *testing.T) {
	ts, tok, _, hyConf := build(t)
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "iff", "password": "pw1234"})
	if err := os.WriteFile(hyConf, []byte(":\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// create still succeeds; syncAuth logs and swallows the config error
	resp, _ := do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "iff2", "password": "pw1234"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create after corrupt config: %d", resp.StatusCode)
	}
}

func TestUsersBadJSONArray(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "POST", ts.URL+"/api/users", tok, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body: want 400 got %d", resp.StatusCode)
	}
}
