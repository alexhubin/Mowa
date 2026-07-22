package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	AppOrigin        string
	CookieSecure     bool
	LiveKitURL       string
	LiveKitAPIKey    string
	LiveKitAPISecret string
	LiveKitTokenTTL  time.Duration
	WebAuthnRPID     string
	WebAuthnRPName   string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		DatabaseURL:      env("DATABASE_URL", "postgres://mova:mova-development-password@localhost:5432/mova?sslmode=disable"),
		AppOrigin:        env("APP_ORIGIN", "http://localhost"),
		LiveKitURL:       env("LIVEKIT_URL", "ws://localhost:7880"),
		LiveKitAPIKey:    env("LIVEKIT_API_KEY", "devkey"),
		LiveKitAPISecret: env("LIVEKIT_API_SECRET", "secretsecretsecretsecretsecretsecret"),
		LiveKitTokenTTL:  10 * time.Minute,
		WebAuthnRPName:   env("WEBAUTHN_RP_NAME", "Mowa"),
	}
	appURL, err := url.Parse(cfg.AppOrigin)
	if err != nil || appURL.Hostname() == "" {
		return Config{}, fmt.Errorf("APP_ORIGIN must be an absolute URL")
	}
	cfg.WebAuthnRPID = env("WEBAUTHN_RP_ID", appURL.Hostname())

	secure, err := strconv.ParseBool(env("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("parse COOKIE_SECURE: %w", err)
	}
	cfg.CookieSecure = secure

	if raw := os.Getenv("LIVEKIT_TOKEN_TTL"); raw != "" {
		cfg.LiveKitTokenTTL, err = time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse LIVEKIT_TOKEN_TTL: %w", err)
		}
	}
	if cfg.LiveKitTokenTTL <= 0 || cfg.LiveKitTokenTTL > time.Hour {
		return Config{}, fmt.Errorf("LIVEKIT_TOKEN_TTL must be between 1ns and 1h")
	}
	if len(cfg.LiveKitAPISecret) < 32 {
		return Config{}, fmt.Errorf("LIVEKIT_API_SECRET must contain at least 32 characters")
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
