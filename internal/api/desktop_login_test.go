package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

func TestDesktopLoginProofExpirySingleUseAndSeparateSession(t *testing.T) {
	server, browser, db := newTestServer(t)
	user := provisionTestUser(t, db, "desktop@example.com", "desktop", "secure-password", "Desktop", false)
	r := doJSON(t, browser, "POST", server.URL+"/api/auth/login", map[string]string{"email": user.Email, "password": "secure-password"})
	r.Body.Close()
	desktop := newHTTPClient(t)
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	start := func() string {
		r := doJSON(t, desktop, "POST", server.URL+"/api/auth/desktop/start", map[string]string{"challenge": challenge})
		if r.StatusCode != 201 {
			t.Fatal(r.StatusCode, responseBody(t, r))
		}
		var value map[string]string
		decodeResponse(t, r, &value)
		return value["id"]
	}
	check := func(r *http.Response, want int) {
		t.Helper()
		defer r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("got %d want %d: %s", r.StatusCode, want, responseBody(t, r))
		}
	}
	id := start()
	exchange := func(proof string) *http.Response {
		return doJSON(t, desktop, "POST", server.URL+"/api/auth/desktop/exchange", map[string]string{"id": id, "verifier": proof})
	}
	check(exchange(verifier), 202)
	check(doJSON(t, desktop, "POST", server.URL+"/api/auth/desktop/approve", map[string]string{"id": id}), 401)
	check(doJSON(t, browser, "POST", server.URL+"/api/auth/desktop/approve", map[string]string{"id": id}), 204)
	check(exchange(strings.Repeat("b", 43)), 403)
	check(exchange(verifier), 200)
	check(exchange(verifier), 410)
	check(doJSON(t, browser, "POST", server.URL+"/api/auth/logout", nil), 204)
	check(doJSON(t, desktop, "GET", server.URL+"/api/auth/me", nil), 200)
	check(doJSON(t, desktop, "POST", server.URL+"/api/auth/logout", nil), 204)
	check(doJSON(t, desktop, "GET", server.URL+"/api/auth/me", nil), 401)
	id = start()
	if _, err := db.ExecContext(context.Background(), "UPDATE desktop_logins SET expires_at=now()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	check(exchange(verifier), 410)
}
