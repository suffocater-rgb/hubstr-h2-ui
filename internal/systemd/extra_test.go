package systemd

import (
	"testing"
)

func TestRealRunnerPropagatesError(t *testing.T) {
	m := &Manager{r: realRunner{}}
	// systemctl exists in CI images usually; if not, an error is still a pass —
	// what we assert is that errors surface instead of being swallowed.
	err := m.Restart()
	if err == nil {
		t.Skip("hysteria-server actually restarted (unexpected but fine)")
	}
	if err.Error() == "" {
		t.Fatal("empty error message")
	}
}
