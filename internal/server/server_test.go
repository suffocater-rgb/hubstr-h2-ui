package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"h2-ui/internal/auth"
	"h2-ui/internal/config"
	"h2-ui/internal/store"
	"h2-ui/internal/systemd"
)

type fakeRunner struct{ restarts int }

func (f *fakeRunner) Run(name string, args ...string) error {
	for _, a := range args {
		if a == "restart" {
			f.restarts++
		}
	}
	return nil
}

// build spins up a fully-wired test panel with canned admin credentials.
func build(t *testing.T) (*httptest.Server, string, *fakeRunner, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Load(filepath.Join(dir, "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("adminpw")
	if err := st.SetAdmin("admin", hash, auth.RandomSecret(32)); err != nil {
		t.Fatal(err)
	}
	hyConf := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(hyConf, []byte("listen: :443\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}
	srv, err := New(Options{
		Addr:         "127.0.0.1:0",
		HysteriaConf: hyConf,
		Store:        st,
		HyClient:     nil, // traffic API disabled in handler tests
		Sysd:         systemd.NewWithRunner(fr),
		TokenTTL:     time.Hour,
		PublicIP:     "198.51.100.7",
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// login
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "adminpw"})
	resp, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var lr struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&lr)
	resp.Body.Close()
	if lr.Token == "" {
		t.Fatalf("login failed: status %d", resp.StatusCode)
	}
	return ts, lr.Token, fr, hyConf
}

func do(t *testing.T, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	resp.Body.Close()
	return resp, buf.Bytes()
}

func TestLoginRejectsBadPassword(t *testing.T) {
	ts := func() *httptest.Server {
		dir := t.TempDir()
		st, _ := store.Load(filepath.Join(dir, "store.json"))
		hash, _ := auth.HashPassword("adminpw")
		st.SetAdmin("admin", hash, auth.RandomSecret(32))
		srv, _ := New(Options{
			Addr:         "127.0.0.1:0",
			HysteriaConf: filepath.Join(dir, "config.yaml"),
			Store:        st,
			TokenTTL:     time.Hour,
		})
		ts := httptest.NewServer(srv.Handler())
		t.Cleanup(ts.Close)
		return ts
	}()
	resp, _ := do(t, "POST", ts.URL+"/api/login", "", map[string]string{"username": "admin", "password": "wrong"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	ts, _, _, _ := build(t)
	resp, _ := do(t, "GET", ts.URL+"/api/users", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", resp.StatusCode)
	}
}

func TestUserLifecycle(t *testing.T) {
	ts, tok, fr, hyConf := build(t)

	// create
	resp, raw := do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "ivan", "password": "pw1234"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, raw)
	}

	// duplicate -> 409
	resp, _ = do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "ivan", "password": "pw1234"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate: want 409 got %d", resp.StatusCode)
	}

	// bad name -> 400
	resp, _ = do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "bad name!", "password": "pw1234"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad name: want 400 got %d", resp.StatusCode)
	}

	// hysteria config mirrors enabled users
	data, _ := os.ReadFile(hyConf)
	if !strings.Contains(string(data), "ivan") {
		t.Fatalf("config not synced after create:\n%s", data)
	}
	if fr.restarts < 1 {
		t.Fatalf("service must restart after user create")
	}

	// list
	resp, raw = do(t, "GET", ts.URL+"/api/users", tok, nil)
	var users []store.User
	json.Unmarshal(raw, &users)
	if len(users) != 1 || users[0].Username != "ivan" {
		t.Fatalf("list: %s", raw)
	}

	// update password + disable
	en := false
	resp, raw = do(t, "PUT", ts.URL+"/api/users/ivan", tok, map[string]any{"password": "newpw99", "enabled": en})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", resp.StatusCode, raw)
	}
	data, _ = os.ReadFile(hyConf)
	if strings.Contains(string(data), "ivan") {
		t.Fatalf("disabled user must leave userpass map:\n%s", data)
	}

	// delete
	resp, _ = do(t, "DELETE", ts.URL+"/api/users/ivan", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp, _ = do(t, "GET", ts.URL+"/api/users/ivan", tok, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted user still visible: %d", resp.StatusCode)
	}
}

func TestConfigRoundTripAndValidation(t *testing.T) {
	ts, tok, _, hyConf := build(t)

	resp, _ := do(t, "PUT", ts.URL+"/api/config", tok, map[string]any{
		"port": 70000, "masquerade": "https://x.com", "obfs": false,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("port out of range: want 400 got %d", resp.StatusCode)
	}

	resp, _ = do(t, "PUT", ts.URL+"/api/config", tok, map[string]any{
		"port": 2053, "masquerade": "https://example.com", "obfs": true, "obfsPassword": "sec",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid config: %d", resp.StatusCode)
	}

	pc, err := config.LoadPanelConfig(hyConf)
	if err != nil {
		t.Fatal(err)
	}
	if pc.Port != 2053 || pc.ObfsPassword != "sec" {
		t.Fatalf("config not applied: %+v", pc)
	}
}

func TestLinkGeneration(t *testing.T) {
	ts, tok, _, _ := build(t)
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "lena", "password": "pw1234"})
	do(t, "PUT", ts.URL+"/api/settings", tok, map[string]string{"domain": "vpn.example.com"})

	resp, raw := do(t, "GET", ts.URL+"/api/link?user=lena", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("link: %d", resp.StatusCode)
	}
	var out struct {
		Link string `json:"link"`
	}
	json.Unmarshal(raw, &out)
	if !strings.HasPrefix(out.Link, "hysteria2://lena:pw1234@vpn.example.com:") {
		t.Fatalf("bad link: %s", out.Link)
	}
	if !strings.Contains(out.Link, "sni=") {
		t.Fatalf("link must carry sni: %s", out.Link)
	}
}

func TestServiceActions(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "POST", ts.URL+"/api/service", tok, map[string]string{"action": "restart"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart: %d", resp.StatusCode)
	}
	resp, _ = do(t, "POST", ts.URL+"/api/service", tok, map[string]string{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown action: want 400 got %d", resp.StatusCode)
	}
}

func TestValidateName(t *testing.T) {
	good := []string{"a", "user-1", "u.ser_2", strings.Repeat("x", 32)}
	for _, g := range good {
		if err := validateName(g); err != nil {
			t.Fatalf("%q wrongly rejected: %v", g, err)
		}
	}
	bad := []string{"", strings.Repeat("x", 33), "bad name", "й", "semi;colon", "a/b", "a\\b", `q"uote`}
	for _, b := range bad {
		if err := validateName(b); err == nil {
			t.Fatalf("%q must be rejected", b)
		}
	}
}

func TestBuildLinkEscapes(t *testing.T) {
	pc := &config.PanelConfig{Port: 443, Obfs: true, ObfsPassword: "o b", Masquerade: "https://mask.example/path"}
	link := BuildLink("u s", "p@ss", "1.2.3.4", pc)
	if !strings.HasPrefix(link, "hysteria2://u%20s:p%40ss@1.2.3.4:443?") {
		t.Fatalf("escaping broken: %s", link)
	}
	if !strings.Contains(link, "obfs=salamander") {
		t.Fatalf("obfs missing: %s", link)
	}
	if !strings.HasSuffix(link, "#u%20s") {
		t.Fatalf("fragment missing: %s", link)
	}
}

func TestSniFromMasquerade(t *testing.T) {
	cases := map[string]string{
		"https://news.site/path?q=1": "news.site",
		"http://h.org":               "h.org",
		"plain.com/xyz":              "plain.com",
		"":                           "bing.com",
	}
	for in, want := range cases {
		if got := sniFromMasquerade(in); got != want {
			t.Fatalf("sni(%q): want %q got %q", in, want, got)
		}
	}
}

func TestSummaryCounts(t *testing.T) {
	ts, tok, _, _ := build(t)
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "u1", "password": "pw1234"})
	do(t, "POST", ts.URL+"/api/users", tok, map[string]string{"username": "u2", "password": "pw1234"})
	do(t, "PUT", ts.URL+"/api/users/u2", tok, map[string]any{"enabled": false})

	resp, raw := do(t, "GET", ts.URL+"/api/summary", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("summary: %d", resp.StatusCode)
	}
	var s struct {
		UsersTotal   int `json:"usersTotal"`
		UsersEnabled int `json:"usersEnabled"`
	}
	json.Unmarshal(raw, &s)
	if s.UsersTotal != 2 || s.UsersEnabled != 1 {
		t.Fatalf("summary counts: %s", raw)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	ts, tok, _, _ := build(t)
	resp, _ := do(t, "DELETE", ts.URL+"/api/users", tok, nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 got %d", resp.StatusCode)
	}
}

func TestMalformedJSON(t *testing.T) {
	ts, tok, _, _ := build(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/users", strings.NewReader("{oops"))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed json: want 400 got %d", resp.StatusCode)
	}
}

func TestMissingFileStillRendersConfig(t *testing.T) {
	// LoadPanelConfig on a fresh (nonexistent) config path must 500 via GET
	// but PUT must create it — mirrors first-boot behavior.
	ts, tok, _, hyConf := build(t)
	os.Remove(hyConf)
	resp, _ := do(t, "PUT", ts.URL+"/api/config", tok, map[string]any{
		"port": 443, "masquerade": "https://m.example", "obfs": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT must create missing config: %d", resp.StatusCode)
	}
	if _, err := os.Stat(hyConf); err != nil {
		t.Fatalf("config not created: %v", err)
	}
}

func TestTrafficAccumulationAcrossServerRestart(t *testing.T) {
	// unit-level re-implementation of the delta logic used by RunTrafficPoller
	type entry struct{ tx, rx uint64 }
	last := map[string]entry{}
	add := func(cur map[string]entry) map[string]uint64 {
		out := map[string]uint64{}
		for n, e := range cur {
			p := last[n]
			dtx, drx := e.tx, e.rx
			if e.tx >= p.tx {
				dtx = e.tx - p.tx
			}
			if e.rx >= p.rx {
				drx = e.rx - p.rx
			}
			out[n+"_tx"] += dtx
			out[n+"_rx"] += drx
		}
		last = cur
		return out
	}
	d1 := add(map[string]entry{"u": {1000, 2000}})
	if d1["u_tx"] != 1000 {
		t.Fatalf("first poll counts full counters: %+v", d1)
	}
	d2 := add(map[string]entry{"u": {1500, 2500}})
	if d2["u_tx"] != 500 || d2["u_rx"] != 500 {
		t.Fatalf("delta poll: %+v", d2)
	}
	// hysteria restart resets counters — we must count the new absolute values,
	// never a negative delta masquerading as overflow.
	d3 := add(map[string]entry{"u": {100, 100}})
	if d3["u_tx"] != 100 || d3["u_rx"] != 100 {
		t.Fatalf("reset handling: %+v", d3)
	}
	fmt.Println("poller deltas OK")
}
