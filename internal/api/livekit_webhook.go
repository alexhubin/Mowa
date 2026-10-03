package api

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/alexhubin/Mowa/internal/database/dbgen"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/webhook"
)

func (s *Server) liveKitWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	event, err := webhook.ReceiveWebhookEvent(r, auth.NewSimpleKeyProvider(s.cfg.LiveKitAPIKey, s.cfg.LiveKitAPISecret))
	if err != nil {
		slog.Warn("reject livekit webhook", "error", err)
		writeError(w, http.StatusUnauthorized, "Invalid LiveKit signature")
		return
	}
	if event.Room == nil || event.Room.Name == "" || event.Room.Sid == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch event.Event {
	case webhook.EventRoomStarted:
		if err := s.queries.MarkRoomStarted(r.Context(), dbgen.MarkRoomStartedParams{
			ID: event.Room.Name, LivekitRoomSid: sql.NullString{String: event.Room.Sid, Valid: true},
		}); err != nil {
			slog.Error("mark livekit room started", "room_id", event.Room.Name, "room_sid", event.Room.Sid, "error", err)
			writeError(w, http.StatusInternalServerError, "Could not save room state")
			return
		}
	case webhook.EventRoomFinished:
		deleted, err := s.queries.DeleteRoomByFinishedSession(r.Context(), dbgen.DeleteRoomByFinishedSessionParams{
			ID: event.Room.Name, LivekitRoomSid: sql.NullString{String: event.Room.Sid, Valid: true},
		})
		if err != nil {
			slog.Error("delete finished room", "room_id", event.Room.Name, "room_sid", event.Room.Sid, "error", err)
			writeError(w, http.StatusInternalServerError, "Could not remove ended room")
			return
		}
		if deleted > 0 {
			slog.Info("deleted finished room", "room_id", event.Room.Name, "room_sid", event.Room.Sid)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
