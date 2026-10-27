// Package store is the panel's persistent state: users, traffic counters,
// the panel's admin account and panel settings. It is a single JSON file,
// written atomically (tmp + rename) under a mutex.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// User is a Hysteria 2 account managed by the panel.
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	Up        uint64    `json:"up"`   // bytes, cumulative
	Down      uint64    `json:"down"` // bytes, cumulative
}

// PanelSettings are panel-side preferences (not hysteria config).
type PanelSettings struct {
	Domain string `json:"domain"` // used for link generation; empty = server IP
}

// State is the root of the JSON file.
type State struct {
	AdminUser     string           `json:"adminUser"`
	AdminPassHash string           `json:"adminPassHash"`
	JWTSecret     string           `json:"jwtSecret"`
	Users         map[string]*User `json:"users"`
	NextUserID    int64            `json:"nextUserId"`
	Settings      PanelSettings    `json:"settings"`
}

// Store guards the state and owns persistence.
type Store struct {
	mu   sync.RWMutex
	path string
	s    *State
}

// Load opens (or initializes) the store file.
func Load(path string) (*Store, error) {
	st := &Store{path: path}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		st.s = &State{
			Users:      map[string]*User{},
			NextUserID: 1,
		}
		return st, st.saveLocked()
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse store: %w", err)
	}
	if s.Users == nil {
		s.Users = map[string]*User{}
	}
	if s.NextUserID == 0 {
		s.NextUserID = 1
	}
	st.s = &s
	return st, nil
}

func (st *Store) saveLocked() error {
	raw, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

// Save forces a flush (used by callers that mutate returned pointers).
func (st *Store) Save() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.saveLocked()
}

// --- auth metadata ----------------------------------------------------

// AdminCredentials returns admin login data (write-once via SetAdmin).
func (st *Store) AdminCredentials() (user, passHash, jwtSecret string) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.s.AdminUser, st.s.AdminPassHash, st.s.JWTSecret
}

// SetAdmin sets/replaces admin credentials.
func (st *Store) SetAdmin(user, passHash, jwtSecret string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.AdminUser = user
	st.s.AdminPassHash = passHash
	st.s.JWTSecret = jwtSecret
	return st.saveLocked()
}

// --- users ------------------------------------------------------------

// ErrUserExists / ErrUserNotFound are sentinel errors mapped to HTTP codes.
var (
	ErrUserExists   = fmt.Errorf("user already exists")
	ErrUserNotFound = fmt.Errorf("user not found")
)

// ListUsers returns a snapshot sorted by id.
func (st *Store) ListUsers() []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*User, 0, len(st.s.Users))
	for _, u := range st.s.Users {
		cp := *u
		out = append(out, &cp)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ID < out[i].ID {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// GetUser returns a copy of one user.
func (st *Store) GetUser(name string) (*User, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	u, ok := st.s.Users[name]
	if !ok {
		return nil, ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}

// AddUser inserts a new user.
func (st *Store) AddUser(name, password string) (*User, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.s.Users[name]; ok {
		return nil, ErrUserExists
	}
	u := &User{
		ID:        st.s.NextUserID,
		Username:  name,
		Password:  password,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
	}
	st.s.NextUserID++
	st.s.Users[name] = u
	if err := st.saveLocked(); err != nil {
		return nil, err
	}
	cp := *u
	return &cp, nil
}

// UpdateUser modifies password / enabled flag.
func (st *Store) UpdateUser(name string, password *string, enabled *bool) (*User, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	u, ok := st.s.Users[name]
	if !ok {
		return nil, ErrUserNotFound
	}
	if password != nil {
		u.Password = *password
	}
	if enabled != nil {
		u.Enabled = *enabled
	}
	if err := st.saveLocked(); err != nil {
		return nil, err
	}
	cp := *u
	return &cp, nil
}

// DeleteUser removes a user.
func (st *Store) DeleteUser(name string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.s.Users[name]; !ok {
		return ErrUserNotFound
	}
	delete(st.s.Users, name)
	return st.saveLocked()
}

// UserpassMap returns enabled users as hysteria auth map.
func (st *Store) UserpassMap() map[string]string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := map[string]string{}
	for name, u := range st.s.Users {
		if u.Enabled {
			out[name] = u.Password
		}
	}
	return out
}

// AddTraffic increments counters for a user (identified by username).
func (st *Store) AddTraffic(name string, tx, rx uint64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if u, ok := st.s.Users[name]; ok {
		u.Up += tx
		u.Down += rx
	}
}

// ResetTraffic zeroes up/down counters for one user. No-op for unknown user.
func (st *Store) ResetTraffic(name string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	u, ok := st.s.Users[name]
	if !ok {
		return ErrUserNotFound
	}
	u.Up = 0
	u.Down = 0
	return st.saveLocked()
}

// Settings / SetSettings ------------------------------------------------

func (st *Store) Settings() PanelSettings {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.s.Settings
}

func (st *Store) SetSettings(s PanelSettings) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.Settings = s
	return st.saveLocked()
}
