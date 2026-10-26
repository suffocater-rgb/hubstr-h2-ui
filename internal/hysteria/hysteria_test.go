package hysteria

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTrafficParsesResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/traffic" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "sec" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"alice": map[string]uint64{"tx": 1000, "rx": 2000},
			"bob":   map[string]uint64{"tx": 5, "rx": 6},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "sec")
	m, err := c.Traffic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m["alice"].Tx != 1000 || m["alice"].Rx != 2000 || m["bob"].Tx != 5 {
		t.Fatalf("traffic: %+v", m)
	}
}

func TestTrafficErrorStatuses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	c := NewClient(ts.URL, "")
	if _, err := c.Traffic(context.Background()); err == nil {
		t.Fatal("500 must surface an error")
	}
}

func TestOnlineParsesResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]int{"u1": 1, "u2": 2})
	}))
	defer ts.Close()
	c := NewClient(ts.URL, "")
	o, err := c.Online(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(o) != 2 || o["u2"] != 2 {
		t.Fatalf("online: %+v", o)
	}
}

func TestKickPostsUserList(t *testing.T) {
	got := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var users []string
		json.NewDecoder(r.Body).Decode(&users)
		for i, u := range users {
			if i > 0 {
				got += ","
			}
			got += u
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	c := NewClient(ts.URL, "")
	if err := c.Kick(context.Background(), []string{"lena", "ivan"}); err != nil {
		t.Fatal(err)
	}
	if got != "lena,ivan" {
		t.Fatalf("kick body: %q", got)
	}
}

func TestClientTimesOut(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(6 * time.Second)
	}))
	defer ts.Close()
	c := NewClient(ts.URL, "")
	_, err := c.Traffic(context.Background())
	if err == nil {
		t.Fatal("timeout must produce error")
	}
}
