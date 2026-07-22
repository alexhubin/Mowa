package api

import (
	"net/http"
	"testing"
)

func TestPersistentAccountsFriendsAndDirectCall(t *testing.T) {
	server, annaClient, db := newTestServer(t)
	borisClient := newHTTPClient(t)
	provisionTestUser(t, db, "anna@example.com", "anna", "very-secure-password", "Анна", false)
	boris := provisionTestUser(t, db, "boris@example.com", "boris", "another-secure-password", "Борис", false)

	response := doJSON(t, annaClient, http.MethodPost, server.URL+"/api/auth/login", map[string]string{
		"email": "anna@example.com", "password": "very-secure-password",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login Anna status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, borisClient, http.MethodPost, server.URL+"/api/auth/login", map[string]string{
		"email": "boris@example.com", "password": "another-secure-password",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login Boris status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, annaClient, http.MethodPost, server.URL+"/api/friend-requests", map[string]string{"username": "boris"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("friend request status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, borisClient, http.MethodGet, server.URL+"/api/friends", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list friend requests status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var borisFriends friendsResponse
	decodeResponse(t, response, &borisFriends)
	if len(borisFriends.Incoming) != 1 || borisFriends.Incoming[0].User.Username != "anna" {
		t.Fatalf("unexpected incoming requests: %+v", borisFriends.Incoming)
	}

	response = doJSON(t, borisClient, http.MethodPost, server.URL+"/api/friend-requests/"+borisFriends.Incoming[0].ID+"/accept", nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("accept request status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, annaClient, http.MethodGet, server.URL+"/api/friends", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list friends status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var annaFriends friendsResponse
	decodeResponse(t, response, &annaFriends)
	if len(annaFriends.Friends) != 1 || annaFriends.Friends[0].ID != boris.ID || !annaFriends.Friends[0].Online {
		t.Fatalf("unexpected friends: %+v", annaFriends.Friends)
	}

	response = doJSON(t, borisClient, http.MethodPost, server.URL+"/api/auth/logout", nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("logout Boris status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, annaClient, http.MethodGet, server.URL+"/api/friends", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list offline friends status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	decodeResponse(t, response, &annaFriends)
	if len(annaFriends.Friends) != 1 || annaFriends.Friends[0].Online {
		t.Fatalf("Boris should be offline: %+v", annaFriends.Friends)
	}

	response = doJSON(t, annaClient, http.MethodPost, server.URL+"/api/calls", map[string]string{"user_id": boris.ID})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("offline call status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, borisClient, http.MethodPost, server.URL+"/api/auth/login", map[string]string{"email": "boris@example.com", "password": "another-secure-password"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login Boris status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, annaClient, http.MethodPost, server.URL+"/api/calls", map[string]string{"user_id": boris.ID})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create call status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var call callResponse
	decodeResponse(t, response, &call)
	if call.Status != "ringing" || call.Incoming || call.InviteCode == "" {
		t.Fatalf("unexpected outgoing call: %+v", call)
	}

	response = doJSON(t, borisClient, http.MethodGet, server.URL+"/api/calls", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list calls status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var calls []callResponse
	decodeResponse(t, response, &calls)
	if len(calls) != 1 || !calls[0].Incoming || calls[0].Peer.Username != "anna" {
		t.Fatalf("unexpected incoming calls: %+v", calls)
	}

	response = doJSON(t, borisClient, http.MethodPost, server.URL+"/api/calls/"+call.ID+"/accept", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("accept call status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	decodeResponse(t, response, &call)
	if call.Status != "active" {
		t.Fatalf("accepted call status = %q", call.Status)
	}

	response = doJSON(t, borisClient, http.MethodPost, server.URL+"/api/rooms/"+call.InviteCode+"/token", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("direct room token status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	response = doJSON(t, annaClient, http.MethodPut, server.URL+"/api/account/settings", map[string]string{"video_quality": "low"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("update settings status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var settings settingsResponse
	decodeResponse(t, response, &settings)
	if settings.VideoQuality != "low" {
		t.Fatalf("video quality = %q", settings.VideoQuality)
	}

	response = doJSON(t, annaClient, http.MethodPut, server.URL+"/api/account/settings", map[string]string{"video_quality": "medium"})
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("deprecated 15 fps quality status = %d", response.StatusCode)
	}
	response.Body.Close()

	response = doJSON(t, annaClient, http.MethodPatch, server.URL+"/api/account/profile", map[string]string{"username": "anna_voice", "display_name": "Анна Нова"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("update profile status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var anna userResponse
	decodeResponse(t, response, &anna)
	if anna.Username != "anna_voice" || anna.DisplayName != "Анна Нова" {
		t.Fatalf("unexpected profile: %+v", anna)
	}
}
