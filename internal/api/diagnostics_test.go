package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDesktopDiagnosticsAuthenticationValidationAndDedup(t *testing.T) {
	server, client, db := newTestServer(t)
	event := map[string]any{"id": strings.Repeat("a", 32), "call": strings.Repeat("b", 32), "kind": "call_start", "version": "0.1.0+test", "os": "windows", "arch": "x86_64"}
	send := func(want int) {
		t.Helper()
		r := doJSON(t, client, "POST", server.URL+"/api/desktop/diagnostics", event)
		defer r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("got %d want %d: %s", r.StatusCode, want, responseBody(t, r))
		}
	}
	send(401)
	user := provisionTestUser(t, db, "reports@example.com", "reports", "secure-password", "Reports", false)
	r := doJSON(t, client, "POST", server.URL+"/api/auth/login", map[string]string{"email": user.Email, "password": "secure-password"})
	r.Body.Close()
	send(204)
	send(204)
	var count int
	if err := db.QueryRow("SELECT count(*) FROM desktop_diagnostics WHERE user_id=$1", user.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dedup count %d err %v", count, err)
	}
	event["raw_log"] = "token=secret"
	send(400)
	delete(event, "raw_log")
	event["metrics"] = map[string]any{"ip_address": 123}
	send(400)
	event["metrics"] = map[string]any{"errors": -1}
	send(400)
	event["metrics"] = map[string]any{"errors": 3}
	event["id"] = strings.Repeat("c", 32)
	send(204)
	event["encoder"] = strings.Repeat("x", 5000)
	send(400)
	delete(event, "encoder")
	event["id"] = strings.Repeat("d", 32)
	// Exercise the database-backed limit, including concurrent-request serialization.
	_, err := db.Exec("INSERT INTO desktop_diagnostics(user_id,event_id,call_id,payload) SELECT $1, 'fixture-'||g::text, $2, '{}'::jsonb FROM generate_series(1,118) g", user.ID, event["call"])
	if err != nil {
		t.Fatal(err)
	}
	send(429)
	// No report-reading HTTP endpoint is exposed to another user.
	r = doJSON(t, client, "GET", server.URL+"/api/desktop/diagnostics", nil)
	defer r.Body.Close()
	if r.StatusCode != 405 {
		t.Fatal(r.StatusCode)
	}
}
func TestDiagnosticContract(t *testing.T) {
	var e diagnosticEvent
	if err := json.Unmarshal([]byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","call":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","kind":"call_end","version":"0.1.0+abc-dirty","os":"macos","arch":"aarch64","metrics":{"video_fps_avg":59.5,"errors":0}}`), &e); err != nil || !e.valid() {
		t.Fatal("valid summary rejected", err)
	}
	e.Code = "secret token"
	if e.valid() {
		t.Fatal("arbitrary error text accepted")
	}
}
