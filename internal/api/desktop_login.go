package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/alexhubin/Mowa/internal/auth"
	"github.com/alexhubin/Mowa/internal/database/dbgen"
)

var proofPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

const desktopLoginTTL = 5 * time.Minute

func (s *Server) startDesktopLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		Challenge string `json:"challenge"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !proofPattern.MatchString(input.Challenge) {
		writeError(w, 400, "Invalid sign-in request")
		return
	}
	id, _, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	_, err = s.db.ExecContext(r.Context(), "DELETE FROM desktop_logins WHERE expires_at <= $1", s.now())
	if err == nil {
		_, err = s.db.ExecContext(r.Context(), "INSERT INTO desktop_logins (id,challenge,expires_at) VALUES ($1,$2,$3)", id, input.Challenge, s.now().Add(desktopLoginTTL))
	}
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (s *Server) approveDesktopLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		ID string `json:"id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := s.db.ExecContext(r.Context(), "UPDATE desktop_logins SET user_id=$1 WHERE id=$2 AND user_id IS NULL AND expires_at>$3", currentUser(r).ID, input.ID, s.now())
	if err != nil {
		writeError(w, 500, "Could not approve sign-in")
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		writeError(w, 410, "Sign-in request expired or has already been used")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) exchangeDesktopLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		ID       string `json:"id"`
		Verifier string `json:"verifier"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !proofPattern.MatchString(input.Verifier) {
		writeError(w, 400, "Invalid sign-in request")
		return
	}
	sum := sha256.Sum256([]byte(input.Verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Could not complete sign-in")
		return
	}
	defer tx.Rollback()
	var expected string
	var userID sql.NullString
	err = tx.QueryRowContext(r.Context(), "SELECT challenge,user_id FROM desktop_logins WHERE id=$1 AND expires_at>$2 FOR UPDATE", input.ID, s.now()).Scan(&expected, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 410, "Sign-in request expired. Try again.")
		return
	}
	if err != nil {
		writeError(w, 500, "Could not complete sign-in")
		return
	}
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(expected)) != 1 {
		writeError(w, 403, "Invalid sign-in proof")
		return
	}
	if !userID.Valid {
		writeJSON(w, 202, map[string]bool{"pending": true})
		return
	}
	q := s.queries.WithTx(tx)
	user, err := q.GetUserByID(r.Context(), userID.String)
	if err != nil || user.MustChangePassword {
		writeError(w, 403, "Change your temporary password on the website first")
		return
	}
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, 500, "Could not start session")
		return
	}
	now := s.now()
	if err = q.CreateSession(r.Context(), dbgen.CreateSessionParams{TokenHash: hash, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(sessionTTL)}); err != nil {
		writeError(w, 500, "Could not start session")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM desktop_logins WHERE id=$1", input.ID); err != nil {
		writeError(w, 500, "Could not complete sign-in")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "Could not complete sign-in")
		return
	}
	http.SetCookie(w, s.cookie(raw, now.Add(sessionTTL)))
	writeJSON(w, 200, publicUser(user))
}
