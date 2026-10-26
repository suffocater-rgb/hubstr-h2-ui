package systemd

import (
	"strings"
	"testing"
)

type fakeRunner struct {
	calls []string
	err   error
}

func (f *fakeRunner) Run(name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.err
}

func TestActionsCallSystemctl(t *testing.T) {
	f := &fakeRunner{}
	m := NewWithRunner(f)
	m.Start()
	m.Stop()
	m.Restart()
	want := []string{
		"systemctl start hysteria-server",
		"systemctl stop hysteria-server",
		"systemctl restart hysteria-server",
	}
	if len(f.calls) != len(want) {
		t.Fatalf("calls: %v", f.calls)
	}
	for i := range want {
		if f.calls[i] != want[i] {
			t.Fatalf("call %d: want %q got %q", i, want[i], f.calls[i])
		}
	}
}

func TestUnitRendersPaths(t *testing.T) {
	u := Unit("/usr/local/bin/hysteria", "/etc/hysteria/config.yaml")
	for _, frag := range []string{
		"ExecStart=/usr/local/bin/hysteria server -c /etc/hysteria/config.yaml",
		"[Install]",
		"WantedBy=multi-user.target",
		"Restart=on-failure",
	} {
		if !strings.Contains(u, frag) {
			t.Fatalf("unit missing %q:\n%s", frag, u)
		}
	}
}
