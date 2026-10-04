package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexhubin/Mowa/internal/auth"
)

func assertStatus(t *testing.T, r *http.Response, status int) {
	t.Helper()
	defer r.Body.Close()
	if r.StatusCode != status {
		t.Fatalf("status %d want %d: %s", r.StatusCode, status, responseBody(t, r))
	}
}
func identity(t *testing.T, db *sql.DB, client *http.Client, serverURL, email, subject string) {
	t.Helper()
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO auth_flows(token_hash,kind,email,subject,expires_at) VALUES($1,'identity',$2,$3,$4)`, hash, email, subject, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(serverURL)
	client.Jar.SetCookies(u, []*http.Cookie{{Name: flowCookie, Value: raw, Path: "/"}})
}
func TestVerifiedRegistrationAndSharedLimit(t *testing.T) {
	server, client, db := newTestServer(t)
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/register", map[string]string{}), 410)
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/complete", map[string]string{"username": "alice"}), 401)
	identity(t, db, client, server.URL, "alice@gmail.com", "")
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/complete", map[string]string{"username": "alice"}), 200)
	assertStatus(t, doJSON(t, client, "GET", server.URL+"/api/auth/me", nil), 200)
	// Proof was consumed; replay cannot register again.
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/complete", map[string]string{"username": "other"}), 401)
	// Google links to the same verified email and does not use another slot.
	identity(t, db, client, server.URL, "alice@gmail.com", "google-alice")
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/identity", nil), 200)
	var count int
	db.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
	clients := make([]*http.Client, 8)
	for i := range clients {
		clients[i] = newHTTPClient(t)
		subject := ""
		if i%2 == 0 {
			subject = fmt.Sprintf("google-%d", i)
		}
		identity(t, db, clients[i], server.URL, fmt.Sprintf("new%d@example.com", i), subject)
	}
	results := make(chan int, 8)
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *http.Client) {
			defer wg.Done()
			r := doJSON(t, c, "POST", server.URL+"/api/auth/complete", map[string]string{"username": fmt.Sprintf("user_%d", i)})
			defer r.Body.Close()
			results <- r.StatusCode
		}(i, c)
	}
	wg.Wait()
	close(results)
	created := 0
	for status := range results {
		if status == 200 {
			created++
		} else if status != 403 {
			t.Fatal(status)
		}
	}
	db.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if count != 2 || created != 1 {
		t.Fatalf("count=%d created=%d", count, created)
	}
	// Existing Google user can still sign in after registration closes, even if email changes.
	identity(t, db, client, server.URL, "changed@gmail.com", "google-alice")
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/identity", nil), 200)
}
func TestEmailCodesRateExpiryAttemptsReplayAndResend(t *testing.T) {
	var mu sync.Mutex
	var sent string
	server, client, db := newConfiguredTestServer(t, func(s *Server) {
		s.sendCode = func(_ context.Context, _ string, code string) error { mu.Lock(); sent = code; mu.Unlock(); return nil }
	})
	start := func() {
		assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/email/start", map[string]string{"email": "Alice@Example.com", "next": "//evil.test"}), 200)
	}
	code := func() string { mu.Lock(); defer mu.Unlock(); return sent }
	verify := func(c string) *http.Response {
		return doJSON(t, client, "POST", server.URL+"/api/auth/email/verify", map[string]string{"code": c})
	}
	start()
	first := code()
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/email/start", map[string]string{"email": "alice@example.com"}), 429)
	assertStatus(t, doJSON(t, newHTTPClient(t), "POST", server.URL+"/api/auth/email/verify", map[string]string{"code": first}), 401)
	for range 5 {
		assertStatus(t, verify("invalid"), 401)
	}
	assertStatus(t, verify(first), 401)
	db.Exec("DELETE FROM auth_rate_limits")
	start()
	assertStatus(t, verify(code()), 200)
	assertStatus(t, verify(code()), 401)
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/complete", map[string]string{"username": "alice"}), 200)
	db.Exec("DELETE FROM auth_rate_limits")
	start()
	db.Exec("UPDATE auth_flows SET expires_at=now()-interval '1 second'")
	assertStatus(t, verify(code()), 401)
	db.Exec("DELETE FROM auth_rate_limits")
	start()
	r := verify(code())
	var result struct {
		User *userResponse `json:"user"`
		Next string        `json:"next"`
	}
	decodeResponse(t, r, &result)
	if result.User == nil || result.User.Username != "alice" || result.Next != "/" {
		t.Fatalf("unexpected result %+v", result)
	}
}
func TestGoogleStatePKCEAndSingleUseCallback(t *testing.T) {
	var calls atomic.Int32
	server, client, _ := newConfiguredTestServer(t, func(s *Server) {
		s.cfg.GoogleClientID = "test"
		s.cfg.GoogleClientSecret = "secret"
		s.googleClaims = func(_ context.Context, code, verifier, nonce string) (string, string, error) {
			calls.Add(1)
			if code != "test-code" || verifier == "" || nonce == "" {
				t.Error("missing proof")
			}
			return "alice@gmail.com", "google-alice", nil
		}
	})
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	r, err := client.Get(server.URL + "/api/auth/google/start?next=%2Fdesktop-login%3Frequest%3Dtest")
	if err != nil {
		t.Fatal(err)
	}
	location, _ := url.Parse(r.Header.Get("Location"))
	assertStatus(t, r, 302)
	if location.Query().Get("code_challenge_method") != "S256" || location.Query().Get("nonce") == "" {
		t.Fatal(location)
	}
	state := location.Query().Get("state")
	callback := server.URL + "/api/auth/google/callback?code=test-code&state=" + url.QueryEscape(state)
	other := newHTTPClient(t)
	other.CheckRedirect = client.CheckRedirect
	r, err = other.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, r, 303)
	if calls.Load() != 0 {
		t.Fatal("accepted missing cookie")
	}
	r, err = client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, r, 303)
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/identity", nil), 200)
	r, err = client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, r, 303)
	if calls.Load() != 1 {
		t.Fatal("callback replay")
	}
}
func TestSafeNext(t *testing.T) {
	for _, raw := range []string{"https://evil.test", "//evil.test", "/\\evil.test", "/\r\nevil"} {
		if safeNext(raw) != "/" {
			t.Fatal(raw)
		}
	}
	if safeNext("/desktop-login?request=ok") != "/desktop-login?request=ok" {
		t.Fatal("lost desktop request")
	}
}
