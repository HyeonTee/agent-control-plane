package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	Environment  string
	OriginSecret string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:     os.Getenv("HUB_HTTP_ADDR"),
		DatabaseURL:  os.Getenv("HUB_DATABASE_URL"),
		Environment:  os.Getenv("HUB_ENV"),
		OriginSecret: os.Getenv("HUB_ORIGIN_SECRET"),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("HUB_DATABASE_URL is required")
	}
	if cfg.Environment != "" && cfg.Environment != "local" && cfg.Environment != "production" {
		return Config{}, errors.New("HUB_ENV must be local or production")
	}
	if cfg.Environment == "production" && len(strings.TrimSpace(cfg.OriginSecret)) < 32 {
		return Config{}, errors.New("HUB_ORIGIN_SECRET must have at least 32 characters in production")
	}
	return cfg, nil
}
