package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexhubin/Mowa/internal/database/dbgen"
	"github.com/go-chi/chi/v5"
)

type createCallRequest struct {
	UserID string `json:"user_id"`
}

type callResponse struct {
	ID         string             `json:"id"`
	Status     string             `json:"status"`
	InviteCode string             `json:"invite_code"`
	Peer       friendUserResponse `json:"peer"`
	Incoming   bool               `json:"incoming"`
	CreatedAt  time.Time          `json:"created_at"`
}

func (s *Server) listCalls(w http.ResponseWriter, r *http.Request) {
	s.expireStaleCalls(r)
	rows, err := s.queries.ListOpenCallsForUser(r.Context(), currentUser(r).ID)
	if err != nil {
		slog.Error("list calls", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить звонки")
		return
	}
	result := make([]callResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, callFromRow(row))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createDirectCall(w http.ResponseWriter, r *http.Request) {
	var input createCallRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	caller := currentUser(r)
	s.expireStaleCalls(r)
	if input.UserID == caller.ID || input.UserID == "" {
		writeError(w, http.StatusUnprocessableEntity, "Выберите друга для звонка")
		return
	}
	callee, err := s.queries.GetUserByID(r.Context(), input.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Пользователь не найден")
		return
	}
	if err != nil {
		slog.Error("get call target", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	isFriend, err := s.queries.IsFriend(r.Context(), dbgen.IsFriendParams{UserID: caller.ID, FriendID: callee.ID})
	if err != nil {
		slog.Error("check call friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	if !isFriend {
		writeError(w, http.StatusForbidden, "Звонить можно только друзьям")
		return
	}
	now := s.now()
	online, err := s.queries.IsUserOnline(r.Context(), dbgen.IsUserOnlineParams{UserID: callee.ID, ExpiresAt: now, LastSeenAt: now.Add(-presenceTTL)})
	if err != nil {
		slog.Error("check call presence", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось проверить статус пользователя")
		return
	}
	if !online {
		writeError(w, http.StatusConflict, "Пользователь не в сети")
		return
	}
	if _, err := s.queries.GetOpenCallForUser(r.Context(), caller.ID); err == nil {
		writeError(w, http.StatusConflict, "Сначала завершите текущий звонок")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check caller open call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	if _, err := s.queries.GetOpenCallForUser(r.Context(), callee.ID); err == nil {
		writeError(w, http.StatusConflict, "Пользователь уже участвует в другом звонке")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check callee open call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	if _, err := s.queries.GetOpenCallBetween(r.Context(), dbgen.GetOpenCallBetweenParams{CallerID: caller.ID, CalleeID: callee.ID}); err == nil {
		writeError(w, http.StatusConflict, "Между вами уже есть активный звонок")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check open call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	invite, err := s.newInvite()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	now = s.now()
	room, err := queries.CreateRoom(r.Context(), dbgen.CreateRoomParams{
		ID: s.newID(), InviteCode: invite, Name: caller.DisplayName + " × " + callee.DisplayName, OwnerID: caller.ID, Kind: "direct", CreatedAt: now,
	})
	if err != nil {
		slog.Error("create direct room", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	for _, userID := range []string{caller.ID, callee.ID} {
		if err := queries.AddRoomMember(r.Context(), dbgen.AddRoomMemberParams{RoomID: room.ID, UserID: userID, CreatedAt: now}); err != nil {
			slog.Error("add direct room member", "error", err)
			writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
			return
		}
	}
	call, err := queries.CreateDirectCall(r.Context(), dbgen.CreateDirectCallParams{ID: s.newID(), RoomID: room.ID, CallerID: caller.ID, CalleeID: callee.ID, CreatedAt: now})
	if err != nil {
		slog.Error("create direct call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error("commit direct call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать звонок")
		return
	}
	s.callEvents.notify(caller.ID, callee.ID)
	writeJSON(w, http.StatusCreated, callResponse{ID: call.ID, Status: call.Status, InviteCode: room.InviteCode, Peer: friendUserResponse{ID: callee.ID, Username: callee.Username, DisplayName: callee.DisplayName}, Incoming: false, CreatedAt: call.CreatedAt})
}

func (s *Server) acceptDirectCall(w http.ResponseWriter, r *http.Request) {
	call, err := s.queries.AcceptDirectCall(r.Context(), dbgen.AcceptDirectCallParams{ID: chi.URLParam(r, "callID"), CalleeID: currentUser(r).ID, AnsweredAt: sql.NullTime{Time: s.now(), Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusConflict, "Звонок уже завершён")
		return
	}
	if err != nil {
		slog.Error("accept call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось принять звонок")
		return
	}
	s.callEvents.notify(call.CallerID, call.CalleeID)
	writeCallByID(w, r, s, call.ID)
}

func (s *Server) declineDirectCall(w http.ResponseWriter, r *http.Request) {
	call, err := s.queries.DeclineDirectCall(r.Context(), dbgen.DeclineDirectCallParams{ID: chi.URLParam(r, "callID"), CalleeID: currentUser(r).ID, EndedAt: sql.NullTime{Time: s.now(), Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusConflict, "Звонок уже завершён")
		return
	}
	if err != nil {
		slog.Error("decline call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось отклонить звонок")
		return
	}
	s.callEvents.notify(call.CallerID, call.CalleeID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) endDirectCall(w http.ResponseWriter, r *http.Request) {
	call, err := s.queries.EndDirectCall(r.Context(), dbgen.EndDirectCallParams{ID: chi.URLParam(r, "callID"), CallerID: currentUser(r).ID, EndedAt: sql.NullTime{Time: s.now(), Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		slog.Error("end call", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось завершить звонок")
		return
	}
	s.callEvents.notify(call.CallerID, call.CalleeID)
	w.WriteHeader(http.StatusNoContent)
}

func writeCallByID(w http.ResponseWriter, r *http.Request, s *Server, callID string) {
	rows, err := s.queries.ListOpenCallsForUser(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить звонок")
		return
	}
	for _, row := range rows {
		if row.ID == callID {
			writeJSON(w, http.StatusOK, callFromRow(row))
			return
		}
	}
	writeError(w, http.StatusNotFound, "Звонок не найден")
}

func callFromRow(row dbgen.ListOpenCallsForUserRow) callResponse {
	return callResponse{
		ID: row.ID, Status: row.Status, InviteCode: row.InviteCode, Incoming: row.Incoming, CreatedAt: row.CreatedAt,
		Peer: friendUserResponse{ID: row.PeerID, Username: row.PeerUsername, DisplayName: row.PeerDisplayName},
	}
}

func (s *Server) expireStaleCalls(r *http.Request) {
	now := s.now()
	if err := s.queries.ExpireStaleCalls(r.Context(), dbgen.ExpireStaleCallsParams{
		EndedAt: sql.NullTime{Time: now, Valid: true}, CreatedAt: now.Add(-2 * time.Minute), CreatedAt_2: now.Add(-24 * time.Hour),
	}); err != nil {
		slog.Warn("expire stale calls", "error", err)
	}
}
