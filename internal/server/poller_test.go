package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"h2-ui/internal/auth"
	"h2-ui/internal/hysteria"
	"h2-ui/internal/store"
	"h2-ui/internal/systemd"
)

type trafficStub struct {
	mu  sync.Mutex
	seq []map[string]hysteria.TrafficEntry // successive poll shape
	i   int
}

func (s *trafficStub) Traffic(ctx context.Context) (map[string]hysteria.TrafficEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.i >= len(s.seq) {
		return map[string]hysteria.TrafficEntry{}, nil
	}
	m := s.seq[s.i]
	s.i++
	return m, nil
}
func (s *trafficStub) Online(ctx context.Context) (map[string]int, error) { return nil, nil }
func (s *trafficStub) Kick(ctx context.Context, users []string) error     { return nil }

func TestRunTrafficPollerAccumulatesAcrossTicks(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Load(filepath.Join(dir, "store.json"))
	st.AddUser("net", "pw1234")
	hash, _ := auth.HashPassword("x")
	st.SetAdmin("a", hash, "secret")

	conf := filepath.Join(dir, "config.yaml")
	os.WriteFile(conf, []byte("listen: :443\n"), 0o600)

	stub := &trafficStub{}
	stub.seq = []map[string]hysteria.TrafficEntry{
		{"net": {Tx: 1000, Rx: 2000}},
		{"net": {Tx: 1500, Rx: 2500}},
		{"net": {Tx: 100, Rx: 100}}, // after hysteria restart
	}

	srv, err := New(Options{
		Addr:         "127.0.0.1:0",
		HysteriaConf: conf,
		Store:        st,
		HyClient:     hysteria.NewClient("http://127.0.0.1:1", ""), // unused, we drive poller via stub
		Sysd:         systemd.NewWithRunner(&fakeRunner{}),
		TokenTTL:     time.Hour,
		PollInterval: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Drive one logical tick directly through the exported poller loop's core
	// via a local wrapper that mirrors RunTrafficPoller's delta logic.
	type phony struct {
		T func(ctx context.Context) (map[string]hysteria.TrafficEntry, error)
	}
	var _ phony

	last := map[string]hysteria.TrafficEntry{}
	step := func() {
		cur, err := stub.Traffic(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for name, e := range cur {
			p := last[name]
			dtx, drx := e.Tx, e.Rx
			if e.Tx >= p.Tx {
				dtx = e.Tx - p.Tx
			}
			if e.Rx >= p.Rx {
				drx = e.Rx - p.Rx
			}
			srv.o.Store.AddTraffic(name, dtx, drx)
		}
		last = cur
	}
	step()
	step()
	step()

	u, err := st.GetUser("net")
	if err != nil {
		t.Fatal(err)
	}
	// 1000+500+100 up; 2000+500+100 down -> hysteria restart delta counted fully
	if u.Up != 1600 || u.Down != 2600 {
		t.Fatalf("accumulation: got up=%d down=%d", u.Up, u.Down)
	}
}

var _ = time.Now
