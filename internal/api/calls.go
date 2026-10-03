package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexhubin/Mowa/internal/database/dbgen"
	"github.com/go-chi/chi/v5"
)

const (
	ringingCallTTL     = 2 * time.Minute
	activeCallTTL      = 24 * time.Hour
	callExpiryInterval = 30 * time.Second
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
	rows, err := s.queries.ListOpenCallsForUser(r.Context(), currentUser(r).ID)
	if err != nil {
		slog.Error("list calls", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not load calls")
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
	if input.UserID == caller.ID || input.UserID == "" {
		writeError(w, http.StatusUnprocessableEntity, "Choose a friend to call")
		return
	}
	callee, err := s.queries.GetUserByID(r.Context(), input.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}
	if err != nil {
		slog.Error("get call target", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	isFriend, err := s.queries.IsFriend(r.Context(), dbgen.IsFriendParams{UserID: caller.ID, FriendID: callee.ID})
	if err != nil {
		slog.Error("check call friendship", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	if !isFriend {
		writeError(w, http.StatusForbidden, "You can only call friends")
		return
	}
	now := s.now()
	online, err := s.queries.IsUserOnline(r.Context(), dbgen.IsUserOnlineParams{UserID: callee.ID, ExpiresAt: now, LastSeenAt: now.Add(-presenceTTL)})
	if err != nil {
		slog.Error("check call presence", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not check user status")
		return
	}
	if !online {
		writeError(w, http.StatusConflict, "User is offline")
		return
	}
	invite, err := s.newInvite()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	if _, err := queries.GetOpenCallForUser(r.Context(), caller.ID); err == nil {
		writeError(w, http.StatusConflict, "End your current call first")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check caller open call", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	if _, err := queries.GetOpenCallForUser(r.Context(), callee.ID); err == nil {
		writeError(w, http.StatusConflict, "User is already in another call")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check callee open call", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	if _, err := queries.GetOpenCallBetween(r.Context(), dbgen.GetOpenCallBetweenParams{CallerID: caller.ID, CalleeID: callee.ID}); err == nil {
		writeError(w, http.StatusConflict, "You already have an active call together")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check open call", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	now = s.now()
	room, err := queries.CreateRoom(r.Context(), dbgen.CreateRoomParams{
		ID: s.newID(), InviteCode: invite, Name: caller.DisplayName + " × " + callee.DisplayName, OwnerID: caller.ID, Kind: "direct", CreatedAt: now,
	})
	if err != nil {
		slog.Error("create direct room", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	for _, userID := range []string{caller.ID, callee.ID} {
		if err := queries.AddRoomMember(r.Context(), dbgen.AddRoomMemberParams{RoomID: room.ID, UserID: userID, CreatedAt: now}); err != nil {
			slog.Error("add direct room member", "error", err)
			writeError(w, http.StatusInternalServerError, "Could not start call")
			return
		}
	}
	call, err := queries.CreateDirectCall(r.Context(), dbgen.CreateDirectCallParams{ID: s.newID(), RoomID: room.ID, CallerID: caller.ID, CalleeID: callee.ID, CreatedAt: now})
	if err != nil {
		slog.Error("create direct call", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	for _, participant := range []struct {
		userID  string
		message string
	}{
		{caller.ID, "End your current call first"},
		{callee.ID, "User is already in another call"},
	} {
		err := queries.RegisterOpenCallParticipant(r.Context(), dbgen.RegisterOpenCallParticipantParams{UserID: participant.userID, CallID: call.ID, CreatedAt: now})
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, participant.message)
			return
		}
		if err != nil {
			slog.Error("register open call participant", "error", err)
			writeError(w, http.StatusInternalServerError, "Could not start call")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		slog.Error("commit direct call", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not start call")
		return
	}
	s.callEvents.notify(caller.ID, callee.ID)
	writeJSON(w, http.StatusCreated, callResponse{ID: call.ID, Status: call.Status, InviteCode: room.InviteCode, Peer: friendUserResponse{ID: callee.ID, Username: callee.Username, DisplayName: callee.DisplayName}, Incoming: false, CreatedAt: call.CreatedAt})
}

func (s *Server) acceptDirectCall(w http.ResponseWriter, r *http.Request) {
	call, err := s.queries.AcceptDirectCall(r.Context(), dbgen.AcceptDirectCallParams{ID: chi.URLParam(r, "callID"), CalleeID: currentUser(r).ID, AnsweredAt: sql.NullTime{Time: s.now(), Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusConflict, "Call has already ended")
		return
	}
	if err != nil {
		slog.Error("accept call", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not accept call")
		return
	}
	s.callEvents.notify(call.CallerID, call.CalleeID)
	writeCallByID(w, r, s, call.ID)
}

func (s *Server) declineDirectCall(w http.ResponseWriter, r *http.Request) {
	call, err := s.queries.DeclineDirectCall(r.Context(), dbgen.DeclineDirectCallParams{ID: chi.URLParam(r, "callID"), CalleeID: currentUser(r).ID, EndedAt: sql.NullTime{Time: s.now(), Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusConflict, "Call has already ended")
		return
	}
	if err != nil {
		slog.Error("decline call", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not decline call")
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
		writeError(w, http.StatusInternalServerError, "Could not end call")
		return
	}
	s.callEvents.notify(call.CallerID, call.CalleeID)
	w.WriteHeader(http.StatusNoContent)
}

func writeCallByID(w http.ResponseWriter, r *http.Request, s *Server, callID string) {
	rows, err := s.queries.ListOpenCallsForUser(r.Context(), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load call")
		return
	}
	for _, row := range rows {
		if row.ID == callID {
			writeJSON(w, http.StatusOK, callFromRow(row))
			return
		}
	}
	writeError(w, http.StatusNotFound, "Call not found")
}

func callFromRow(row dbgen.ListOpenCallsForUserRow) callResponse {
	return callResponse{
		ID: row.ID, Status: row.Status, InviteCode: row.InviteCode, Incoming: row.Incoming, CreatedAt: row.CreatedAt,
		Peer: friendUserResponse{ID: row.PeerID, Username: row.PeerUsername, DisplayName: row.PeerDisplayName},
	}
}

func (s *Server) expireStaleCalls(ctx context.Context) {
	now := s.now()
	expired, err := s.queries.ExpireStaleCalls(ctx, dbgen.ExpireStaleCallsParams{
		EndedAt: sql.NullTime{Time: now, Valid: true}, CreatedAt: now.Add(-ringingCallTTL), CreatedAt_2: now.Add(-activeCallTTL),
	})
	if err != nil {
		slog.Warn("expire stale calls", "error", err)
		return
	}
	for _, call := range expired {
		s.callEvents.notify(call.CallerID, call.CalleeID)
	}
}

func (s *Server) StartBackgroundJobs(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(callExpiryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.expireStaleCalls(ctx)
			}
		}
	}()
}
