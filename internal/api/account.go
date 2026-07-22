package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alexhubin/Mowa/internal/auth"
	"github.com/alexhubin/Mowa/internal/database/dbgen"
)

type profileRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type firstPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

type settingsRequest struct {
	VideoQuality string `json:"video_quality"`
}

type settingsResponse struct {
	VideoQuality string `json:"video_quality"`
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	var input profileRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Username = normalizeUsername(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !usernamePattern.MatchString(input.Username) {
		writeError(w, http.StatusUnprocessableEntity, "Username: 3–32 символа, только латинские буквы, цифры и _")
		return
	}
	if len([]rune(input.DisplayName)) < 2 || len([]rune(input.DisplayName)) > 40 {
		writeError(w, http.StatusUnprocessableEntity, "Имя должно содержать от 2 до 40 символов")
		return
	}

	user, err := s.queries.UpdateProfile(r.Context(), dbgen.UpdateProfileParams{
		ID: currentUser(r).ID, Username: input.Username, DisplayName: input.DisplayName, UpdatedAt: s.now(),
	})
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "Этот username уже занят")
		return
	}
	if err != nil {
		slog.Error("update profile", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить профиль")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user))
}

func (s *Server) updatePassword(w http.ResponseWriter, r *http.Request) {
	var input passwordRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	user := currentUser(r)
	if !auth.VerifyPassword(user.PasswordHash, input.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "Текущий пароль указан неверно")
		return
	}
	if !s.changePassword(w, r, user, input.NewPassword) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) completeFirstPassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !user.MustChangePassword {
		writeError(w, http.StatusConflict, "Временный пароль уже был заменён")
		return
	}
	var input firstPasswordRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !s.changePassword(w, r, user, input.NewPassword) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, user dbgen.User, newPassword string) bool {
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Новый пароль должен содержать от 8 до 128 символов")
		return false
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Не удалось изменить пароль")
		return false
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	if err := queries.UpdatePassword(r.Context(), dbgen.UpdatePasswordParams{ID: user.ID, PasswordHash: hash, UpdatedAt: s.now()}); err != nil {
		slog.Error("update password", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось изменить пароль")
		return false
	}
	if err := queries.DeleteUserSessions(r.Context(), user.ID); err != nil {
		slog.Error("delete sessions after password change", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось изменить пароль")
		return false
	}
	if err := tx.Commit(); err != nil {
		slog.Error("commit password", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось изменить пароль")
		return false
	}
	if err := s.startSession(w, r, user.ID); err != nil {
		slog.Error("restart session", "error", err)
		writeError(w, http.StatusInternalServerError, "Пароль изменён, но не удалось обновить сессию")
		return false
	}
	return true
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.queries.GetUserSettings(r.Context(), currentUser(r).ID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, settingsResponse{VideoQuality: "high"})
		return
	}
	if err != nil {
		slog.Error("get settings", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось загрузить настройки")
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{VideoQuality: settings.VideoQuality})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var input settingsRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.VideoQuality != "low" && input.VideoQuality != "high" {
		writeError(w, http.StatusUnprocessableEntity, "Неизвестное качество видео")
		return
	}
	settings, err := s.queries.UpdateUserSettings(r.Context(), dbgen.UpdateUserSettingsParams{
		UserID: currentUser(r).ID, VideoQuality: input.VideoQuality, UpdatedAt: s.now(),
	})
	if err != nil {
		slog.Error("update settings", "error", err)
		writeError(w, http.StatusInternalServerError, "Не удалось сохранить настройки")
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{VideoQuality: settings.VideoQuality})
}
