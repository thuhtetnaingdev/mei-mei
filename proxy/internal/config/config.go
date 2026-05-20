package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port     string
	ProxyURL string
	MapToken string
}

func Load() Config {
	_ = godotenv.Load()

	cfg := Config{
		Port:     getEnv("PORT", "9091"),
		ProxyURL: getEnv("PROXY_URL", ""),
		MapToken: getEnv("MAP_TOKEN", ""),
	}

	if cfg.ProxyURL == "" {
		log.Fatal("missing required environment variable PROXY_URL")
	}
	if cfg.MapToken == "" {
		log.Fatal("missing required environment variable MAP_TOKEN")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
