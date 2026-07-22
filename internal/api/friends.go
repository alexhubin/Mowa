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

type friendUserResponse struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Relationship string `json:"relationship,omitempty"`
	Online       bool   `json:"online"`
}

type friendRequestResponse struct {
	ID        string             `json:"id"`
	User      friendUserResponse `json:"user"`
	CreatedAt time.Time          `json:"created_at"`
}

type friendsResponse struct {
	Friends  []friendUserResponse    `json:"friends"`
	Incoming []friendRequestResponse `json:"incoming"`
	Outgoing []friendRequestResponse `json:"outgoing"`
}

type createFriendRequestInput struct {
	Username string `json:"username"`
}

func (s *Server) searchUsers(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 || len([]rune(query)) > 40 {
		writeError(w, http.StatusUnprocessableEntity, "Введите минимум 2 символа")
		return
	}
	users, err := s.queries.SearchUsers(r.Context(), dbgen.SearchUsersParams{UserID: currentUser(r).ID, Lower: query})
	if err != nil {
		slog.Error("search users", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось выполнить поиск")
		return
	}
	result := make([]friendUserResponse, 0, len(users))
	for _, user := range users {
		result = append(result, friendUserResponse{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Relationship: user.Relationship})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listFriends(w http.ResponseWriter, r *http.Request) {
	userID := currentUser(r).ID
	now := s.now()
	friends, err := s.queries.ListFriends(r.Context(), dbgen.ListFriendsParams{UserID: userID, ExpiresAt: now, LastSeenAt: now.Add(-presenceTTL)})
	if err != nil {
		slog.Error("list friends", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить друзей")
		return
	}
	incoming, err := s.queries.ListIncomingFriendRequests(r.Context(), userID)
	if err != nil {
		slog.Error("list incoming friend requests", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить заявки")
		return
	}
	outgoing, err := s.queries.ListOutgoingFriendRequests(r.Context(), userID)
	if err != nil {
		slog.Error("list outgoing friend requests", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить заявки")
		return
	}

	response := friendsResponse{
		Friends:  make([]friendUserResponse, 0, len(friends)),
		Incoming: make([]friendRequestResponse, 0, len(incoming)),
		Outgoing: make([]friendRequestResponse, 0, len(outgoing)),
	}
	for _, friend := range friends {
		response.Friends = append(response.Friends, friendUserResponse{ID: friend.ID, Username: friend.Username, DisplayName: friend.DisplayName, Online: friend.Online})
	}
	for _, request := range incoming {
		response.Incoming = append(response.Incoming, friendRequestResponse{ID: request.ID, CreatedAt: request.CreatedAt, User: friendUserResponse{ID: request.UserID, Username: request.Username, DisplayName: request.DisplayName}})
	}
	for _, request := range outgoing {
		response.Outgoing = append(response.Outgoing, friendRequestResponse{ID: request.ID, CreatedAt: request.CreatedAt, User: friendUserResponse{ID: request.UserID, Username: request.Username, DisplayName: request.DisplayName}})
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) createFriendRequest(w http.ResponseWriter, r *http.Request) {
	var input createFriendRequestInput
	if !decodeJSON(w, r, &input) {
		return
	}
	target, err := s.queries.GetUserByUsername(r.Context(), normalizeUsername(input.Username))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	if err != nil {
		slog.Error("find friend target", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отправить заявку")
		return
	}
	userID := currentUser(r).ID
	if target.ID == userID {
		writeError(w, http.StatusUnprocessableEntity, "Нельзя добавить самого себя")
		return
	}
	isFriend, err := s.queries.IsFriend(r.Context(), dbgen.IsFriendParams{UserID: userID, FriendID: target.ID})
	if err != nil {
		slog.Error("check friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отправить заявку")
		return
	}
	if isFriend {
		writeError(w, http.StatusConflict, "Вы уже друзья")
		return
	}
	if _, err := s.queries.GetFriendRequestBetween(r.Context(), dbgen.GetFriendRequestBetweenParams{SenderID: userID, ReceiverID: target.ID}); err == nil {
		writeError(w, http.StatusConflict, "Заявка уже существует")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check friend request", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отправить заявку")
		return
	}
	request, err := s.queries.CreateFriendRequest(r.Context(), dbgen.CreateFriendRequestParams{
		ID: s.newID(), SenderID: userID, ReceiverID: target.ID, CreatedAt: s.now(),
	})
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "Заявка уже существует")
		return
	}
	if err != nil {
		slog.Error("create friend request", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отправить заявку")
		return
	}
	writeJSON(w, http.StatusCreated, friendRequestResponse{ID: request.ID, CreatedAt: request.CreatedAt, User: friendUserResponse{ID: target.ID, Username: target.Username, DisplayName: target.DisplayName}})
}

func (s *Server) acceptFriendRequest(w http.ResponseWriter, r *http.Request) {
	userID := currentUser(r).ID
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось принять заявку")
		return
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	request, err := queries.GetFriendRequestForReceiver(r.Context(), dbgen.GetFriendRequestForReceiverParams{ID: chi.URLParam(r, "requestID"), ReceiverID: userID})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Заявка не найдена")
		return
	}
	if err != nil {
		slog.Error("get friend request", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось принять заявку")
		return
	}
	first, second := request.SenderID, request.ReceiverID
	if first > second {
		first, second = second, first
	}
	if err := queries.CreateFriendship(r.Context(), dbgen.CreateFriendshipParams{UserID: first, FriendID: second, CreatedAt: s.now()}); err != nil {
		slog.Error("create friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось принять заявку")
		return
	}
	if err := queries.DeleteFriendRequest(r.Context(), request.ID); err != nil {
		slog.Error("delete accepted request", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось принять заявку")
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error("commit friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось принять заявку")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) declineFriendRequest(w http.ResponseWriter, r *http.Request) {
	request, err := s.queries.GetFriendRequestForReceiver(r.Context(), dbgen.GetFriendRequestForReceiverParams{ID: chi.URLParam(r, "requestID"), ReceiverID: currentUser(r).ID})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Заявка не найдена")
		return
	}
	if err != nil {
		slog.Error("get declined request", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отклонить заявку")
		return
	}
	if err := s.queries.DeleteFriendRequest(r.Context(), request.ID); err != nil {
		slog.Error("delete friend request", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отклонить заявку")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteFriend(w http.ResponseWriter, r *http.Request) {
	if err := s.queries.DeleteFriendship(r.Context(), dbgen.DeleteFriendshipParams{UserID: currentUser(r).ID, FriendID: chi.URLParam(r, "userID")}); err != nil {
		slog.Error("delete friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось удалить друга")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
