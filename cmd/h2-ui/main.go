// h2-ui is a 3x-ui-style single-binary management panel for Hysteria 2.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"h2-ui/internal/auth"
	"h2-ui/internal/hysteria"
	"h2-ui/internal/server"
	"h2-ui/internal/store"
	"h2-ui/internal/systemd"
)

const version = "0.2.2"

func main() {
	var (
		addr         = flag.String("addr", envOr("H2UI_ADDR", "0.0.0.0:2053"), "panel listen address")
		dataDir      = flag.String("data", envOr("H2UI_DATA", "/etc/h2-ui"), "panel data dir (store.json)")
		hyConf       = flag.String("hyconf", envOr("H2UI_HYCONF", "/etc/hysteria/config.yaml"), "hysteria config path")
		trafficAPI   = flag.String("traffic-api", envOr("H2UI_TRAFFIC_API", "http://127.0.0.1:9999"), "hysteria traffic API base")
		trafficToken = flag.String("traffic-secret", os.Getenv("H2UI_TRAFFIC_SECRET"), "traffic API secret")
		policy       = flag.Duration("poll", 15*time.Second, "traffic poll interval")
		showVersion  = flag.Bool("version", false, "print version and exit")
		resetAdmin   = flag.Bool("reset-admin", false, "reset admin password interactively and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("h2-ui", version)
		return
	}

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	st, err := store.Load(filepath.Join(*dataDir, "store.json"))
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	adminUser, _, _ := st.AdminCredentials()
	if adminUser == "" || *resetAdmin {
		u := envOr("H2UI_ADMIN_USER", "admin")
		p := os.Getenv("H2UI_ADMIN_PASS")
		if p == "" {
			p = auth.RandomSecret(9)
		}
		h, err := auth.HashPassword(p)
		if err != nil {
			log.Fatalf("hash: %v", err)
		}
		if err := st.SetAdmin(u, h, auth.RandomSecret(32)); err != nil {
			log.Fatalf("set admin: %v", err)
		}
		fmt.Printf("== h2-ui admin credentials ==\nlogin: %s\npassword: %s\n", u, p)
		if *resetAdmin {
			return
		}
	}

	publicIP := os.Getenv("H2UI_PUBLIC_IP")
	if publicIP == "" {
		publicIP = detectOutboundIP()
	}

	srv, err := server.New(server.Options{
		Addr:         *addr,
		HysteriaConf: *hyConf,
		Store:        st,
		HyClient:     hysteria.NewClient(*trafficAPI, *trafficToken),
		Sysd:         systemd.New(),
		TokenTTL:     24 * time.Hour,
		PollInterval: *policy,
		PublicIP:     publicIP,
	})
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go srv.RunTrafficPoller(ctx)

	log.Printf("h2-ui %s starting", version)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// detectOutboundIP finds the primary outbound IPv4 (no packets are sent).
func detectOutboundIP() string {
	conn, err := net.Dial("udp", "203.0.113.1:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
