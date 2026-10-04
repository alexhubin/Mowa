package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/alexhubin/Mowa/internal/auth"
	"github.com/alexhubin/Mowa/internal/database/dbgen"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const flowCookie = "mowa_auth_flow"
const flowTTL = 10 * time.Minute

func safeNext(raw string) string {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\\r\n") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return "/"
	}
	return raw
}
func normalizedEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	a, err := mail.ParseAddress(email)
	return email, err == nil && a.Address == email && len(email) <= 254
}
func (s *Server) authMethods(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]bool{"email": s.cfg.ResendKey != "", "google": s.cfg.GoogleClientID != "" && s.cfg.GoogleClientSecret != ""})
}
func (s *Server) setFlowCookie(w http.ResponseWriter, raw string) {
	c := s.cookie(raw, s.now().Add(flowTTL))
	c.Name = flowCookie
	http.SetCookie(w, c)
}
func flowToken(r *http.Request) string {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// Database-backed counters survive restarts and serialize requests from multiple replicas.
func (s *Server) authRate(r *http.Request, key string, limit int, window time.Duration) bool {
	var count int
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO auth_rate_limits(key_hash,count,expires_at) VALUES($1,1,$2)
 ON CONFLICT(key_hash) DO UPDATE SET count=CASE WHEN auth_rate_limits.expires_at <= $3 THEN 1 ELSE auth_rate_limits.count+1 END,
 expires_at=CASE WHEN auth_rate_limits.expires_at <= $3 THEN $2 ELSE auth_rate_limits.expires_at END RETURNING count`, auth.HashSessionToken(key), s.now().Add(window), s.now()).Scan(&count)
	return err == nil && count <= limit
}
func (s *Server) startEmailLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
		Next  string `json:"next"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	email, valid := normalizedEmail(input.Email)
	if !valid {
		writeError(w, 422, "Enter a valid email address")
		return
	}
	if s.cfg.ResendKey == "" && s.sendCode == nil {
		writeError(w, 503, "Email sign-in is unavailable")
		return
	}
	if !s.authRate(r, "email-global", 60, time.Hour) || !s.authRate(r, "email-minute:"+email, 1, time.Minute) || !s.authRate(r, "email-hour:"+email, 6, time.Hour) {
		w.Header().Set("Retry-After", "60")
		writeError(w, 429, "Too many requests. Please try again later.")
		return
	}
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	// The browser-only random token salts the six-digit code; neither is stored in plaintext.
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO auth_flows(token_hash,kind,email,code_hash,next_path,expires_at) VALUES($1,'otp',$2,$3,$4,$5)`, hash, email, auth.HashSessionToken(raw+":"+code), safeNext(input.Next), s.now().Add(flowTTL))
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	send := s.sendCode
	if send == nil {
		send = s.sendEmailCode
	}
	if err = send(r.Context(), email, code); err != nil {
		_, _ = s.db.ExecContext(r.Context(), "DELETE FROM auth_flows WHERE token_hash=$1", hash)
		writeError(w, 502, "Could not send the code. Please try again.")
		return
	}
	// Invalidate older codes only after successful delivery submission.
	_, _ = s.db.ExecContext(r.Context(), "DELETE FROM auth_flows WHERE (kind='otp' AND email=$1 AND token_hash<>$2) OR expires_at<=$3", email, hash, s.now())
	_, _ = s.db.ExecContext(r.Context(), "DELETE FROM auth_rate_limits WHERE expires_at <= $1", s.now())
	s.setFlowCookie(w, raw)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]int{"retry_after": 60})
}
func (s *Server) sendEmailCode(ctx context.Context, email, code string) error {
	payload, _ := json.Marshal(map[string]any{"from": s.cfg.ResendFrom, "to": []string{email}, "subject": "Your Mowa sign-in code", "text": "Your Mowa code is " + code + ".\n\nIt expires in 10 minutes. If you did not request this code, ignore this email."})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.ResendKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("email provider status %d", response.StatusCode)
	}
	return nil
}
func (s *Server) verifyEmailLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	raw := flowToken(r)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Could not verify code")
		return
	}
	defer tx.Rollback()
	var expected string
	var attempts int
	err = tx.QueryRowContext(r.Context(), "SELECT code_hash,attempts FROM auth_flows WHERE token_hash=$1 AND kind='otp' AND expires_at>$2 FOR UPDATE", auth.HashSessionToken(raw), s.now()).Scan(&expected, &attempts)
	if err != nil || attempts >= 5 {
		writeError(w, 401, "Code expired or invalid. Request a new code.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(auth.HashSessionToken(raw+":"+input.Code))) != 1 {
		_, err = tx.ExecContext(r.Context(), "UPDATE auth_flows SET attempts=attempts+1 WHERE token_hash=$1", auth.HashSessionToken(raw))
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			writeError(w, 500, "Could not verify code")
			return
		}
		writeError(w, 401, "Incorrect code")
		return
	}
	// Rotate the browser proof so replaying an OTP cannot complete an identity twice.
	nextRaw, nextHash, err := auth.NewSessionToken()
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE auth_flows SET token_hash=$1,kind='identity',code_hash='' WHERE token_hash=$2", nextHash, auth.HashSessionToken(raw))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 500, "Could not verify code")
		return
	}
	s.setFlowCookie(w, nextRaw)
	s.resolveIdentity(w, r, nextRaw, "")
}
func (s *Server) pendingIdentity(w http.ResponseWriter, r *http.Request) {
	s.resolveIdentity(w, r, flowToken(r), "")
}
func (s *Server) completeIdentity(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	username := normalizeUsername(input.Username)
	if !usernamePattern.MatchString(username) {
		writeError(w, 422, "Use 3–32 lowercase letters, numbers or underscores")
		return
	}
	s.resolveIdentity(w, r, flowToken(r), username)
}
func (s *Server) resolveIdentity(w http.ResponseWriter, r *http.Request, raw, username string) {
	w.Header().Set("Cache-Control", "no-store")
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "Could not sign in")
		return
	}
	defer tx.Rollback()
	var email, subject, next string
	err = tx.QueryRowContext(r.Context(), "SELECT email,subject,next_path FROM auth_flows WHERE token_hash=$1 AND kind='identity' AND expires_at>$2 FOR UPDATE", auth.HashSessionToken(raw), s.now()).Scan(&email, &subject, &next)
	if err != nil {
		writeError(w, 401, "Sign-in expired. Please try again.")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		writeError(w, 500, "Could not sign in")
		return
	}
	q := s.queries.WithTx(tx)
	var user dbgen.User
	if subject != "" {
		var id string
		err = tx.QueryRowContext(r.Context(), "SELECT user_id FROM google_identities WHERE subject=$1", subject).Scan(&id)
		if err == nil {
			user, err = q.GetUserByID(r.Context(), id)
		}
	} else {
		err = sql.ErrNoRows
	}
	if errors.Is(err, sql.ErrNoRows) {
		user, err = q.GetUserByEmail(r.Context(), email)
	}
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
			writeError(w, 500, "Could not create account")
			return
		}
		if count >= registrationAccountLimit {
			writeError(w, 403, "Registration is temporarily closed")
			return
		}
		if username == "" {
			writeJSON(w, 200, map[string]any{"needs_username": true, "email": email})
			return
		}
		// Passwordless accounts have no usable password hash.
		user, err = q.CreateUser(r.Context(), dbgen.CreateUserParams{ID: s.newID(), Email: email, Username: username, DisplayName: username, PasswordHash: "!passwordless", CreatedAt: s.now()})
		if err == nil {
			err = q.UpdatePassword(r.Context(), dbgen.UpdatePasswordParams{ID: user.ID, PasswordHash: "!passwordless", UpdatedAt: s.now()})
			user.MustChangePassword = false
		}
		if err == nil {
			_, err = q.CreateUserSettings(r.Context(), dbgen.CreateUserSettingsParams{UserID: user.ID, UpdatedAt: s.now()})
		}
	}
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, 409, "Username is already taken")
		} else {
			writeError(w, 500, "Could not sign in")
		}
		return
	}
	if subject != "" {
		var linked string
		err = tx.QueryRowContext(r.Context(), "INSERT INTO google_identities(subject,user_id) VALUES($1,$2) ON CONFLICT(subject) DO UPDATE SET subject=EXCLUDED.subject RETURNING user_id", subject, user.ID).Scan(&linked)
		if err != nil || linked != user.ID {
			writeError(w, 409, "This account is linked to another Google identity. Use email sign-in.")
			return
		}
	}
	sessionRaw, sessionHash, err := auth.NewSessionToken()
	if err == nil {
		err = q.CreateSession(r.Context(), dbgen.CreateSessionParams{TokenHash: sessionHash, UserID: user.ID, CreatedAt: s.now(), ExpiresAt: s.now().Add(sessionTTL)})
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM auth_flows WHERE token_hash=$1", auth.HashSessionToken(raw))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeError(w, 500, "Could not start session")
		return
	}
	http.SetCookie(w, s.cookie(sessionRaw, s.now().Add(sessionTTL)))
	c := s.cookie("", s.now().Add(-time.Hour))
	c.Name = flowCookie
	http.SetCookie(w, c)
	writeJSON(w, 200, map[string]any{"user": publicUser(user), "next": safeNext(next)})
}
func (s *Server) googleConfig() oauth2.Config {
	return oauth2.Config{ClientID: s.cfg.GoogleClientID, ClientSecret: s.cfg.GoogleClientSecret, RedirectURL: strings.TrimRight(s.cfg.AppOrigin, "/") + "/api/auth/google/callback", Endpoint: google.Endpoint, Scopes: []string{"openid", "email", "profile"}}
}
func (s *Server) startGoogleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.GoogleClientID == "" || s.cfg.GoogleClientSecret == "" {
		writeError(w, 503, "Google sign-in is unavailable")
		return
	}
	if !s.authRate(r, "google-global", 120, time.Hour) {
		writeError(w, 429, "Too many requests. Try again later.")
		return
	}
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	nonce, _, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), "DELETE FROM auth_flows WHERE expires_at <= $1", s.now())
	_, _ = s.db.ExecContext(r.Context(), "DELETE FROM auth_rate_limits WHERE expires_at <= $1", s.now())
	verifier := oauth2.GenerateVerifier()
	_, err = s.db.ExecContext(r.Context(), "INSERT INTO auth_flows(token_hash,kind,next_path,nonce,verifier,expires_at) VALUES($1,'google',$2,$3,$4,$5)", hash, safeNext(r.URL.Query().Get("next")), nonce, verifier, s.now().Add(flowTTL))
	if err != nil {
		writeError(w, 500, "Could not start sign-in")
		return
	}
	s.setFlowCookie(w, raw)
	cfg := s.googleConfig()
	http.Redirect(w, r, cfg.AuthCodeURL(raw, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), 302)
}
func (s *Server) verifyGoogle(ctx context.Context, code, verifier, nonce string) (string, string, error) {
	cfg := s.googleConfig()
	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", "", err
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return "", "", errors.New("missing ID token")
	}
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return "", "", err
	}
	id, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(ctx, raw)
	if err != nil {
		return "", "", err
	}
	var claims struct {
		Email        string `json:"email"`
		Verified     bool   `json:"email_verified"`
		Nonce        string `json:"nonce"`
		HostedDomain string `json:"hd"`
	}
	if err = id.Claims(&claims); err != nil {
		return "", "", err
	}
	email, valid := normalizedEmail(claims.Email)
	// Google is authoritative for Gmail and verified Workspace domains. Other addresses use email OTP.
	if !valid || !claims.Verified || claims.Nonce != nonce || id.Subject == "" || (!strings.HasSuffix(email, "@gmail.com") && claims.HostedDomain == "") {
		return "", "", errors.New("email requires verification")
	}
	return email, id.Subject, nil
}
func (s *Server) finishGoogleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fail := func() { http.Redirect(w, r, "/login?auth_error=google", 303) }
	raw := flowToken(r)
	if raw == "" || subtle.ConstantTimeCompare([]byte(raw), []byte(r.URL.Query().Get("state"))) != 1 {
		fail()
		return
	}
	var nonce, verifier, next string
	err := s.db.QueryRowContext(r.Context(), "DELETE FROM auth_flows WHERE token_hash=$1 AND kind='google' AND expires_at>$2 RETURNING nonce,verifier,next_path", auth.HashSessionToken(raw), s.now()).Scan(&nonce, &verifier, &next)
	if err != nil || r.URL.Query().Get("code") == "" {
		fail()
		return
	}
	verify := s.googleClaims
	if verify == nil {
		verify = s.verifyGoogle
	}
	email, subject, err := verify(r.Context(), r.URL.Query().Get("code"), verifier, nonce)
	if err != nil {
		fail()
		return
	}
	newRaw, hash, err := auth.NewSessionToken()
	if err != nil {
		fail()
		return
	}
	_, err = s.db.ExecContext(r.Context(), "INSERT INTO auth_flows(token_hash,kind,email,subject,next_path,expires_at) VALUES($1,'identity',$2,$3,$4,$5)", hash, email, subject, safeNext(next), s.now().Add(flowTTL))
	if err != nil {
		fail()
		return
	}
	s.setFlowCookie(w, newRaw)
	http.Redirect(w, r, "/login?verified=1", 303)
}
