package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	Environment        string
	OriginSecret       string
	PublicURL          string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubOwnerID      int64
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:           os.Getenv("HUB_HTTP_ADDR"),
		DatabaseURL:        os.Getenv("HUB_DATABASE_URL"),
		Environment:        os.Getenv("HUB_ENV"),
		OriginSecret:       os.Getenv("HUB_ORIGIN_SECRET"),
		PublicURL:          strings.TrimRight(os.Getenv("HUB_PUBLIC_URL"), "/"),
		GitHubClientID:     os.Getenv("HUB_GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("HUB_GITHUB_CLIENT_SECRET"),
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
	ownerID := os.Getenv("HUB_GITHUB_OWNER_ID")
	if cfg.GitHubClientID != "" || cfg.GitHubClientSecret != "" || ownerID != "" {
		if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" || cfg.PublicURL == "" {
			return Config{}, errors.New("device authorization requires HUB_PUBLIC_URL and GitHub OAuth credentials")
		}
		id, err := strconv.ParseInt(ownerID, 10, 64)
		if err != nil || id <= 0 {
			return Config{}, errors.New("HUB_GITHUB_OWNER_ID must be a positive GitHub user ID")
		}
		cfg.GitHubOwnerID = id
		if cfg.Environment == "production" && !strings.HasPrefix(cfg.PublicURL, "https://") {
			return Config{}, errors.New("HUB_PUBLIC_URL must use HTTPS in production")
		}
	}
	return cfg, nil
}
