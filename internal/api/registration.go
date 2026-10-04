package api

import "net/http"

// Shared by all verified identity providers. Never expose this number in the UI.
const registrationAccountLimit = 2

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "Use email verification or Google to create an account")
}
