package api

import (
	"context"
	"github.com/alexhubin/Mowa/internal/auth"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestRoomRequiresAccountEvenWithValidGuestCookie(t *testing.T) {
	server, owner, db := newTestServer(t)
	user := provisionTestUser(t, db, "owner@example.com", "owner", "secure-password", "Owner", false)
	if _, err := db.ExecContext(context.Background(), "INSERT INTO rooms (id,invite_code,name,owner_id) VALUES ('test-room','test-invite','Test',$1)", user.ID); err != nil {
		t.Fatal(err)
	}
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(context.Background(), "INSERT INTO room_guests (id,room_id,display_name,token_hash,expires_at) VALUES ('guest_old','test-room','Guest',$1,$2)", hash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	guest := newHTTPClient(t)
	u, _ := url.Parse(server.URL)
	guest.Jar.SetCookies(u, []*http.Cookie{{Name: "mowa_guest", Value: raw, Path: "/"}})
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "", 401}, {"POST", "/token", 401}, {"GET", "/messages", 401}, {"GET", "/messages/events", 401}, {"POST", "/messages", 401},
		{"POST", "/guest", 404}, {"GET", "/guest", 404}, {"DELETE", "/guest", 404},
	} {
		r := doJSON(t, guest, tc.method, server.URL+"/api/rooms/test-invite"+tc.path, nil)
		if r.StatusCode != tc.status {
			t.Fatalf("%s %s: got %d", tc.method, tc.path, r.StatusCode)
		}
		r.Body.Close()
	}
	r := doJSON(t, owner, "POST", server.URL+"/api/auth/login", map[string]string{"email": user.Email, "password": "secure-password"})
	r.Body.Close()
	r = doJSON(t, owner, "POST", server.URL+"/api/rooms/test-invite/token", nil)
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("account token: %d", r.StatusCode)
	}
}
