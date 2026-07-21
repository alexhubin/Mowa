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
	"net/mail"
	"strings"
	"time"

	"github.com/alexhubin/Mova/internal/auth"
	"github.com/alexhubin/Mova/internal/config"
	"github.com/alexhubin/Mova/internal/database/dbgen"
	"github.com/alexhubin/Mova/internal/media"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const (
	sessionCookie = "mova_session"
	sessionTTL    = 7 * 24 * time.Hour
	maxBodyBytes  = 64 << 10
)

type Server struct {
	queries   *dbgen.Queries
	cfg       config.Config
	issuer    media.TokenIssuer
	now       func() time.Time
	newID     func() string
	newInvite func() (string, error)
}

type contextKey string

const userContextKey contextKey = "user"

func New(db *sql.DB, cfg config.Config) *Server {
	return &Server{
		queries: dbgen.New(db),
		cfg:     cfg,
		issuer:  media.TokenIssuer{APIKey: cfg.LiveKitAPIKey, APISecret: cfg.LiveKitAPISecret, TTL: cfg.LiveKitTokenTTL},
		now:     time.Now,
		newID:   uuid.NewString,
		newInvite: func() (string, error) {
			value := make([]byte, 8)
			if _, err := rand.Read(value); err != nil {
				return "", err
			}
			return base64.RawURLEncoding.EncodeToString(value), nil
		},
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))
	r.Use(s.securityHeaders)
	r.Use(s.verifyOrigin)

	r.Get("/api/health", s.health)
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/register", s.register)
		r.Post("/login", s.login)
		r.With(s.requireUser).Post("/logout", s.logout)
		r.With(s.requireUser).Get("/me", s.me)
	})
	r.Group(func(r chi.Router) {
		r.Use(s.requireUser)
		r.Post("/api/rooms", s.createRoom)
		r.Get("/api/rooms/{inviteCode}", s.getRoom)
		r.Post("/api/rooms/{inviteCode}/token", s.roomToken)
	})

	return r
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
		user, err := s.queries.GetSessionUser(r.Context(), dbgen.GetSessionUserParams{
			TokenHash: auth.HashSessionToken(cookie.Value),
			ExpiresAt: s.now().Unix(),
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
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

type authRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type userResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var input authRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email || len(input.Email) > 254 {
		writeError(w, http.StatusUnprocessableEntity, "Введите корректный email")
		return
	}
	if len([]rune(input.DisplayName)) < 2 || len([]rune(input.DisplayName)) > 40 {
		writeError(w, http.StatusUnprocessableEntity, "Имя должно содержать от 2 до 40 символов")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Пароль должен содержать от 8 до 128 символов")
		return
	}
	user, err := s.queries.CreateUser(r.Context(), dbgen.CreateUserParams{
		ID: s.newID(), Email: input.Email, DisplayName: input.DisplayName, PasswordHash: hash, CreatedAt: s.now().Unix(),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "Аккаунт с таким email уже существует")
			return
		}
		slog.Error("create user", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось создать аккаунт")
		return
	}
	if err := s.startSession(w, r, user.ID); err != nil {
		slog.Error("create session", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось начать сессию")
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(user))
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
		TokenHash: hash, UserID: userID, ExpiresAt: now.Add(sessionTTL).Unix(), CreatedAt: now.Unix(),
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
	ID         string `json:"id"`
	InviteCode string `json:"invite_code"`
	Name       string `json:"name"`
	OwnerID    string `json:"owner_id"`
	CreatedAt  int64  `json:"created_at"`
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
	room, err := s.queries.CreateRoom(r.Context(), dbgen.CreateRoomParams{
		ID: s.newID(), InviteCode: invite, Name: input.Name, OwnerID: currentUser(r).ID, CreatedAt: s.now().Unix(),
	})
	if err != nil {
		slog.Error("create room", "error", err)
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
	return room, true
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
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
	return userResponse{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName}
}

func publicRoom(room dbgen.Room) roomResponse {
	return roomResponse{ID: room.ID, InviteCode: room.InviteCode, Name: room.Name, OwnerID: room.OwnerID, CreatedAt: room.CreatedAt}
}
