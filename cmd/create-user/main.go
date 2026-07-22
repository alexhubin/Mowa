package main

import (
	"context"
	"flag"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/alexhubin/Mova/internal/auth"
	"github.com/alexhubin/Mova/internal/config"
	"github.com/alexhubin/Mova/internal/database"
	"github.com/alexhubin/Mova/internal/database/dbgen"
	"github.com/google/uuid"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

func main() {
	email := flag.String("email", "", "email нового пользователя")
	username := flag.String("username", "", "ник: a-z, 0-9 и _")
	name := flag.String("name", "", "отображаемое имя")
	flag.Parse()

	password := os.Getenv("MOVA_TEMP_PASSWORD")
	*email = strings.ToLower(strings.TrimSpace(*email))
	*username = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(*username, "@")))
	*name = strings.TrimSpace(*name)
	address, err := mail.ParseAddress(*email)
	if err != nil || address.Address != *email || len(*email) > 254 {
		fail("укажите корректный -email")
	}
	if !usernamePattern.MatchString(*username) {
		fail("-username должен содержать 3–32 символа: a-z, 0-9 и _")
	}
	if len([]rune(*name)) < 2 || len([]rune(*name)) > 40 {
		fail("-name должен содержать от 2 до 40 символов")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fail("MOVA_TEMP_PASSWORD должен содержать от 8 до 128 символов")
	}

	cfg, err := config.Load()
	if err != nil {
		fail("конфигурация: %v", err)
	}
	db, err := database.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fail("база данных: %v", err)
	}
	defer db.Close()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		fail("начать транзакцию: %v", err)
	}
	defer tx.Rollback()
	queries := dbgen.New(db).WithTx(tx)
	now := time.Now()
	user, err := queries.CreateUser(context.Background(), dbgen.CreateUserParams{
		ID: uuid.NewString(), Username: *username, Email: *email, DisplayName: *name, PasswordHash: hash, CreatedAt: now,
	})
	if err != nil {
		fail("создать пользователя (проверьте уникальность email и username): %v", err)
	}
	if _, err := queries.CreateUserSettings(context.Background(), dbgen.CreateUserSettingsParams{UserID: user.ID, UpdatedAt: now}); err != nil {
		fail("создать настройки: %v", err)
	}
	if err := tx.Commit(); err != nil {
		fail("сохранить пользователя: %v", err)
	}
	fmt.Printf("Пользователь %s создан. При первом входе потребуется новый пароль.\n", user.Email)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
