package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexhubin/Mowa/internal/database/dbgen"
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
	rows, err := s.queries.ListRoomMessages(r.Context(), room.ID)
	if err != nil {
		slog.Error("list room messages", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить сообщения")
		return
	}
	result := make([]roomMessageResponse, len(rows))
	for index, row := range rows {
		result[len(rows)-1-index] = roomMessageResponse{
			ID: row.ID, Body: row.Body, CreatedAt: row.CreatedAt,
			Author: messageAuthorResponse{ID: row.UserID, Username: row.Username, DisplayName: row.DisplayName},
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createRoomMessage(w http.ResponseWriter, r *http.Request) {
	room, ok := s.findRoom(w, r)
	if !ok {
		return
	}
	var input createRoomMessageRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if length := len([]rune(input.Body)); length == 0 || length > maxMessageLength {
		writeError(w, http.StatusUnprocessableEntity, "Сообщение должно содержать от 1 до 2000 символов")
		return
	}
	user := currentUser(r)
	message, err := s.queries.CreateRoomMessage(r.Context(), dbgen.CreateRoomMessageParams{
		ID: s.newID(), RoomID: room.ID, UserID: user.ID, Body: input.Body, CreatedAt: s.now(),
	})
	if err != nil {
		slog.Error("create room message", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отправить сообщение")
		return
	}
	s.messageEvents.notify(room.ID)
	writeJSON(w, http.StatusCreated, roomMessageResponse{
		ID: message.ID, Body: message.Body, CreatedAt: message.CreatedAt,
		Author: messageAuthorResponse{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName},
	})
}

type messageEventBroker struct {
	mu          sync.Mutex
	subscribers map[string]map[chan struct{}]struct{}
}

func newMessageEventBroker() *messageEventBroker {
	return &messageEventBroker{subscribers: make(map[string]map[chan struct{}]struct{})}
}

func (b *messageEventBroker) subscribe(roomID string) (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	b.mu.Lock()
	if b.subscribers[roomID] == nil {
		b.subscribers[roomID] = make(map[chan struct{}]struct{})
	}
	b.subscribers[roomID][updates] = struct{}{}
	b.mu.Unlock()
	return updates, func() {
		b.mu.Lock()
		delete(b.subscribers[roomID], updates)
		if len(b.subscribers[roomID]) == 0 {
			delete(b.subscribers, roomID)
		}
		b.mu.Unlock()
	}
}

func (b *messageEventBroker) notify(roomID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for updates := range b.subscribers[roomID] {
		select {
		case updates <- struct{}{}:
		default:
		}
	}
}

func (s *Server) streamRoomMessageEvents(w http.ResponseWriter, r *http.Request) {
	room, ok := s.findRoom(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Поток сообщений недоступен")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")

	updates, unsubscribe := s.messageEvents.subscribe(room.ID)
	defer unsubscribe()
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	controller := http.NewResponseController(w)
	writeDeadline := func() { _ = controller.SetWriteDeadline(time.Now().Add(35 * time.Second)) }
	writeEvent := func() bool {
		writeDeadline()
		if _, err := fmt.Fprint(w, "event: messages\ndata: {}\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !writeEvent() {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			if !writeEvent() {
				return
			}
		case <-keepAlive.C:
			writeDeadline()
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
