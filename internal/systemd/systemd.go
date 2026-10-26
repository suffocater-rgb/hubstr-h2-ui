// Package systemd renders and runs the commands that (re)start the
// hysteria-server service and apply the panel-managed config.
//
// Everything here is tested against a fake exec runner in unit tests; on the
// real server it calls systemctl.
package systemd

import (
	"fmt"
	"os/exec"
	"strings"
)

// Runner abstracts exec.Command for tests.
type Runner interface {
	Run(name string, args ...string) error
}

type realRunner struct{}

func (realRunner) Run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Manager applies actions to the hysteria-server systemd unit.
type Manager struct{ r Runner }

// New returns a Manager using the real systemctl.
func New() *Manager { return &Manager{r: realRunner{}} }

// NewWithRunner is for tests.
func NewWithRunner(r Runner) *Manager { return &Manager{r: r} }

// Restart restarts the service (config changes need this).
func (m *Manager) Restart() error { return m.r.Run("systemctl", "restart", "hysteria-server") }

// Start starts the service.
func (m *Manager) Start() error { return m.r.Run("systemctl", "start", "hysteria-server") }

// Stop stops the service.
func (m *Manager) Stop() error { return m.r.Run("systemctl", "stop", "hysteria-server") }

// Unit renders the systemd unit for hysteria-server.
// binary is /usr/local/bin/hysteria, config /etc/hysteria/config.yaml.
func Unit(binary, config string) string {
	return fmt.Sprintf(`[Unit]
Description=Hysteria 2 Server
After=network.target

[Service]
Type=simple
ExecStart=%s server -c %s
Restart=on-failure
RestartSec=3
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`, binary, config)
}
