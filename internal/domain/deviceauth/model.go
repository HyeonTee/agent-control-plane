package deviceauth

import (
	"errors"
	"time"
)

var (
	ErrPending       = errors.New("authorization pending")
	ErrSlowDown      = errors.New("slow down")
	ErrDenied        = errors.New("access denied")
	ErrExpired       = errors.New("device code expired")
	ErrRefreshReplay = errors.New("refresh token replay")
)

type Authorization struct {
	DeviceCode string    `json:"device_code,omitempty"`
	UserCode   string    `json:"user_code"`
	Label      string    `json:"client_label"`
	Scopes     []string  `json:"scopes"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type Client struct {
	ID         string     `json:"id"`
	Label      string     `json:"client_label"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}
