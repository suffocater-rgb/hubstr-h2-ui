package store

import (
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	st, err := Load(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return st
}

func TestLoadCreatesEmptyState(t *testing.T) {
	st := tempStore(t)
	if got := len(st.ListUsers()); got != 0 {
		t.Fatalf("want 0 users, got %d", got)
	}
	u, h, s := st.AdminCredentials()
	if u != "" || h != "" || s != "" {
		t.Fatalf("admin should be empty initially")
	}
}

func TestSetAdminPersists(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "store.json")
	st, _ := Load(p)
	if err := st.SetAdmin("admin", "hash", "secret"); err != nil {
		t.Fatal(err)
	}
	st2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	u, h, s := st2.AdminCredentials()
	if u != "admin" || h != "hash" || s != "secret" {
		t.Fatalf("credentials not persisted: %q %q %q", u, h, s)
	}
}

func TestAddAndGetUser(t *testing.T) {
	st := tempStore(t)
	u, err := st.AddUser("alice", "pw1234")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 1 {
		t.Fatalf("first id should be 1, got %d", u.ID)
	}
	if !u.Enabled {
		t.Fatalf("new user should be enabled")
	}
	got, err := st.GetUser("alice")
	if err != nil || got.Password != "pw1234" {
		t.Fatalf("GetUser: %v %+v", err, got)
	}
}

func TestAddDuplicateUser(t *testing.T) {
	st := tempStore(t)
	if _, err := st.AddUser("bob", "pw1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddUser("bob", "other"); err != ErrUserExists {
		t.Fatalf("want ErrUserExists, got %v", err)
	}
}

func TestUserIDsIncrease(t *testing.T) {
	st := tempStore(t)
	a, _ := st.AddUser("a", "pw1234")
	b, _ := st.AddUser("b", "pw1234")
	c, _ := st.AddUser("c", "pw1234")
	if !(a.ID < b.ID && b.ID < c.ID) {
		t.Fatalf("ids must increase: %d %d %d", a.ID, b.ID, c.ID)
	}
	// deleting b must NOT reuse its id
	if err := st.DeleteUser("b"); err != nil {
		t.Fatal(err)
	}
	d, _ := st.AddUser("d", "pw1234")
	if d.ID <= c.ID {
		t.Fatalf("id reused after delete: %d", d.ID)
	}
}

func TestUpdateUser(t *testing.T) {
	st := tempStore(t)
	if _, err := st.AddUser("carl", "pw1234"); err != nil {
		t.Fatal(err)
	}
	newPw := "newpass"
	en := false
	u, err := st.UpdateUser("carl", &newPw, &en)
	if err != nil {
		t.Fatal(err)
	}
	if u.Password != "newpass" || u.Enabled {
		t.Fatalf("update failed: %+v", u)
	}
	if _, err := st.UpdateUser("ghost", &newPw, nil); err != ErrUserNotFound {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestDeleteUser(t *testing.T) {
	st := tempStore(t)
	st.AddUser("dana", "pw1234")
	if err := st.DeleteUser("dana"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteUser("dana"); err != ErrUserNotFound {
		t.Fatalf("want ErrUserNotFound, got %v", err)
	}
}

func TestUserpassMapOnlyEnabled(t *testing.T) {
	st := tempStore(t)
	st.AddUser("e1", "pw1234")
	st.AddUser("e2", "pw1234")
	en := false
	st.UpdateUser("e2", nil, &en)
	m := st.UserpassMap()
	if len(m) != 1 {
		t.Fatalf("want 1 enabled user, got %d", len(m))
	}
	if _, ok := m["e1"]; !ok {
		t.Fatalf("e1 should be enabled")
	}
	if _, ok := m["e2"]; ok {
		t.Fatalf("e2 is disabled and must not be in auth map")
	}
}

func TestAddTrafficAccumulates(t *testing.T) {
	st := tempStore(t)
	st.AddUser("fay", "pw1234")
	st.AddTraffic("fay", 100, 200)
	st.AddTraffic("fay", 50, 25)
	u, _ := st.GetUser("fay")
	if u.Up != 150 || u.Down != 225 {
		t.Fatalf("traffic: up=%d down=%d", u.Up, u.Down)
	}
	// unknown user must not panic or create entries
	st.AddTraffic("ghost", 1, 1)
	if _, err := st.GetUser("ghost"); err != ErrUserNotFound {
		t.Fatalf("ghost user must not be created by traffic")
	}
}

func TestUsersPersistAfterReload(t *testing.T) {
	p := filepath.Join(t.TempDir(), "store.json")
	st, _ := Load(p)
	st.AddUser("gina", "pw1234")
	st.AddTraffic("gina", 7, 9)
	st.Save()

	st2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	u, err := st2.GetUser("gina")
	if err != nil {
		t.Fatal(err)
	}
	if u.Up != 7 || u.Down != 9 {
		t.Fatalf("traffic not persisted: %+v", u)
	}
}

func TestListUsersSortedByID(t *testing.T) {
	st := tempStore(t)
	st.AddUser("z", "pw1234")
	st.AddUser("a", "pw1234")
	st.AddUser("m", "pw1234")
	list := st.ListUsers()
	for i := 1; i < len(list); i++ {
		if list[i-1].ID >= list[i].ID {
			t.Fatalf("not sorted by id")
		}
	}
}

func TestListUsersReturnsCopies(t *testing.T) {
	st := tempStore(t)
	st.AddUser("h", "pw1234")
	list := st.ListUsers()
	list[0].Password = "hacked"
	u, _ := st.GetUser("h")
	if u.Password == "hacked" {
		t.Fatalf("list mutation leaked into store")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	st := tempStore(t)
	if err := st.SetSettings(PanelSettings{Domain: "vpn.example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := st.Settings().Domain; got != "vpn.example.com" {
		t.Fatalf("want vpn.example.com, got %q", got)
	}
}
