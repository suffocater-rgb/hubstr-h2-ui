// Package server wires everything together: REST API under /api, the
// embedded SPA for everything else, and the traffic poller that feeds
// hysteria's cumulative counters into the store.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"h2-ui/internal/auth"
	"h2-ui/internal/config"
	"h2-ui/internal/hysteria"
	"h2-ui/internal/store"
	"h2-ui/internal/systemd"
	"h2-ui/web"
)

// Options carries runtime dependencies (test-double friendly).
type Options struct {
	Addr         string // listen addr of the panel itself, e.g. 127.0.0.1:8080
	HysteriaConf string // path to /etc/hysteria/config.yaml
	Store        *store.Store
	HyClient     *hysteria.Client
	Sysd         *systemd.Manager
	TokenTTL     time.Duration
	// PollInterval for traffic sync; 0 = disabled (tests).
	PollInterval time.Duration
	// PublicIP overrides link host when panelSettings.Domain is empty.
	PublicIP string
}

// Server is the panel HTTP server.
type Server struct {
	o   Options
	mux *http.ServeMux
}

// New builds the server and its routes.
func New(o Options) (*Server, error) {
	s := &Server{o: o}
	if s.o.TokenTTL == 0 {
		s.o.TokenTTL = 24 * time.Hour
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s, nil
}

// Handler exposes the root http.Handler (tests use httptest).
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	_, admin, jwtSecret := split3(s.o.Store.AdminCredentials())
	_ = admin

	s.mux.HandleFunc("/api/login", s.handleLogin)

	protected := http.NewServeMux()
	protected.HandleFunc("/api/summary", s.handleSummary)
	protected.HandleFunc("/api/users/reset-traffic", s.handleResetTraffic) // POST — должен идти ДО /api/users/
	protected.HandleFunc("/api/users", s.handleUsers)                      // GET list, POST create
	protected.HandleFunc("/api/users/", s.handleUser)                      // GET/PUT/DELETE /api/users/{name}, POST /api/users/{name}/kick
	protected.HandleFunc("/api/config", s.handleConfig)                    // GET/PUT
	protected.HandleFunc("/api/service", s.handleService)                  // POST {action}
	protected.HandleFunc("/api/settings", s.handleSettings)                // GET/PUT
	protected.HandleFunc("/api/logs", s.handleLogs)
	protected.HandleFunc("/api/link", s.handleLink) // GET ?user=name

	s.mux.Handle("/api/", auth.Middleware(jwtSecret, protected))

	// SPA: static files, fallback to index.html
	dist, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		log.Printf("web assets not embedded: %v", err)
	} else {
		fileServer := http.FileServer(http.FS(dist))
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			p := strings.TrimPrefix(r.URL.Path, "/")
			if p == "" {
				p = "index.html"
			}
			if _, err := fs.Stat(dist, p); err != nil {
				r.URL.Path = "/" // SPA fallback
			}
			fileServer.ServeHTTP(w, r)
		})
	}
}

func split3(a, b, c string) (string, string, string) { return a, b, c }

// --- handlers ----------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	adminUser, adminHash, jwtSecret := s.o.Store.AdminCredentials()
	tok, ok := auth.Login(adminUser, adminHash, jwtSecret, body.Username, body.Password, s.o.TokenTTL)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	users := s.o.Store.ListUsers()
	var up, down uint64
	enabled := 0
	for _, u := range users {
		up += u.Up
		down += u.Down
		if u.Enabled {
			enabled++
		}
	}
	online := map[string]int{}
	if s.o.HyClient != nil {
		if o, err := s.o.HyClient.Online(r.Context()); err == nil {
			online = o
		}
	}
	// Hysteria приводит userpass auth к lowercase, а в store имена могут быть
	// с mixed case (Art). Нормализуем online к регистру store.
	{
		byLower := make(map[string]string, len(users))
		for _, u := range users {
			byLower[strings.ToLower(u.Username)] = u.Username
		}
		normalized := make(map[string]int, len(online))
		for name, count := range online {
			if orig, ok := byLower[strings.ToLower(name)]; ok {
				normalized[orig] = count
			} else {
				normalized[name] = count
			}
		}
		online = normalized
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       hysteria.ServiceStatus(),
		"usersTotal":   len(users),
		"usersEnabled": enabled,
		"trafficUp":    up,
		"trafficDown":  down,
		"online":       online,
		"onlineTotal":  len(online),
		"domain":       s.o.Store.Settings().Domain,
	})
}

// handleResetTraffic clears up/down counters for every user.
func (s *Server) handleResetTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	users := s.o.Store.ListUsers()
	for _, u := range users {
		// UpdateUser mutates the copy, so we mutate counters via direct API:
		// Store doesn't have a counter-reset helper, we do it here in-line by
		// re-using AddTraffic with negative deltas is wrong — instead use a
		// dedicated reset on the store.
		_ = s.o.Store.ResetTraffic(u.Username)
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "traffic reset", "count": strconv.Itoa(len(users))})
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.o.Store.ListUsers())
	case http.MethodPost:
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "bad json")
			return
		}
		if err := validateName(body.Username); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(body.Password) < 4 {
			writeErr(w, http.StatusBadRequest, "password must be at least 4 chars")
			return
		}
		u, err := s.o.Store.AddUser(body.Username, body.Password)
		if errors.Is(err, store.ErrUserExists) {
			writeErr(w, http.StatusConflict, "user exists")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.syncAuth(r.Context())
		writeJSON(w, http.StatusCreated, u)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if strings.HasSuffix(rest, "/kick") {
		name := strings.TrimSuffix(rest, "/kick")
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if _, err := s.o.Store.GetUser(name); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if s.o.HyClient == nil {
			writeErr(w, http.StatusBadGateway, "traffic api disabled")
			return
		}
		if err := s.o.HyClient.Kick(r.Context(), []string{name}); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "kicked"})
		return
	}
	name := rest
	switch r.Method {
	case http.MethodGet:
		u, err := s.o.Store.GetUser(name)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, u)
	case http.MethodPut:
		var body struct {
			Password *string `json:"password"`
			Enabled  *bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "bad json")
			return
		}
		if body.Password != nil && len(*body.Password) < 4 {
			writeErr(w, http.StatusBadRequest, "password must be at least 4 chars")
			return
		}
		u, err := s.o.Store.UpdateUser(name, body.Password, body.Enabled)
		if errors.Is(err, store.ErrUserNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.syncAuth(r.Context())
		writeJSON(w, http.StatusOK, u)
	case http.MethodDelete:
		if err := s.o.Store.DeleteUser(name); errors.Is(err, store.ErrUserNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		} else if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.syncAuth(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"ok": "deleted"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		pc, err := config.LoadPanelConfig(s.o.HysteriaConf)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, pc)
	case http.MethodPut:
		var pc config.PanelConfig
		if err := json.NewDecoder(r.Body).Decode(&pc); err != nil {
			writeErr(w, http.StatusBadRequest, "bad json")
			return
		}
		if err := config.ValidatePort(pc.Port); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if pc.Obfs && pc.ObfsPassword == "" {
			writeErr(w, http.StatusBadRequest, "obfs password required when obfs is on")
			return
		}
		// hysteria userpass always mirrors the (enabled) panel users:
		pc.Users = s.o.Store.UserpassMap()
		if err := pc.ApplyTo(s.o.HysteriaConf); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.o.Sysd.Restart(); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "applied"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	var err error
	switch body.Action {
	case "start":
		err = s.o.Sysd.Start()
	case "stop":
		err = s.o.Sysd.Stop()
	case "restart":
		err = s.o.Sysd.Restart()
	default:
		writeErr(w, http.StatusBadRequest, "action must be start|stop|restart")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": body.Action})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.o.Store.Settings())
	case http.MethodPut:
		var ps store.PanelSettings
		if err := json.NewDecoder(r.Body).Decode(&ps); err != nil {
			writeErr(w, http.StatusBadRequest, "bad json")
			return
		}
		if err := s.o.Store.SetSettings(ps); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ps)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	lines, err := hysteria.TailLogs(n)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("user")
	u, err := s.o.Store.GetUser(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	pc, err := config.LoadPanelConfig(s.o.HysteriaConf)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	host := s.o.Store.Settings().Domain
	if host == "" {
		host = s.o.PublicIP
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"link":   BuildLink(u.Username, u.Password, host, pc),
		"scheme": "hysteria2",
	})
}

// BuildLink renders the hysteria2:// share URI for a user.
// Format: hysteria2://user:pass@host:port?obfs=salamander&obfs-password=..&sni=..#name
// SNI defaults to the link host (домен серта сервера); masquerade — это то,
// на что сервер отвечает для чужих TLS-запросов, не связано с SNI подключения.
// sniOverride, если не пусто, перебивает (например, при разделении SNI/хоста).
func BuildLink(user, pass, host string, pc *config.PanelConfig) string {
	sni := host
	if i := strings.IndexByte(host, ':'); i >= 0 { // strip port if any
		sni = host[:i]
	}
	if pc.SNI != "" {
		sni = pc.SNI
	}
	var b strings.Builder
	b.WriteString("hysteria2://")
	b.WriteString(urlQueryEscape(user))
	b.WriteByte(':')
	b.WriteString(urlQueryEscape(pass))
	b.WriteByte('@')
	b.WriteString(host)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(pc.Port))
	params := []string{"sni=" + urlQueryEscape(sni)}
	if pc.Obfs && pc.ObfsPassword != "" {
		params = append(params, "obfs=salamander", "obfs-password="+urlQueryEscape(pc.ObfsPassword))
	}
	b.WriteByte('?')
	b.WriteString(strings.Join(params, "&"))
	b.WriteByte('#')
	b.WriteString(urlQueryEscape(user))
	return b.String()
}

func urlQueryEscape(s string) string {
	// small subset sufficient for user/pass/sni
	repl := strings.NewReplacer(
		"%", "%25", "@", "%40", ":", "%3A", "/", "%2F", "?", "%3F", "#", "%23",
		"&", "%26", "=", "%3D", "+", "%2B", " ", "%20",
	)
	return repl.Replace(s)
}

// validateName enforces safe usernames (they land in YAML and journald).
func validateName(n string) error {
	if len(n) < 1 || len(n) > 32 {
		return errors.New("username must be 1..32 chars")
	}
	for _, c := range n {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.'
		if !ok {
			return errors.New("username may contain letters, digits, _ . -")
		}
	}
	return nil
}

// syncAuth pushes the enabled userpass map into the hysteria config and
// restarts the service. Failures are logged, not fatal to the request.
func (s *Server) syncAuth(ctx context.Context) {
	pc, err := config.LoadPanelConfig(s.o.HysteriaConf)
	if err != nil {
		log.Printf("syncAuth: load config: %v", err)
		return
	}
	pc.Users = s.o.Store.UserpassMap()
	if err := pc.ApplyTo(s.o.HysteriaConf); err != nil {
		log.Printf("syncAuth: apply config: %v", err)
		return
	}
	if err := s.o.Sysd.Restart(); err != nil {
		log.Printf("syncAuth: restart: %v", err)
	}
}

// RunTrafficPoller periodically merges hysteria counters into the store.
func (s *Server) RunTrafficPoller(ctx context.Context) {
	if s.o.PollInterval <= 0 || s.o.HyClient == nil {
		return
	}
	last := map[string]hysteria.TrafficEntry{}
	tick := time.NewTicker(s.o.PollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cur, err := s.o.HyClient.Traffic(ctx)
			if err != nil {
				log.Printf("traffic poll: %v", err)
				continue
			}
			// Hysteria приводит имя к lower case; маппим обратно к case в store.
			byLower := map[string]string{}
			for _, u := range s.o.Store.ListUsers() {
				byLower[strings.ToLower(u.Username)] = u.Username
			}
			for name, e := range cur {
				prev := last[name]
				dtx, drx := e.Tx, e.Rx
				if e.Tx >= prev.Tx {
					dtx = e.Tx - prev.Tx
				}
				if e.Rx >= prev.Rx {
					drx = e.Rx - prev.Rx
				}
				realName := name
				if orig, ok := byLower[strings.ToLower(name)]; ok {
					realName = orig
				}
				s.o.Store.AddTraffic(realName, dtx, drx)
			}
			// counters reset for users that disappeared (server restart)
			for name := range last {
				if _, ok := cur[name]; !ok {
					// keep store totals; nothing to add
					delete(last, name)
				}
			}
			last = cur
			_ = s.o.Store.Save()
		}
	}
}

// ListenAndServe starts the panel.
func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.o.Addr)
	if err != nil {
		return err
	}
	log.Printf("h2-ui panel on http://%s", s.o.Addr)
	return http.Serve(l, s.mux)
}
