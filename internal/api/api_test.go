package api

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	internalAuth "github.com/alexhubin/Mova/internal/auth"
	"github.com/alexhubin/Mova/internal/config"
	"github.com/alexhubin/Mova/internal/database"
	"github.com/google/uuid"
	livekitAuth "github.com/livekit/protocol/auth"
)

func TestAuthRoomAndLiveKitTokenFlow(t *testing.T) {
	server, client, db := newTestServer(t)
	provisionTestUser(t, db, "anna@example.com", "anna", "very-secure-password", "Анна", true)

	response := doJSON(t, client, http.MethodPost, server.URL+"/api/auth/login", map[string]string{
		"email": "anna@example.com", "password": "very-secure-password",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var loginUser userResponse
	decodeResponse(t, response, &loginUser)
	if !loginUser.MustChangePassword {
		t.Fatal("temporary account must require a password change")
	}

	response = doJSON(t, client, http.MethodPost, server.URL+"/api/rooms", map[string]string{"name": "Blocked"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("room before first password status = %d", response.StatusCode)
	}
	response.Body.Close()

	response = doJSON(t, client, http.MethodPut, server.URL+"/api/auth/first-password", map[string]string{"new_password": "new-secure-password"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("first password status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/calls/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	response.Body.Close()
	if err != nil || line != "event: calls\n" {
		t.Fatalf("initial call event = %q, error = %v", line, err)
	}

	response = doJSON(t, client, http.MethodGet, server.URL+"/api/auth/me", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var me userResponse
	decodeResponse(t, response, &me)
	if me.Email != "anna@example.com" || me.DisplayName != "Анна" || me.Username != "anna" || me.MustChangePassword {
		t.Fatalf("unexpected me response: %+v", me)
	}

	response = doJSON(t, client, http.MethodPost, server.URL+"/api/presence", nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("presence status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, client, http.MethodPost, server.URL+"/api/rooms", map[string]string{"name": "Команда Mova"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create room status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var room roomResponse
	decodeResponse(t, response, &room)
	if room.Name != "Команда Mova" || len(room.InviteCode) < 8 {
		t.Fatalf("unexpected room response: %+v", room)
	}

	response = doJSON(t, client, http.MethodPost, server.URL+"/api/rooms/"+room.InviteCode+"/token", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var tokenResponse struct {
		Token     string `json:"token"`
		ServerURL string `json:"server_url"`
		ExpiresIn int    `json:"expires_in"`
	}
	decodeResponse(t, response, &tokenResponse)
	if tokenResponse.ServerURL != "ws://livekit.test:7880" || tokenResponse.ExpiresIn != 600 {
		t.Fatalf("unexpected token response: %+v", tokenResponse)
	}
	verifier, err := livekitAuth.ParseAPIToken(tokenResponse.Token)
	if err != nil {
		t.Fatalf("parse LiveKit token: %v", err)
	}
	_, grants, err := verifier.Verify("secretsecretsecretsecretsecretsecret")
	if err != nil {
		t.Fatalf("verify LiveKit token: %v", err)
	}
	if verifier.Identity() != me.ID || grants.Video == nil || !grants.Video.RoomJoin || grants.Video.Room != room.ID || grants.Video.GetCanPublishData() {
		t.Fatalf("unexpected LiveKit grants: %+v", grants.Video)
	}

	response = doJSON(t, client, http.MethodPost, server.URL+"/api/auth/logout", nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d", response.StatusCode)
	}
	response.Body.Close()

	response = doJSON(t, client, http.MethodGet, server.URL+"/api/auth/me", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func TestProtectedEndpointsAndOrigin(t *testing.T) {
	server, client, _ := newTestServer(t)

	response := doJSON(t, client, http.MethodPost, server.URL+"/api/auth/register", map[string]string{})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("public register status = %d, want 404", response.StatusCode)
	}
	response.Body.Close()

	response = doJSON(t, client, http.MethodPost, server.URL+"/api/rooms", map[string]string{"name": "Private"})
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized create room status = %d", response.StatusCode)
	}
	response.Body.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/login", bytes.NewBufferString(`{"email":"a@b.co","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://evil.example")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin login status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func newTestServer(t *testing.T) (*httptest.Server, *http.Client, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL API integration tests")
	}
	db, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(context.Background(), "TRUNCATE direct_calls, friendships, friend_requests, room_members, rooms, sessions, user_settings, users CASCADE"); err != nil {
		t.Fatalf("reset test database: %v", err)
	}

	cfg := config.Config{
		AppOrigin: "http://localhost", CookieSecure: false,
		LiveKitURL: "ws://livekit.test:7880", LiveKitAPIKey: "devkey",
		LiveKitAPISecret: "secretsecretsecretsecretsecretsecret", LiveKitTokenTTL: 10 * time.Minute,
	}
	server := httptest.NewServer(New(db, cfg).Handler())
	t.Cleanup(server.Close)
	return server, newHTTPClient(t), db
}

func provisionTestUser(t *testing.T, db *sql.DB, email, username, password, displayName string, mustChangePassword bool) userResponse {
	t.Helper()
	hash, err := internalAuth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := userResponse{ID: uuid.NewString(), Email: email, Username: username, DisplayName: displayName, MustChangePassword: mustChangePassword}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO users (id, username, email, display_name, password_hash, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6)`, user.ID, username, email, displayName, hash, mustChangePassword); err != nil {
		t.Fatalf("provision test user: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO user_settings (user_id) VALUES ($1)`, user.ID); err != nil {
		t.Fatalf("provision test settings: %v", err)
	}
	return user
}

func newHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 5 * time.Second}
}

func doJSON(t *testing.T, client *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, url, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func responseBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	var value any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return err.Error()
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
