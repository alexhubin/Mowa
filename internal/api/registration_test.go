package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestRegistrationCreatesUsableAccountAndRejectsDuplicates(t *testing.T) {
	server, client, db := newTestServer(t)
	input := map[string]string{"email": "new@example.com", "username": "new_user", "display_name": "New", "password": "secure-password"}
	response := doJSON(t, client, "POST", server.URL+"/api/auth/register", input)
	if response.StatusCode != 201 {
		t.Fatal(response.StatusCode, responseBody(t, response))
	}
	var user userResponse
	decodeResponse(t, response, &user)
	if user.DisplayName != "new_user" {
		t.Fatalf("display name must match username: %q", user.DisplayName)
	}
	if user.MustChangePassword {
		t.Fatal("self-registered password must be usable")
	}
	response = doJSON(t, client, "POST", server.URL+"/api/rooms", map[string]string{"name": "Test room"})
	if response.StatusCode != 201 {
		t.Fatal(response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()
	response = doJSON(t, client, "POST", server.URL+"/api/auth/register", input)
	if response.StatusCode != 409 {
		t.Fatal(response.StatusCode)
	}
	response.Body.Close()
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Fatalf("users %d %v", count, err)
	}
}
func TestConcurrentRegistrationsCannotExceedAccountLimit(t *testing.T) {
	server, _, db := newTestServer(t)
	_, err := db.ExecContext(context.Background(), `INSERT INTO users (id,username,email,display_name,password_hash)
 SELECT 'seed_'||n,'seed_'||n,'seed_'||n||'@example.com','Seed','unused' FROM generate_series(1,$1::int) n`, registrationAccountLimit-1)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func(i int) {
			body, _ := json.Marshal(map[string]string{"email": fmt.Sprintf("new%d@example.com", i), "username": fmt.Sprintf("new_%d", i), "display_name": "New", "password": "secure-password"})
			r, err := http.Post(server.URL+"/api/auth/register", "application/json", bytes.NewReader(body))
			if err != nil {
				results <- 0
				return
			}
			defer r.Body.Close()
			results <- r.StatusCode
		}(i)
	}
	created := 0
	for i := 0; i < 8; i++ {
		status := <-results
		if status == 201 {
			created++
		} else if status != 403 {
			t.Fatalf("unexpected status %d", status)
		}
	}
	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM users").Scan(&count); err != nil || count != registrationAccountLimit || created != 1 {
		t.Fatalf("created=%d count=%d err=%v", created, count, err)
	}
	// More existing accounts (e.g. after lowering the constant) also close registration.
	_, err = db.ExecContext(context.Background(), `INSERT INTO users(id,username,email,display_name,password_hash) VALUES('extra','extra','extra@example.com','Extra','unused')`)
	if err != nil {
		t.Fatal(err)
	}
	r := doJSON(t, newHTTPClient(t), "POST", server.URL+"/api/auth/register", map[string]string{"email": "blocked@example.com", "username": "blocked", "display_name": "Blocked", "password": "secure-password"})
	if r.StatusCode != 403 {
		t.Fatal(r.StatusCode)
	}
	body := responseBody(t, r)
	if !strings.Contains(body, "Registration is temporarily closed") || strings.Contains(body, "10") {
		t.Fatal(body)
	}
}
