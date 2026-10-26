// Package hysteria talks to the local Hysteria 2 server: traffic API
// (GET /traffic) and log tailing. It never shell-outs into the sandbox, all
// calls are plain HTTP inside localhost.
package hysteria

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// TrafficEntry is one authenticated user in the traffic API response.
type TrafficEntry struct {
	Tx uint64 `json:"tx"`
	Rx uint64 `json:"rx"`
}

// Client polls the hysteria traffic API.
type Client struct {
	secret string
	base   string
	hc     *http.Client
}

// NewClient builds a traffic API client. base is like http://127.0.0.1:9999.
func NewClient(base, secret string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), secret: secret, hc: &http.Client{Timeout: 4 * time.Second}}
}

// Traffic returns per-user cumulative counters. Offline users are absent.
func (c *Client) Traffic(ctx context.Context) (map[string]TrafficEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/traffic", nil)
	if err != nil {
		return nil, err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", c.secret)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("traffic api: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out map[string]TrafficEntry
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Kick kicks (disconnects) users by name via POST /kick.
func (c *Client) Kick(ctx context.Context, users []string) error {
	body, err := json.Marshal(users)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/kick", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", c.secret)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("kick: %s", resp.Status)
	}
	return nil
}

// Online returns the currently-online user names via GET /online.
func (c *Client) Online(ctx context.Context) (map[string]int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/online", nil)
	if err != nil {
		return nil, err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", c.secret)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("online: %s", resp.Status)
	}
	var out map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Exec runs external commands; replaced in tests to avoid real shell-outs.
var Exec = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// ServiceStatus asks systemctl for the hysteria-server unit state.
// It returns "active", "inactive", "failed", ... or "unknown" without systemd.
func ServiceStatus() string {
	out, err := Exec("systemctl", "is-active", "hysteria-server")
	s := strings.TrimSpace(out)
	if err != nil {
		if s == "" {
			return "unknown"
		}
		return s
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// TailLogs returns the last n lines of journalctl for hysteria-server.
func TailLogs(n int) ([]string, error) {
	if n <= 0 {
		n = 100
	}
	out, err := Exec("journalctl", "-u", "hysteria-server", "-n", fmt.Sprint(n), "--no-pager", "-o", "cat")
	if err != nil {
		return nil, err
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}
