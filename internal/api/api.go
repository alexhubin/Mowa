package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alexhubin/Mowa/internal/auth"
	"github.com/alexhubin/Mowa/internal/config"
	"github.com/alexhubin/Mowa/internal/database/dbgen"
	"github.com/alexhubin/Mowa/internal/media"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	sessionCookie = "mova_session"
	sessionTTL    = 30 * 24 * time.Hour
	presenceTTL   = 30 * time.Second
	maxBodyBytes  = 64 << 10
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

type Server struct {
	db         *sql.DB
	queries    *dbgen.Queries
	cfg        config.Config
	issuer     media.TokenIssuer
	now        func() time.Time
	newID      func() string
	newInvite  func() (string, error)
	callEvents *callEventBroker
	webAuthn   *webauthn.WebAuthn
}

type contextKey string

const userContextKey contextKey = "user"

func New(db *sql.DB, cfg config.Config) (*Server, error) {
	webAuthn, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.WebAuthnRPID,
		RPDisplayName: cfg.WebAuthnRPName,
		RPOrigins:     []string{cfg.AppOrigin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
		AttestationPreference: protocol.PreferNoAttestation,
	})
	if err != nil {
		return nil, err
	}
	return &Server{
		db:         db,
		queries:    dbgen.New(db),
		cfg:        cfg,
		callEvents: newCallEventBroker(),
		webAuthn:   webAuthn,
		issuer:     media.TokenIssuer{APIKey: cfg.LiveKitAPIKey, APISecret: cfg.LiveKitAPISecret, TTL: cfg.LiveKitTokenTTL},
		now:        time.Now,
		newID:      uuid.NewString,
		newInvite: func() (string, error) {
			value := make([]byte, 8)
			if _, err := rand.Read(value); err != nil {
				return "", err
			}
			return base64.RawURLEncoding.EncodeToString(value), nil
		},
	}, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestTimeout)
	r.Use(s.securityHeaders)
	r.Use(s.verifyOrigin)

	r.Get("/api/health", s.health)
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", s.login)
		r.Post("/passkey/login/begin", s.beginPasskeyLogin)
		r.Post("/passkey/login/finish", s.finishPasskeyLogin)
		r.With(s.requireUser).Post("/logout", s.logout)
		r.With(s.requireUser).Get("/me", s.me)
		r.With(s.requireUser).Put("/first-password", s.completeFirstPassword)
	})
	r.Group(func(r chi.Router) {
		r.Use(s.requireUser)
		r.Use(s.requirePasswordChanged)
		r.Patch("/api/account/profile", s.updateProfile)
		r.Put("/api/account/password", s.updatePassword)
		r.Get("/api/account/passkeys", s.listPasskeys)
		r.Post("/api/account/passkeys/register/begin", s.beginPasskeyRegistration)
		r.Post("/api/account/passkeys/register/finish", s.finishPasskeyRegistration)
		r.Delete("/api/account/passkeys/{passkeyID}", s.deletePasskey)
		r.Get("/api/account/settings", s.getSettings)
		r.Put("/api/account/settings", s.updateSettings)
		r.Get("/api/users/search", s.searchUsers)
		r.Get("/api/friends", s.listFriends)
		r.Post("/api/friend-requests", s.createFriendRequest)
		r.Post("/api/friend-requests/{requestID}/accept", s.acceptFriendRequest)
		r.Delete("/api/friend-requests/{requestID}", s.declineFriendRequest)
		r.Delete("/api/friends/{userID}", s.deleteFriend)
		r.Get("/api/calls", s.listCalls)
		r.Get("/api/calls/events", s.streamCallEvents)
		r.Post("/api/calls", s.createDirectCall)
		r.Post("/api/calls/{callID}/accept", s.acceptDirectCall)
		r.Post("/api/calls/{callID}/decline", s.declineDirectCall)
		r.Post("/api/calls/{callID}/end", s.endDirectCall)
		r.Post("/api/rooms", s.createRoom)
		r.Get("/api/rooms/{inviteCode}", s.getRoom)
		r.Post("/api/rooms/{inviteCode}/token", s.roomToken)
		r.Post("/api/presence", s.presence)
	})

	return r
}

func requestTimeout(next http.Handler) http.Handler {
	standard := middleware.Timeout(15 * time.Second)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/calls/events" {
			next.ServeHTTP(w, r)
			return
		}
		standard.ServeHTTP(w, r)
	})
}

func (s *Server) presence(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) verifyOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" && strings.TrimRight(origin, "/") != strings.TrimRight(s.cfg.AppOrigin, "/") {
				writeError(w, http.StatusForbidden, "Недопустимый источник запроса")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "Требуется вход")
			return
		}
		now := s.now()
		tokenHash := auth.HashSessionToken(cookie.Value)
		user, err := s.queries.GetSessionUser(r.Context(), dbgen.GetSessionUserParams{
			TokenHash: tokenHash,
			ExpiresAt: now,
		})
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "Сессия истекла")
			return
		}
		if err != nil {
			slog.Error("load session", "error", err)
			writeError(w, http.StatusInternalServerError, "Не удалось проверить сессию")
			return
		}
		if err := s.queries.TouchSession(r.Context(), dbgen.TouchSessionParams{TokenHash: tokenHash, LastSeenAt: now}); err != nil {
			slog.Warn("touch session presence", "error", err)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *Server) requirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).MustChangePassword {
			writeError(w, http.StatusForbidden, "Сначала измените временный пароль")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	Email              string `json:"email"`
	DisplayName        string `json:"display_name"`
	MustChangePassword bool   `json:"must_change_password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input authRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	user, err := s.queries.GetUserByEmail(r.Context(), strings.TrimSpace(input.Email))
	if err != nil || !auth.VerifyPassword(user.PasswordHash, input.Password) {
		writeError(w, http.StatusUnauthorized, "Неверный email или пароль")
		return
	}
	if err := s.startSession(w, r, user.ID); err != nil {
		slog.Error("create session", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать сессию")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if err := s.queries.DeleteSession(r.Context(), auth.HashSessionToken(cookie.Value)); err != nil {
			slog.Warn("delete session", "error", err)
		}
	}
	http.SetCookie(w, s.cookie("", s.now().Add(-time.Hour)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, publicUser(currentUser(r)))
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID string) error {
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	now := s.now()
	if err := s.queries.CreateSession(r.Context(), dbgen.CreateSessionParams{
		TokenHash: hash, UserID: userID, ExpiresAt: now.Add(sessionTTL), CreatedAt: now,
	}); err != nil {
		return err
	}
	http.SetCookie(w, s.cookie(raw, now.Add(sessionTTL)))
	return nil
}

func (s *Server) cookie(value string, expires time.Time) *http.Cookie {
	return &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(expires.Sub(s.now()).Seconds())}
}

type createRoomRequest struct {
	Name string `json:"name"`
}

type roomResponse struct {
	ID         string    `json:"id"`
	InviteCode string    `json:"invite_code"`
	Name       string    `json:"name"`
	OwnerID    string    `json:"owner_id"`
	Kind       string    `json:"kind"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	var input createRoomRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 2 || len([]rune(input.Name)) > 80 {
		writeError(w, http.StatusUnprocessableEntity, "Название должно содержать от 2 до 80 символов")
		return
	}
	invite, err := s.newInvite()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать приглашение")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось создать комнату")
		return
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	now := s.now()
	ownerID := currentUser(r).ID
	room, err := queries.CreateRoom(r.Context(), dbgen.CreateRoomParams{
		ID: s.newID(), InviteCode: invite, Name: input.Name, OwnerID: ownerID, Kind: "group", CreatedAt: now,
	})
	if err != nil {
		slog.Error("create room", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось создать комнату")
		return
	}
	if err := queries.AddRoomMember(r.Context(), dbgen.AddRoomMemberParams{RoomID: room.ID, UserID: ownerID, CreatedAt: now}); err != nil {
		slog.Error("add room owner", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось создать комнату")
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error("commit room", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось создать комнату")
		return
	}
	writeJSON(w, http.StatusCreated, publicRoom(room))
}

func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) {
	room, ok := s.findRoom(w, r)
	if ok {
		writeJSON(w, http.StatusOK, publicRoom(room))
	}
}

func (s *Server) roomToken(w http.ResponseWriter, r *http.Request) {
	room, ok := s.findRoom(w, r)
	if !ok {
		return
	}
	user := currentUser(r)
	if room.Kind == "direct" {
		member, err := s.queries.IsRoomMember(r.Context(), dbgen.IsRoomMemberParams{RoomID: room.ID, UserID: user.ID})
		if err != nil || !member {
			writeError(w, http.StatusForbidden, "Этот звонок доступен только его участникам")
			return
		}
	}
	token, err := s.issuer.Issue(room.ID, user.ID, user.DisplayName)
	if err != nil {
		slog.Error("issue livekit token", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось подключиться к звонку")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token, "server_url": s.cfg.LiveKitURL, "expires_in": int(s.cfg.LiveKitTokenTTL.Seconds()),
	})
}

func (s *Server) findRoom(w http.ResponseWriter, r *http.Request) (dbgen.Room, bool) {
	code := chi.URLParam(r, "inviteCode")
	if len(code) < 8 || len(code) > 32 {
		writeError(w, http.StatusNotFound, "Комната не найдена")
		return dbgen.Room{}, false
	}
	room, err := s.queries.GetRoomByInviteCode(r.Context(), code)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Комната не найдена")
		return dbgen.Room{}, false
	}
	if err != nil {
		slog.Error("find room", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить комнату")
		return dbgen.Room{}, false
	}
	if room.Kind == "direct" {
		member, err := s.queries.IsRoomMember(r.Context(), dbgen.IsRoomMemberParams{RoomID: room.ID, UserID: currentUser(r).ID})
		if err != nil {
			slog.Error("check room membership", "error", err)
			writeError(w, http.StatusInternalServerError, "Не удалось проверить доступ к звонку")
			return dbgen.Room{}, false
		}
		if !member {
			writeError(w, http.StatusNotFound, "Комната не найдена")
			return dbgen.Room{}, false
		}
	}
	return room, true
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "Некорректный JSON")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "JSON должен содержать ровно один объект")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Warn("encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func currentUser(r *http.Request) dbgen.User {
	return r.Context().Value(userContextKey).(dbgen.User)
}

func publicUser(user dbgen.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, Email: user.Email, DisplayName: user.DisplayName, MustChangePassword: user.MustChangePassword}
}

func publicRoom(room dbgen.Room) roomResponse {
	return roomResponse{ID: room.ID, InviteCode: room.InviteCode, Name: room.Name, OwnerID: room.OwnerID, Kind: room.Kind, CreatedAt: room.CreatedAt}
}

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "@")))
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
