package api

import (
	"net/http"
	"net/mail"
	"strings"
	"unicode"

	"github.com/alexhubin/Mowa/internal/auth"
	"github.com/alexhubin/Mowa/internal/database/dbgen"
)

// Includes existing accounts. Lowering this closes registration without deleting users.
const registrationAccountLimit = 10

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email       string `json:"email"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.TrimSpace(input.Email)
	input.Username = normalizeUsername(input.Username)
	input.DisplayName = input.Username
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 || !usernamePattern.MatchString(input.Username) || len([]rune(input.DisplayName)) < 1 || len([]rune(input.DisplayName)) > 40 || strings.ContainsFunc(input.DisplayName, unicode.IsControl) || len(input.Password) < 8 || len(input.Password) > 128 {
		writeError(w, 422, "Check your email, username and password (8–128 characters)")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	defer tx.Rollback()
	// A table lock also serializes registrations against administrator INSERTs.
	if _, err = tx.ExecContext(r.Context(), "LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	var count int
	if err = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	if count >= registrationAccountLimit {
		writeError(w, http.StatusForbidden, "Registration is temporarily closed")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	q := s.queries.WithTx(tx)
	now := s.now()
	user, err := q.CreateUser(r.Context(), dbgen.CreateUserParams{ID: s.newID(), Username: input.Username, Email: input.Email, DisplayName: input.DisplayName, PasswordHash: hash, CreatedAt: now})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, 409, "Email or username is already taken")
		} else {
			writeError(w, 500, "Could not create account")
		}
		return
	}
	if err = q.UpdatePassword(r.Context(), dbgen.UpdatePasswordParams{ID: user.ID, PasswordHash: hash, UpdatedAt: now}); err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	user.MustChangePassword = false
	if _, err = q.CreateUserSettings(r.Context(), dbgen.CreateUserSettingsParams{UserID: user.ID, UpdatedAt: now}); err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	raw, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, 500, "Could not start session")
		return
	}
	if err = q.CreateSession(r.Context(), dbgen.CreateSessionParams{TokenHash: tokenHash, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(sessionTTL)}); err != nil {
		writeError(w, 500, "Could not start session")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "Could not create account")
		return
	}
	http.SetCookie(w, s.cookie(raw, now.Add(sessionTTL)))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, publicUser(user))
}
