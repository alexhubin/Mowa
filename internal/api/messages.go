package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexhubin/Mowa/internal/database/dbgen"
	"github.com/go-chi/chi/v5"
)

const maxMessageLength = 2000

type createRoomMessageRequest struct {
	Body string `json:"body"`
}

type messageAuthorResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type roomMessageResponse struct {
	ID        string                `json:"id"`
	Body      string                `json:"body"`
	Author    messageAuthorResponse `json:"author"`
	CreatedAt time.Time             `json:"created_at"`
}

func (s *Server) listRoomMessages(w http.ResponseWriter, r *http.Request) {
	room, ok := s.findRoom(w, r)
	if !ok {
		return
	}
	if room.Kind == "direct" {
		userID := currentUser(r).ID
		peerID, ok := s.directRoomPeerID(w, r, room.ID, userID)
		if ok {
			s.writeDirectMessages(w, r, userID, peerID)
		}
		return
	}
	rows, err := s.queries.ListRoomMessages(r.Context(), room.ID)
	if err != nil {
		slog.Error("list room messages", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not load messages")
		return
	}
	result := make([]roomMessageResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, roomMessageResponse{
			ID: row.ID, Body: row.Body, CreatedAt: row.CreatedAt,
			Author: messageAuthorResponse{ID: row.AuthorID, Username: row.Username, DisplayName: row.DisplayName},
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createRoomMessage(w http.ResponseWriter, r *http.Request) {
	room, ok := s.findRoom(w, r)
	if !ok {
		return
	}
	body, ok := decodeMessageBody(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	if room.Kind == "direct" {
		peerID, ok := s.directRoomPeerID(w, r, room.ID, user.ID)
		if ok {
			s.createDirectMessageFor(w, r, user, peerID, body)
		}
		return
	}
	userID := sql.NullString{String: user.ID, Valid: true}
	guestID := sql.NullString{}
	message, err := s.queries.CreateRoomMessage(r.Context(), dbgen.CreateRoomMessageParams{
		ID: s.newID(), RoomID: room.ID, UserID: userID, GuestID: guestID, Body: body, CreatedAt: s.now(),
	})
	if err != nil {
		slog.Error("create room message", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not send message")
		return
	}
	s.messageEvents.notify("room:" + room.ID)
	writeJSON(w, http.StatusCreated, roomMessageResponse{
		ID: message.ID, Body: message.Body, CreatedAt: message.CreatedAt,
		Author: messageAuthorResponse{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName},
	})
}

func (s *Server) streamRoomMessageEvents(w http.ResponseWriter, r *http.Request) {
	room, ok := s.findRoom(w, r)
	if !ok {
		return
	}
	if room.Kind == "direct" {
		userID := currentUser(r).ID
		if _, ok := s.directRoomPeerID(w, r, room.ID, userID); ok {
			s.streamMessageEvents(w, r, "user:"+userID)
		}
		return
	}
	s.streamMessageEvents(w, r, "room:"+room.ID)
}

func (s *Server) listDirectMessages(w http.ResponseWriter, r *http.Request) {
	userID := currentUser(r).ID
	friendID := chi.URLParam(r, "userID")
	if !s.requireFriend(w, r, userID, friendID) {
		return
	}
	s.writeDirectMessages(w, r, userID, friendID)
}

func (s *Server) writeDirectMessages(w http.ResponseWriter, r *http.Request, userID, friendID string) {
	rows, err := s.queries.ListDirectMessages(r.Context(), dbgen.ListDirectMessagesParams{
		SenderID: userID, RecipientID: friendID,
	})
	if err != nil {
		slog.Error("list direct messages", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not load messages")
		return
	}
	result := make([]roomMessageResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, roomMessageResponse{
			ID: row.ID, Body: row.Body, CreatedAt: row.CreatedAt,
			Author: messageAuthorResponse{ID: row.SenderID, Username: row.Username, DisplayName: row.DisplayName},
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createDirectMessage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	friendID := chi.URLParam(r, "userID")
	if !s.requireFriend(w, r, user.ID, friendID) {
		return
	}
	body, ok := decodeMessageBody(w, r)
	if !ok {
		return
	}
	s.createDirectMessageFor(w, r, user, friendID, body)
}

func (s *Server) createDirectMessageFor(w http.ResponseWriter, r *http.Request, user dbgen.User, friendID, body string) {
	message, err := s.queries.CreateDirectMessage(r.Context(), dbgen.CreateDirectMessageParams{
		ID: s.newID(), SenderID: user.ID, RecipientID: friendID, Body: body, CreatedAt: s.now(),
	})
	if err != nil {
		slog.Error("create direct message", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not send message")
		return
	}
	s.messageEvents.notify("user:" + user.ID)
	s.messageEvents.notify("user:" + friendID)
	writeJSON(w, http.StatusCreated, roomMessageResponse{
		ID: message.ID, Body: message.Body, CreatedAt: message.CreatedAt,
		Author: messageAuthorResponse{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName},
	})
}

func decodeMessageBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var input createRoomMessageRequest
	if !decodeJSON(w, r, &input) {
		return "", false
	}
	body := strings.TrimSpace(input.Body)
	if length := len([]rune(body)); length == 0 || length > maxMessageLength {
		writeError(w, http.StatusUnprocessableEntity, "Message must be 1–2000 characters")
		return "", false
	}
	return body, true
}

func (s *Server) directRoomPeerID(w http.ResponseWriter, r *http.Request, roomID, userID string) (string, bool) {
	peerID, err := s.queries.GetDirectRoomPeerID(r.Context(), dbgen.GetDirectRoomPeerIDParams{RoomID: roomID, CallerID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Direct call not found")
		return "", false
	}
	if err != nil {
		slog.Error("get direct room peer", "room_id", roomID, "error", err)
		writeError(w, http.StatusInternalServerError, "Could not load conversation")
		return "", false
	}
	return peerID, true
}

func (s *Server) streamDirectMessageEvents(w http.ResponseWriter, r *http.Request) {
	userID := currentUser(r).ID
	if !s.requireFriend(w, r, userID, chi.URLParam(r, "userID")) {
		return
	}
	s.streamMessageEvents(w, r, "user:"+userID)
}

func (s *Server) requireFriend(w http.ResponseWriter, r *http.Request, userID, friendID string) bool {
	if friendID == "" || friendID == userID {
		writeError(w, http.StatusForbidden, "Direct messages are only available between friends")
		return false
	}
	isFriend, err := s.queries.IsFriend(r.Context(), dbgen.IsFriendParams{UserID: userID, FriendID: friendID})
	if err != nil {
		slog.Error("check direct message friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not verify friends list")
		return false
	}
	if !isFriend {
		writeError(w, http.StatusForbidden, "Direct messages are only available between friends")
		return false
	}
	return true
}

func (s *Server) streamMessageEvents(w http.ResponseWriter, r *http.Request, scope string) {
	streamEvents(w, r, s.messageEvents, scope, "messages")
}
