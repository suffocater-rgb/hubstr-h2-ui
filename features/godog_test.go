// Godog BDD suite driving the real HTTP server + real config/store,
// with only systemctl and the traffic API faked.
package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"h2-ui/internal/auth"
	"h2-ui/internal/server"
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

type world struct {
	ts       *httptest.Server
	token    string
	hyConf   string
	fr       *fakeRunner
	st       *store.Store
	lastCode int
	lastBody []byte
}

func (w *world) reset(t *testing.T) error {
	dir := t.TempDir()
	st, err := store.Load(filepath.Join(dir, "store.json"))
	if err != nil {
		return err
	}
	hash, _ := auth.HashPassword("adminpw")
	if err := st.SetAdmin("admin", hash, auth.RandomSecret(32)); err != nil {
		return err
	}
	hyConf := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(hyConf, []byte("listen: :443\n"), 0o600); err != nil {
		return err
	}
	fr := &fakeRunner{}
	srv, err := server.New(server.Options{
		Addr:         "127.0.0.1:0",
		HysteriaConf: hyConf,
		Store:        st,
		Sysd:         systemd.NewWithRunner(fr),
		TokenTTL:     time.Hour,
		PublicIP:     "198.51.100.7",
	})
	if err != nil {
		return err
	}
	w.ts = httptest.NewServer(srv.Handler())
	w.token = ""
	w.hyConf = hyConf
	w.fr = fr
	w.st = st
	w.lastCode = 0
	w.lastBody = nil
	return nil
}

func (w *world) do(method, path string, body any) error {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(method, w.ts.URL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	w.lastCode = resp.StatusCode
	w.lastBody = buf.Bytes()
	return nil
}

func (w *world) login(user, pw string) error {
	var lr struct {
		Token string `json:"token"`
	}
	if err := w.do("POST", "/api/login", map[string]string{"username": user, "password": pw}); err != nil {
		return err
	}
	json.Unmarshal(w.lastBody, &lr)
	w.token = lr.Token
	return nil
}

func (w *world) userpassMap() map[string]string {
	raw, _ := os.ReadFile(w.hyConf)
	out := map[string]string{}
	lines := strings.Split(string(raw), "\n")
	inUP := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "userpass:" {
			inUP = true
			continue
		}
		if inUP {
			if !strings.HasPrefix(l, "        ") && !strings.HasPrefix(l, "      ") {
				inUP = false
				continue
			}
			kv := strings.SplitN(t, ":", 2)
			if len(kv) == 2 {
				out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}
	return out
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			w := &world{}

			sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
				if err := w.reset(t); err != nil {
					return ctx, err
				}
				return ctx, nil
			})
			sc.After(func(ctx context.Context, s *godog.Scenario, err error) (context.Context, error) {
				if w.ts != nil {
					w.ts.Close()
				}
				return ctx, nil
			})

			sc.Given(`^a fresh panel with admin credentials$`, func() error { return nil })

			sc.Given(`^I am authenticated$`, func() error {
				return w.login("admin", "adminpw")
			})

			sc.Given(`^user "([^"]+)" exists$`, func(name string) error {
				return w.do("POST", "/api/users", map[string]string{"username": name, "password": "pw1234"})
			})

			sc.Given(`^user "([^"]+)" with password "([^"]+)" exists$`, func(name, pw string) error {
				return w.do("POST", "/api/users", map[string]string{"username": name, "password": pw})
			})

			sc.Given(`^panel domain is "([^"]+)"$`, func(d string) error {
				return w.do("PUT", "/api/settings", map[string]string{"domain": d})
			})

			sc.When(`^I login with admin credentials$`, func() error {
				return w.do("POST", "/api/login", map[string]string{"username": "admin", "password": "adminpw"})
			})

			sc.When(`^I login with a wrong password$`, func() error {
				return w.do("POST", "/api/login", map[string]string{"username": "admin", "password": "nope"})
			})

			sc.When(`^I request "([^"]+)" without a token$`, func(p string) error {
				w.token = ""
				return w.do("GET", p, nil)
			})

			sc.When(`^I create user "([^"]+)" with password "([^"]+)"$`, func(n, pw string) error {
				return w.do("POST", "/api/users", map[string]string{"username": n, "password": pw})
			})

			sc.When(`^I disable user "([^"]+)"$`, func(n string) error {
				en := false
				return w.do("PUT", "/api/users/"+n, map[string]any{"enabled": en})
			})

			sc.When(`^I delete user "([^"]+)"$`, func(n string) error {
				return w.do("DELETE", "/api/users/"+n, nil)
			})

			sc.When(`^I set config port to (\d+)$`, func(p int) error {
				return w.do("PUT", "/api/config", map[string]any{
					"port": p, "masquerade": "https://m.example", "obfs": false,
				})
			})

			sc.When(`^I request the share link for "([^"]+)"$`, func(n string) error {
				return w.do("GET", "/api/link?user="+n, nil)
			})

			sc.When(`^I send service action "([^"]+)"$`, func(a string) error {
				return w.do("POST", "/api/service", map[string]string{"action": a})
			})

			sc.Then(`^the response code is (\d+)$`, func(code int) error {
				if w.lastCode != code {
					return fmt.Errorf("want %d, got %d (%s)", code, w.lastCode, w.lastBody)
				}
				return nil
			})

			sc.Then(`^the response contains a token$`, func() error {
				var lr struct {
					Token string `json:"token"`
				}
				json.Unmarshal(w.lastBody, &lr)
				if lr.Token == "" {
					return fmt.Errorf("no token in %s", w.lastBody)
				}
				return nil
			})

			sc.Then(`^the response has no token$`, func() error {
				var lr struct {
					Token string `json:"token"`
				}
				json.Unmarshal(w.lastBody, &lr)
				if lr.Token != "" {
					return fmt.Errorf("unexpected token on failed login")
				}
				return nil
			})

			sc.Then(`^the hysteria config contains user "([^"]+)"$`, func(n string) error {
				raw, _ := os.ReadFile(w.hyConf)
				if !strings.Contains(string(raw), n+":") {
					return fmt.Errorf("config missing %s:\n%s", n, raw)
				}
				return nil
			})

			sc.Then(`^the hysteria config has NO user "([^"]+)" in userpass$`, func(n string) error {
				if _, ok := w.userpassMap()[n]; ok {
					return fmt.Errorf("user %s still in userpass", n)
				}
				return nil
			})

			sc.Then(`^the hysteria config listen port is (\d+)$`, func(p int) error {
				raw, _ := os.ReadFile(w.hyConf)
				want := fmt.Sprintf("listen: :%d", p)
				if !strings.Contains(string(raw), want) {
					return fmt.Errorf("want %q in:\n%s", want, raw)
				}
				return nil
			})

			sc.Then(`^the hysteria service was restarted$`, func() error {
				if w.fr.restarts < 1 {
					return fmt.Errorf("no restart happened")
				}
				return nil
			})

			sc.Then(`^the link starts with "([^"]+)"$`, func(prefix string) error {
				var out struct {
					Link string `json:"link"`
				}
				json.Unmarshal(w.lastBody, &out)
				if !strings.HasPrefix(out.Link, prefix) {
					return fmt.Errorf("link %q does not start with %q", out.Link, prefix)
				}
				return nil
			})

			sc.Then(`^the link contains SNI$`, func() error {
				if !strings.Contains(string(w.lastBody), "sni=") {
					return fmt.Errorf("no sni in %s", w.lastBody)
				}
				return nil
			})
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog scenarios failed")
	}
}

var _ = time.Now // silence unused in case godog's helpers change
