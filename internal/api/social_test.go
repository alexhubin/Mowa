package api

import (
	"net/http"
	"sync"
	"testing"
)

func TestPersistentAccountsFriendsAndDirectCall(t *testing.T) {
	server, annaClient, db := newTestServer(t)
	borisClient := newHTTPClient(t)
	annaUser := provisionTestUser(t, db, "anna@example.com", "anna", "very-secure-password", "Анна", false)
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

	response = doJSON(t, annaClient, http.MethodPost, server.URL+"/api/direct-messages/"+boris.ID, map[string]string{"body": "  Напишу, пока тебя нет  "})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("offline direct message status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var directMessage roomMessageResponse
	decodeResponse(t, response, &directMessage)
	if directMessage.Body != "Напишу, пока тебя нет" || directMessage.Author.Username != "anna" {
		t.Fatalf("unexpected direct message: %+v", directMessage)
	}

	response = doJSON(t, annaClient, http.MethodGet, server.URL+"/api/direct-messages/"+boris.ID, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list sent direct messages status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var directMessages []roomMessageResponse
	decodeResponse(t, response, &directMessages)
	if len(directMessages) != 1 || directMessages[0].ID != directMessage.ID {
		t.Fatalf("unexpected sent direct messages: %+v", directMessages)
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

	response = doJSON(t, borisClient, http.MethodGet, server.URL+"/api/direct-messages/"+annaUser.ID, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list received direct messages status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	decodeResponse(t, response, &directMessages)
	if len(directMessages) != 1 || directMessages[0].Body != "Напишу, пока тебя нет" {
		t.Fatalf("unexpected received direct messages: %+v", directMessages)
	}

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

	response = doJSON(t, annaClient, http.MethodPost, server.URL+"/api/rooms/"+call.InviteCode+"/messages", map[string]string{"body": "Message из личного звонка"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("direct call message status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var callMessage roomMessageResponse
	decodeResponse(t, response, &callMessage)

	response = doJSON(t, borisClient, http.MethodGet, server.URL+"/api/direct-messages/"+annaUser.ID, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("shared direct call history status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	decodeResponse(t, response, &directMessages)
	if len(directMessages) != 2 || directMessages[1].ID != callMessage.ID || directMessages[1].Body != "Message из личного звонка" {
		t.Fatalf("direct call did not share friend dialog: %+v", directMessages)
	}

	response = doJSON(t, annaClient, http.MethodPut, server.URL+"/api/account/settings", map[string]string{"video_quality": "low"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("update settings status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	var settings settingsResponse
	decodeResponse(t, response, &settings)
	if settings.VideoQuality != "low" {
		t.Fatalf("video quality = %q", settings.VideoQuality)
	}

	response = doJSON(t, annaClient, http.MethodPut, server.URL+"/api/account/settings", map[string]string{"video_quality": "original"})
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("removed original quality status = %d", response.StatusCode)
	}
	response.Body.Close()

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
	if anna.Username != "anna_voice" || anna.DisplayName != "anna_voice" {
		t.Fatalf("unexpected profile: %+v", anna)
	}
}

func TestConcurrentDirectCallsCreateOnlyOne(t *testing.T) {
	server, annaClient, db := newTestServer(t)
	borisClient := newHTTPClient(t)
	provisionTestUser(t, db, "anna@example.com", "anna", "very-secure-password", "Анна", false)
	boris := provisionTestUser(t, db, "boris@example.com", "boris", "another-secure-password", "Борис", false)

	for _, credentials := range []struct {
		client *http.Client
		email  string
		secret string
	}{
		{annaClient, "anna@example.com", "very-secure-password"},
		{borisClient, "boris@example.com", "another-secure-password"},
	} {
		response := doJSON(t, credentials.client, http.MethodPost, server.URL+"/api/auth/login", map[string]string{
			"email": credentials.email, "password": credentials.secret,
		})
		if response.StatusCode != http.StatusOK {
			t.Fatalf("login %s status = %d, body = %s", credentials.email, response.StatusCode, responseBody(t, response))
		}
		response.Body.Close()
	}

	response := doJSON(t, annaClient, http.MethodPost, server.URL+"/api/friend-requests", map[string]string{"username": "boris"})
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
	if len(borisFriends.Incoming) != 1 {
		t.Fatalf("unexpected incoming requests: %+v", borisFriends.Incoming)
	}
	response = doJSON(t, borisClient, http.MethodPost, server.URL+"/api/friend-requests/"+borisFriends.Incoming[0].ID+"/accept", nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("accept request status = %d, body = %s", response.StatusCode, responseBody(t, response))
	}
	response.Body.Close()

	const attempts = 8
	statuses := make([]int, attempts)
	start := make(chan struct{})
	var waiter sync.WaitGroup
	waiter.Add(attempts)
	for index := range attempts {
		go func() {
			defer waiter.Done()
			<-start
			attempt := doJSON(t, annaClient, http.MethodPost, server.URL+"/api/calls", map[string]string{"user_id": boris.ID})
			statuses[index] = attempt.StatusCode
			attempt.Body.Close()
		}()
	}
	close(start)
	waiter.Wait()

	created := 0
	for _, status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
		default:
			t.Fatalf("unexpected concurrent call statuses: %v", statuses)
		}
	}
	if created != 1 {
		t.Fatalf("concurrent calls created %d calls, want 1 (statuses %v)", created, statuses)
	}

	var open int
	if err := db.QueryRow("SELECT count(*) FROM direct_calls WHERE status IN ('ringing', 'active')").Scan(&open); err != nil {
		t.Fatalf("count open calls: %v", err)
	}
	if open != 1 {
		t.Fatalf("open calls in database = %d, want 1", open)
	}
}
