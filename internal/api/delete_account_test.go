package api

import (
	"testing"
	"time"
)

func TestDeleteAccountCascadesRevokesAllSessionsAndFreesSlot(t *testing.T) {
	server, browser, db := newTestServer(t)
	user := provisionTestUser(t, db, "alice@example.com", "alice", "secure-password", "alice", false)
	peer := provisionTestUser(t, db, "bob@example.com", "bob", "secure-password", "bob", false)
	desktop := newHTTPClient(t)
	for _, c := range []struct{ desktop bool }{{false}, {true}} {
		client := browser
		if c.desktop {
			client = desktop
		}
		assertStatus(t, doJSON(t, client, "POST", server.URL+"/api/auth/login", map[string]string{"email": user.Email, "password": "secure-password"}), 200)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO google_identities(subject,user_id) VALUES('google-alice',$1)", user.ID)
	exec("INSERT INTO rooms(id,invite_code,name,owner_id) VALUES('owned','owned','Owned',$1)", user.ID)
	exec("INSERT INTO rooms(id,invite_code,name,owner_id,kind) VALUES('peer_direct','peer_direct','Direct',$1,'direct')", peer.ID)
	exec("INSERT INTO direct_calls(id,room_id,caller_id,callee_id,status) VALUES('call','peer_direct',$1,$2,'active')", peer.ID, user.ID)
	exec("INSERT INTO direct_messages(id,sender_id,recipient_id,body) VALUES('msg1',$1,$2,'hello'),('msg2',$2,$1,'reply')", user.ID, peer.ID)
	exec("INSERT INTO room_messages(id,room_id,user_id,body) VALUES('roommsg','owned',$1,'hello')", peer.ID)
	exec("INSERT INTO friendships(user_id,friend_id) VALUES(LEAST($1::text,$2::text),GREATEST($1::text,$2::text))", user.ID, peer.ID)
	exec("INSERT INTO desktop_logins(id,challenge,user_id,expires_at) VALUES('pending','challenge',$1,$2)", user.ID, time.Now().Add(time.Minute))
	exec("INSERT INTO desktop_diagnostics(user_id,event_id,call_id,payload) VALUES($1,'event','call','{}')", user.ID)
	exec("INSERT INTO passkey_users(user_id,handle) VALUES($1,'hello')", user.ID)
	exec("INSERT INTO passkeys(id,user_id,credential_id,name,credential) VALUES('passkey',$1,'credential','key','{}')", user.ID)
	identity(t, db, desktop, server.URL, user.Email, "google-alice")
	assertStatus(t, doJSON(t, newHTTPClient(t), "DELETE", server.URL+"/api/account", map[string]string{"username": "alice"}), 401)
	assertStatus(t, doJSON(t, browser, "DELETE", server.URL+"/api/account", map[string]string{"username": "bob"}), 422)
	assertStatus(t, doJSON(t, browser, "GET", server.URL+"/api/auth/me", nil), 200)
	assertStatus(t, doJSON(t, browser, "DELETE", server.URL+"/api/account", map[string]string{"username": "alice"}), 204)
	for _, table := range []string{"sessions", "google_identities", "rooms", "direct_calls", "direct_messages", "room_messages", "friendships", "desktop_logins", "desktop_diagnostics", "passkeys", "passkey_users", "auth_flows"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var count int
	db.QueryRow("SELECT count(*) FROM users WHERE id=$1", peer.ID).Scan(&count)
	if count != 1 {
		t.Fatal("peer deleted")
	}
	assertStatus(t, doJSON(t, browser, "GET", server.URL+"/api/auth/me", nil), 401)
	assertStatus(t, doJSON(t, desktop, "GET", server.URL+"/api/auth/me", nil), 401)
	assertStatus(t, doJSON(t, desktop, "POST", server.URL+"/api/auth/identity", nil), 401)
	identity(t, db, browser, server.URL, "new@example.com", "")
	assertStatus(t, doJSON(t, browser, "POST", server.URL+"/api/auth/complete", map[string]string{"username": "new_user"}), 200)
}
