package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexhubin/Mova/internal/auth"
	"github.com/alexhubin/Mova/internal/database/dbgen"
	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	passkeyCeremonyCookie = "mova_webauthn"
	passkeyCeremonyTTL    = 5 * time.Minute
	maxPasskeysPerUser    = 10
	maxWebAuthnBodyBytes  = 256 << 10
)

type passkeyUser struct {
	user        dbgen.User
	handle      []byte
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.handle }
func (u *passkeyUser) WebAuthnName() string                       { return u.user.Email }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.user.DisplayName }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

type passkeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type beginPasskeyRegistrationRequest struct {
	Name string `json:"name"`
}

func (s *Server) listPasskeys(w http.ResponseWriter, r *http.Request) {
	items, err := s.queries.ListPasskeys(r.Context(), currentUser(r).ID)
	if err != nil {
		slog.Error("list passkeys", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить passkey")
		return
	}
	response := make([]passkeyResponse, 0, len(items))
	for _, item := range items {
		response = append(response, publicPasskey(item))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) beginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	var input beginPasskeyRegistrationRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = "Passkey"
	}
	if len([]rune(input.Name)) > 50 {
		writeError(w, http.StatusUnprocessableEntity, "Название passkey не должно превышать 50 символов")
		return
	}

	userRecord := currentUser(r)
	count, err := s.queries.CountPasskeys(r.Context(), userRecord.ID)
	if err != nil {
		slog.Error("count passkeys", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось добавить passkey")
		return
	}
	if count >= maxPasskeysPerUser {
		writeError(w, http.StatusConflict, "Можно добавить не больше 10 passkey")
		return
	}

	user, err := s.ensurePasskeyUser(r, userRecord)
	if err != nil {
		slog.Error("load passkey user", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось добавить passkey")
		return
	}
	creation, session, err := s.webAuthn.BeginRegistration(
		user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()),
	)
	if err != nil {
		slog.Error("begin passkey registration", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать создание passkey")
		return
	}
	if err := s.savePasskeyCeremony(w, r, userRecord.ID, "registration", input.Name, session); err != nil {
		slog.Error("save passkey registration", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать создание passkey")
		return
	}
	writeJSON(w, http.StatusOK, creation)
}

func (s *Server) finishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	ceremony, session, ok := s.consumePasskeyCeremony(w, r, "registration")
	if !ok {
		return
	}
	userRecord := currentUser(r)
	if !ceremony.UserID.Valid || ceremony.UserID.String != userRecord.ID {
		writeError(w, http.StatusUnauthorized, "Сессия создания passkey недействительна")
		return
	}
	user, err := s.loadPasskeyUser(r, userRecord)
	if err != nil {
		slog.Error("load passkey user for registration", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить passkey")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWebAuthnBodyBytes)
	credential, err := s.webAuthn.FinishRegistration(user, session, r)
	if err != nil {
		slog.Warn("finish passkey registration", "error", err)
		writeError(w, http.StatusBadRequest, "Passkey не подтверждён устройством")
		return
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить passkey")
		return
	}
	item, err := s.queries.CreatePasskey(r.Context(), dbgen.CreatePasskeyParams{
		ID:           s.newID(),
		UserID:       userRecord.ID,
		CredentialID: credential.ID,
		Name:         ceremony.Name,
		Credential:   encoded,
		CreatedAt:    s.now(),
	})
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "Этот passkey уже добавлен")
		return
	}
	if err != nil {
		slog.Error("create passkey", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить passkey")
		return
	}
	writeJSON(w, http.StatusCreated, publicPasskey(item))
}

func (s *Server) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	assertion, session, err := s.webAuthn.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		slog.Error("begin passkey login", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать вход по passkey")
		return
	}
	if err := s.savePasskeyCeremony(w, r, "", "login", "", session); err != nil {
		slog.Error("save passkey login", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать вход по passkey")
		return
	}
	writeJSON(w, http.StatusOK, assertion)
}

func (s *Server) finishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	_, session, ok := s.consumePasskeyCeremony(w, r, "login")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWebAuthnBodyBytes)
	validatedUser, credential, err := s.webAuthn.FinishPasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		userRecord, err := s.queries.GetUserByPasskey(r.Context(), dbgen.GetUserByPasskeyParams{
			CredentialID: rawID,
			Handle:       userHandle,
		})
		if err != nil {
			return nil, err
		}
		return s.loadPasskeyUser(r, userRecord)
	}, session, r)
	if err != nil {
		slog.Warn("finish passkey login", "error", err)
		writeError(w, http.StatusUnauthorized, "Passkey не удалось проверить")
		return
	}
	user, ok := validatedUser.(*passkeyUser)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Не удалось завершить вход")
		return
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось завершить вход")
		return
	}
	updated, err := s.queries.UpdatePasskeyCredential(r.Context(), dbgen.UpdatePasskeyCredentialParams{
		CredentialID: credential.ID,
		Credential:   encoded,
		LastUsedAt:   sql.NullTime{Time: s.now(), Valid: true},
	})
	if err != nil || updated != 1 {
		slog.Error("update passkey after login", "error", err, "rows", updated)
		writeError(w, http.StatusInternalServerError, "Не удалось завершить вход")
		return
	}
	if err := s.startSession(w, r, user.user.ID); err != nil {
		slog.Error("create passkey session", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать сессию")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user.user))
}

func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.queries.DeletePasskey(r.Context(), dbgen.DeletePasskeyParams{
		ID:     chi.URLParam(r, "passkeyID"),
		UserID: currentUser(r).ID,
	})
	if err != nil {
		slog.Error("delete passkey", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось удалить passkey")
		return
	}
	if deleted == 0 {
		writeError(w, http.StatusNotFound, "Passkey не найден")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ensurePasskeyUser(r *http.Request, user dbgen.User) (*passkeyUser, error) {
	handle := make([]byte, 64)
	if _, err := rand.Read(handle); err != nil {
		return nil, fmt.Errorf("generate user handle: %w", err)
	}
	record, err := s.queries.EnsurePasskeyUser(r.Context(), dbgen.EnsurePasskeyUserParams{
		UserID: user.ID, Handle: handle, CreatedAt: s.now(),
	})
	if err != nil {
		return nil, err
	}
	return s.loadPasskeyUserWithHandle(r, user, record.Handle)
}

func (s *Server) loadPasskeyUser(r *http.Request, user dbgen.User) (*passkeyUser, error) {
	record, err := s.queries.GetPasskeyUser(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	return s.loadPasskeyUserWithHandle(r, user, record.Handle)
}

func (s *Server) loadPasskeyUserWithHandle(r *http.Request, user dbgen.User, handle []byte) (*passkeyUser, error) {
	records, err := s.queries.ListPasskeys(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		var credential webauthn.Credential
		if err := json.Unmarshal(record.Credential, &credential); err != nil {
			return nil, fmt.Errorf("decode credential %s: %w", record.ID, err)
		}
		credentials = append(credentials, credential)
	}
	return &passkeyUser{user: user, handle: handle, credentials: credentials}, nil
}

func (s *Server) savePasskeyCeremony(w http.ResponseWriter, r *http.Request, userID, kind, name string, session *webauthn.SessionData) error {
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		return err
	}
	now := s.now()
	if err := s.queries.DeleteExpiredPasskeyCeremonies(r.Context(), now); err != nil {
		slog.Warn("delete expired passkey ceremonies", "error", err)
	}
	if err := s.queries.CreatePasskeyCeremony(r.Context(), dbgen.CreatePasskeyCeremonyParams{
		TokenHash:   hash,
		UserID:      sql.NullString{String: userID, Valid: userID != ""},
		Kind:        kind,
		Name:        name,
		SessionData: encoded,
		ExpiresAt:   now.Add(passkeyCeremonyTTL),
		CreatedAt:   now,
	}); err != nil {
		return err
	}
	http.SetCookie(w, s.passkeyCookie(raw, now.Add(passkeyCeremonyTTL)))
	return nil
}

func (s *Server) consumePasskeyCeremony(w http.ResponseWriter, r *http.Request, kind string) (dbgen.PasskeyCeremony, webauthn.SessionData, bool) {
	var session webauthn.SessionData
	cookie, err := r.Cookie(passkeyCeremonyCookie)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "Сессия passkey истекла, попробуйте ещё раз")
		return dbgen.PasskeyCeremony{}, session, false
	}
	http.SetCookie(w, s.passkeyCookie("", s.now().Add(-time.Hour)))
	ceremony, err := s.queries.ConsumePasskeyCeremony(r.Context(), dbgen.ConsumePasskeyCeremonyParams{
		TokenHash: auth.HashSessionToken(cookie.Value), Kind: kind, ExpiresAt: s.now(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "Сессия passkey истекла, попробуйте ещё раз")
		return dbgen.PasskeyCeremony{}, session, false
	}
	if err != nil {
		slog.Error("consume passkey ceremony", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось проверить сессию passkey")
		return dbgen.PasskeyCeremony{}, session, false
	}
	if err := json.Unmarshal(ceremony.SessionData, &session); err != nil {
		slog.Error("decode passkey ceremony", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось проверить сессию passkey")
		return dbgen.PasskeyCeremony{}, session, false
	}
	return ceremony, session, true
}

func (s *Server) passkeyCookie(value string, expires time.Time) *http.Cookie {
	maxAge := int(expires.Sub(s.now()).Seconds())
	if expires.Before(s.now()) {
		maxAge = -1
	}
	return &http.Cookie{
		Name: passkeyCeremonyCookie, Value: value, Path: "/api/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: maxAge,
	}
}

func publicPasskey(item dbgen.Passkey) passkeyResponse {
	var lastUsed *time.Time
	if item.LastUsedAt.Valid {
		value := item.LastUsedAt.Time
		lastUsed = &value
	}
	return passkeyResponse{ID: item.ID, Name: item.Name, CreatedAt: item.CreatedAt, LastUsedAt: lastUsed}
}
