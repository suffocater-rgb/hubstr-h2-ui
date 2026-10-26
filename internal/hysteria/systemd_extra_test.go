package hysteria

import (
	"errors"
	"strings"
	"testing"
)

func TestServiceStatusActive(t *testing.T) {
	old := Exec
	defer func() { Exec = old }()
	Exec = func(name string, args ...string) (string, error) {
		if name != "systemctl" {
			t.Fatalf("unexpected cmd: %s", name)
		}
		return "active\n", nil
	}
	if got := ServiceStatus(); got != "active" {
		t.Fatalf("want active, got %q", got)
	}
}

func TestServiceStatusFailed(t *testing.T) {
	old := Exec
	defer func() { Exec = old }()
	Exec = func(name string, args ...string) (string, error) {
		return "failed\n", errors.New("exit 3")
	}
	if got := ServiceStatus(); got != "failed" {
		t.Fatalf("want failed, got %q", got)
	}
}

func TestServiceStatusUnknownWhenNoSystemd(t *testing.T) {
	old := Exec
	defer func() { Exec = old }()
	Exec = func(name string, args ...string) (string, error) {
		return "", errors.New("command not found")
	}
	if got := ServiceStatus(); got != "unknown" {
		t.Fatalf("want unknown, got %q", got)
	}
}

func TestTailLogsParsesLines(t *testing.T) {
	old := Exec
	defer func() { Exec = old }()
	Exec = func(name string, args ...string) (string, error) {
		if name != "journalctl" {
			t.Fatalf("unexpected cmd: %s", name)
		}
		return "line1\nline2\nline3\n", nil
	}
	lines, err := TailLogs(50)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || lines[0] != "line1" || lines[2] != "line3" {
		t.Fatalf("lines: %#v", lines)
	}
}

func TestTailLogsDefaultCount(t *testing.T) {
	old := Exec
	defer func() { Exec = old }()
	gotArgs := []string{}
	Exec = func(name string, args ...string) (string, error) {
		gotArgs = append(gotArgs, args...)
		return "ok\n", nil
	}
	if _, err := TailLogs(0); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "-n 100") {
		t.Fatalf("default count not applied, args=%v", gotArgs)
	}
}

func TestTailLogsErrorPropagates(t *testing.T) {
	old := Exec
	defer func() { Exec = old }()
	Exec = func(name string, args ...string) (string, error) {
		return "", errors.New("no journal")
	}
	if _, err := TailLogs(10); err == nil {
		t.Fatal("error must propagate")
	}
}
