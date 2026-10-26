package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

const sample = `listen: :443
tls:
  type: acme
  acme:
    domain: vpn.example.com
    email: admin@example.com
auth:
  type: userpass
  userpass:
    alice: pw1
bandwidth:
  up: 200 mbps
  down: 1 gbps
masquerade: https://news.ycombinator.com
obfs:
  type: salamander
  salamander:
    password: obfs-secret
sniff:
  enable: true
`

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPanelConfig(t *testing.T) {
	p := write(t, sample)
	pc, err := LoadPanelConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if pc.Port != 443 {
		t.Fatalf("port: want 443 got %d", pc.Port)
	}
	if !pc.Obfs || pc.ObfsPassword != "obfs-secret" {
		t.Fatalf("obfs: %+v", pc)
	}
	if pc.Masquerade != "https://news.ycombinator.com" {
		t.Fatalf("masq: %q", pc.Masquerade)
	}
	if pc.Users["alice"] != "pw1" {
		t.Fatalf("users: %+v", pc.Users)
	}
}

func TestLoadDefaultsWhenFieldsMissing(t *testing.T) {
	p := write(t, "listen: :8443\n")
	pc, err := LoadPanelConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if pc.Port != 8443 {
		t.Fatalf("port: got %d", pc.Port)
	}
	if pc.Obfs {
		t.Fatal("obfs must be off when absent")
	}
	if len(pc.Users) != 0 {
		t.Fatalf("users must be empty, got %+v", pc.Users)
	}
}

func TestLoadNotExist(t *testing.T) {
	if _, err := LoadPanelConfig(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("missing file must return error")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	p := write(t, "listen: [unclosed")
	if _, err := LoadPanelConfig(p); err == nil {
		t.Fatal("invalid YAML must return error")
	}
}

func TestApplyPreservesUnmanagedKeys(t *testing.T) {
	p := write(t, sample)
	pc := &PanelConfig{
		Port:         8443,
		Obfs:         true,
		ObfsPassword: "new-obfs",
		Masquerade:   "https://example.org",
		Users:        map[string]string{"bob": "pw2", "carl": "pw3"},
	}
	if err := pc.ApplyTo(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	// unmanaged keys survive
	if root["tls"] == nil {
		t.Fatal("tls block lost")
	}
	if root["bandwidth"] == nil {
		t.Fatal("bandwidth block lost")
	}
	if root["sniff"] == nil {
		t.Fatal("sniff block lost")
	}
	// managed keys updated
	if root["listen"] != ":8443" {
		t.Fatalf("listen: %v", root["listen"])
	}
	// v2.13+: masquerade записывается как объект {type: proxy, proxy: {url, rewriteHost}}
	if m, ok := root["masquerade"].(map[string]any); !ok || m["type"] != "proxy" {
		t.Fatalf("masq must be object with type=proxy, got %v", root["masquerade"])
	} else if px, ok := m["proxy"].(map[string]any); !ok || px["url"] != "https://example.org" {
		t.Fatalf("masq proxy.url mismatch: %v", m)
	}
	auth := root["auth"].(map[string]any)
	up := auth["userpass"].(map[string]any)
	if up["bob"] != "pw2" || up["carl"] != "pw3" {
		t.Fatalf("userpass: %+v", up)
	}
	if _, gone := up["alice"]; gone {
		t.Fatal("stale user alice must be replaced wholesale")
	}
}

func TestApplyRemovesAuthWhenNoUsers(t *testing.T) {
	p := write(t, sample)
	pc, _ := LoadPanelConfig(p)
	pc.Users = map[string]string{}
	if err := pc.ApplyTo(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	var root map[string]any
	yaml.Unmarshal(raw, &root)
	if _, ok := root["auth"]; ok {
		t.Fatal("auth must be removed when there are no enabled users")
	}
}

func TestApplyRemovesObfsWhenDisabled(t *testing.T) {
	p := write(t, sample)
	pc, _ := LoadPanelConfig(p)
	pc.Obfs = false
	if err := pc.ApplyTo(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	var root map[string]any
	yaml.Unmarshal(raw, &root)
	if _, ok := root["obfs"]; ok {
		t.Fatal("obfs must be removed when disabled")
	}
}

func TestApplyToMissingFileCreatesIt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	pc := Defaults()
	pc.Users = map[string]string{"a": "b"}
	if err := pc.ApplyTo(p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPanelConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != Defaults().Port || got.Users["a"] != "b" {
		t.Fatalf("roundtrip failed: %+v", got)
	}
}

func TestRoundTrip(t *testing.T) {
	p := write(t, sample)
	pc, _ := LoadPanelConfig(p)
	if err := pc.ApplyTo(p); err != nil {
		t.Fatal(err)
	}
	pc2, err := LoadPanelConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if pc.Port != pc2.Port || pc.Obfs != pc2.Obfs || pc.ObfsPassword != pc2.ObfsPassword || pc.Masquerade != pc2.Masquerade {
		t.Fatalf("roundtrip drift:\nbefore %+v\nafter  %+v", pc, pc2)
	}
	if len(pc2.Users) != 1 || pc2.Users["alice"] != "pw1" {
		t.Fatalf("users drift: %+v", pc2.Users)
	}
}

func TestValidatePort(t *testing.T) {
	for _, ok := range []int{1, 443, 2053, 65535} {
		if err := ValidatePort(ok); err != nil {
			t.Fatalf("valid port %d rejected: %v", ok, err)
		}
	}
	for _, bad := range []int{0, -1, 65536, 100000} {
		if err := ValidatePort(bad); err == nil {
			t.Fatalf("invalid port %d accepted", bad)
		}
	}
}
