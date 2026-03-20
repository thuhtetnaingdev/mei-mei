package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv              string
	Port                string
	DatabasePath        string
	JWTSecret           string
	AdminUsername       string
	AdminPassword       string
	BaseSubscriptionURL string
	BasePublicURL       string
	AllowedOrigins      []string
	FrontendDistDir     string
	NodeSharedToken     string
	SyncTimeoutSeconds  int
}

func Load() Config {
	_ = godotenv.Load()

	cfg := Config{
		AppEnv:              getEnv("APP_ENV", "development"),
		Port:                getEnv("PORT", "8080"),
		DatabasePath:        getEnv("DATABASE_PATH", "./panel.sqlite3"),
		JWTSecret:           mustEnv("JWT_SECRET"),
		AdminUsername:       getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:       getEnv("ADMIN_PASSWORD", "admin"),
		BaseSubscriptionURL: getEnv("BASE_SUBSCRIPTION_URL", "http://localhost:8080/subscription"),
		AllowedOrigins:      getEnvAsSlice("ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://127.0.0.1:5173"}),
		FrontendDistDir:     getEnv("FRONTEND_DIST_DIR", ""),
		NodeSharedToken:     mustEnv("NODE_SHARED_TOKEN"),
		SyncTimeoutSeconds:  getEnvAsInt("SYNC_TIMEOUT_SECONDS", 10),
	}
	cfg.BasePublicURL = getEnv("BASE_PUBLIC_URL", strings.TrimSuffix(cfg.BaseSubscriptionURL, "/subscription"))

	return cfg
}

func getEnvAsSlice(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func mustEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return value
}

func getEnvAsInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return n
}
