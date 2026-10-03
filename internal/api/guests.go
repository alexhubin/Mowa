package api

import "net/http"

// Room access always requires a full account; legacy guest cookies are ignored.
func (s *Server) requireRoomParticipant(next http.Handler) http.Handler {
	return s.requireUser(s.requirePasswordChanged(next))
}
