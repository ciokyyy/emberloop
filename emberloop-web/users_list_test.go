package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// GET /api/users lists every account with its presence. A User is online
// while one live channel of its own is open. Tested at the HTTP seam.

func usersOf(t *testing.T, h http.Handler, c *http.Cookie) []userView {
	t.Helper()
	rec := authDo(t, h, http.MethodGet, "/api/users", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/users = %d: %s", rec.Code, rec.Body.String())
	}
	var out []userView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode users: %v (body %q)", err, rec.Body.String())
	}
	return out
}

func userNamed(list []userView, name string) userView {
	for _, u := range list {
		if u.Username == name {
			return u
		}
	}
	return userView{}
}

func TestUsersListsAccountsInCreationOrderOffline(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	signedIn(t, h, "mira")

	list := usersOf(t, h, admin)
	if len(list) != 2 || list[0].Username != "admin" || list[1].Username != "mira" {
		t.Fatalf("users = %+v, want admin then mira", list)
	}
	if !list[0].Admin || list[1].Admin {
		t.Fatalf("admin flags = %+v, want admin only on the first account", list)
	}
	for _, u := range list {
		if u.Online {
			t.Fatalf("%s online with no live channel open", u.Username)
		}
	}
}

func TestOpenLiveChannelMakesUserOnlineUntilItCloses(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	admin := signedIn(t, h, "admin")
	mira := signedIn(t, h, "mira")
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/live", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(mira)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if line, err := bufio.NewReader(resp.Body).ReadString('\n'); err != nil || !strings.HasPrefix(line, "data: hello") {
		t.Fatalf("first live message = %q, %v; want data: hello", line, err)
	}
	if u := userNamed(usersOf(t, h, admin), "mira"); !u.Online {
		t.Fatalf("mira = %+v with a live channel open, want online", u)
	}

	cancel()
	resp.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for userNamed(usersOf(t, h, admin), "mira").Online {
		if time.Now().After(deadline) {
			t.Fatal("mira still online after its live channel closed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUsersNeedASignedInUser(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	h := d.Handler()
	signedIn(t, h, "mira")
	if rec := authDo(t, h, http.MethodGet, "/api/users", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("signed-out GET /api/users = %d, want 401", rec.Code)
	}
	if rec := authDo(t, withBearer(h, d.machineToken), http.MethodGet, "/api/users", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("machine token GET /api/users = %d, want 401", rec.Code)
	}
}
