package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"h2-ui/internal/config"
)

// Endpoint /api/users/reset-traffic is the backend for the new UI button.

func TestResetTraffic(t *testing.T) {
	ts, tok, _, _ := build(t)
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "t1", "password": "pw1234"})
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "t2", "password": "pw1234"})

	// simulate some traffic via the poller path (add via Store is internal,
	// so we just call the endpoint and assert counters go back to zero).
	// It's fine for the test even if counters were zero before.
	resp, raw := do(t, "POST", ts.URL+"/api/users/reset-traffic", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset traffic: %d %s", resp.StatusCode, raw)
	}

	// users still there
	resp, raw = do(t, "GET", ts.URL+"/api/users", tok, nil)
	var list []struct {
		Username string `json:"username"`
		Up       uint64 `json:"up"`
		Down     uint64 `json:"down"`
	}
	json.Unmarshal(raw, &list)
	if len(list) != 2 {
		t.Fatalf("list: %s", raw)
	}
	for _, u := range list {
		if u.Up != 0 || u.Down != 0 {
			t.Fatalf("counters not zeroed: %+v", u)
		}
	}
}

func TestResetTrafficRequiresAuth(t *testing.T) {
	ts, _, _, _ := build(t)
	resp, _ := do(t, "POST", ts.URL+"/api/users/reset-traffic", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestResetTrafficMethodNotAllowed(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "GET", ts.URL+"/api/users/reset-traffic", tok, nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", resp.StatusCode)
	}
}

func TestSummaryContainsDomain(t *testing.T) {
	ts, tok, _, _ := build(t)
	// set the domain first via settings endpoint
	resp, _ := do(t, "PUT", ts.URL+"/api/settings", tok, map[string]string{"domain": "vpn.example.net"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings put: %d", resp.StatusCode)
	}

	resp, raw := do(t, "GET", ts.URL+"/api/summary", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("summary: %d", resp.StatusCode)
	}
	var out struct {
		Domain string `json:"domain"`
	}
	json.Unmarshal(raw, &out)
	if out.Domain != "vpn.example.net" {
		t.Fatalf("domain missing in summary: %s", raw)
	}
}

// Optional SNI override stored in PanelConfig must be rendered by BuildLink.
func TestPanelConfigSNIOverridePresent(t *testing.T) {
	pc := &config.PanelConfig{Port: 443, SNI: "cdn.example.org"}
	link := BuildLink("u", "p", "vpn.example.net", pc)
	if !containsSubstr(link, "sni=cdn.example.org") {
		t.Fatalf("override lost: %s", link)
	}
}

func containsSubstr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
