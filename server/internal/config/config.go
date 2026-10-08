// Package config читает настройки сервера из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	DatabaseURL string
	ListenAddr  string
	UploadDir   string
	JWTSecret   []byte
	TokenTTL    time.Duration
	// EffconURL — база «Оценки эффективности» для синхронизации пользователей (необязательно).
	EffconURL string
	// FCMCredentialsFile — ключ сервисного аккаунта Firebase для push (необязательно).
	FCMCredentialsFile string
	// APKDir — папка с helpdesk.apk и meta.json для страницы /app.
	APKDir string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		ListenAddr:         getenv("LISTEN_ADDR", ":8091"),
		UploadDir:          getenv("UPLOAD_DIR", "./data/uploads"),
		JWTSecret:          []byte(os.Getenv("JWT_SECRET")),
		EffconURL:          os.Getenv("EFFCON_DATABASE_URL"),
		FCMCredentialsFile: os.Getenv("FCM_CREDENTIALS_FILE"),
		APKDir:             getenv("APK_DIR", "./data/apk"),
	}
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return cfg, errors.New("JWT_SECRET must be at least 32 characters (try: openssl rand -hex 32)")
	}
	ttl, err := time.ParseDuration(getenv("TOKEN_TTL", "720h"))
	if err != nil {
		return cfg, fmt.Errorf("TOKEN_TTL: %w", err)
	}
	cfg.TokenTTL = ttl
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
