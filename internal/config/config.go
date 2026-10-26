// Package config implements read-modify-write of the Hysteria 2 server config.
//
// The panel is authoritative for:
//   - listen port
//   - obfs (salamander) toggle + password
//   - masquerade URL
//   - auth userpass map
//
// Everything else in the YAML file (tls, bandwidth, sniff, custom blocks) is
// preserved: we parse the document, touch only the managed paths and marshal
// it back.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// PanelConfig is the state managed by the panel itself (page /api/config).
type PanelConfig struct {
	Port         int               `json:"port"`
	Obfs         bool              `json:"obfs"`
	ObfsPassword string            `json:"obfsPassword"`
	Masquerade   string            `json:"masquerade"`
	Users        map[string]string `json:"users"`
}

// Defaults returns the panel config for a fresh installation.
func Defaults() *PanelConfig {
	return &PanelConfig{
		Port:         443,
		Obfs:         true,
		ObfsPassword: "",
		Masquerade:   "https://www.bing.com",
		Users:        map[string]string{},
	}
}

// LoadPanelConfig extracts the managed fields from a hysteria YAML document.
func LoadPanelConfig(path string) (*PanelConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse hysteria config: %w", err)
	}
	p := Defaults()
	if s, ok := root["listen"].(string); ok && s != "" {
		host := strings.TrimPrefix(s, ":")
		if port, err := strconv.Atoi(host); err == nil && port > 0 {
			p.Port = port
		}
	}
	if obfs, ok := root["obfs"].(map[string]any); ok {
		p.Obfs = false
		if t, ok := obfs["type"].(string); ok && t == "salamander" {
			p.Obfs = true
			if sal, ok := obfs["salamander"].(map[string]any); ok {
				if pw, ok := sal["password"].(string); ok {
					p.ObfsPassword = pw
				}
			}
		}
	} else {
		p.Obfs = false
	}
	if m, ok := root["masquerade"].(string); ok && m != "" {
		p.Masquerade = m
	} else if m, ok := root["masquerade"].(map[string]any); ok {
		// v2.13+ объект: { type: proxy, proxy: { url, rewriteHost } }
		if t, ok := m["type"].(string); ok && t == "proxy" {
			if px, ok := m["proxy"].(map[string]any); ok {
				if url, ok := px["url"].(string); ok && url != "" {
					p.Masquerade = url
				}
			}
		}
	}
	p.Users = map[string]string{}
	if auth, ok := root["auth"].(map[string]any); ok {
		if up, ok := auth["userpass"].(map[string]any); ok {
			for k, v := range up {
				if vs, ok := v.(string); ok {
					p.Users[k] = vs
				}
			}
		}
	}
	return p, nil
}

// ApplyTo mutates the YAML document at path with the managed fields and
// writes it back atomically (tmp + rename). Unmanaged keys are preserved.
func (p *PanelConfig) ApplyTo(path string) error {
	root := map[string]any{}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("parse hysteria config: %w", err)
		}
	}
	root["listen"] = fmt.Sprintf(":%d", p.Port)
	if p.Obfs {
		root["obfs"] = map[string]any{
			"type":       "salamander",
			"salamander": map[string]any{"password": p.ObfsPassword},
		}
	} else {
		delete(root, "obfs")
	}
	if p.Masquerade != "" {
		// v2.13+ требует объект; рендерим как proxy + url
		root["masquerade"] = map[string]any{
			"type": "proxy",
			"proxy": map[string]any{
				"url":         p.Masquerade,
				"rewriteHost": true,
			},
		}
	}
	if len(p.Users) > 0 {
		auth, _ := root["auth"].(map[string]any)
		if auth == nil {
			auth = map[string]any{}
		}
		auth["type"] = "userpass"
		auth["userpass"] = p.Users
		root["auth"] = auth
	} else {
		delete(root, "auth")
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o660); err != nil {
		return err
	}
	// hysteria работает от не-root юзера и должен читать конфиг; даём группе read
	_ = os.Chmod(tmp, 0o660)
	if gid, err := lookupHysteriaGID(); err == nil {
		_ = os.Chown(tmp, 0, gid)
	}
	return os.Rename(tmp, path)
}

// lookupHysteriaGID резолвит gid системного юзера hysteria, если он есть.
// На системах без такого юзера (в тестах) вернёт ошибку и chown пропустится.
func lookupHysteriaGID() (int, error) {
	out, err := os.ReadFile("/etc/group")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[0] == "hysteria" {
			if gid, err := strconv.Atoi(parts[2]); err == nil {
				return gid, nil
			}
		}
	}
	return 0, fmt.Errorf("no hysteria group")
}

// ValidatePort checks port bounds (it is used by REST validation too).
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be 1..65535, got %d", port)
	}
	return nil
}
