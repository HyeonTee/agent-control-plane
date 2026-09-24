package config

import (
	"errors"
	"os"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    os.Getenv("HUB_HTTP_ADDR"),
		DatabaseURL: os.Getenv("HUB_DATABASE_URL"),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("HUB_DATABASE_URL is required")
	}
	return cfg, nil
}
