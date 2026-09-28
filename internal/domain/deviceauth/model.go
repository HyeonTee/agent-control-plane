package deviceauth

import (
	"errors"
	"strings"
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

// NormalizeUserCode accepts a user code as a person might type it and returns
// the canonical XXXX-XXXX form, or "" when it cannot be a valid code. Codes
// use the RFC 4648 base32 alphabet, so separators and case are ignored and the
// digits 0, 1, and 8 are read as the letters they are commonly confused with.
func NormalizeUserCode(input string) string {
	var code strings.Builder
	for _, r := range strings.ToUpper(input) {
		switch {
		case r == '0':
			r = 'O'
		case r == '1':
			r = 'I'
		case r == '8':
			r = 'B'
		case r == '-' || r == ' ' || r == '\t':
			continue
		}
		if !(r >= 'A' && r <= 'Z' || r >= '2' && r <= '7') {
			return ""
		}
		code.WriteRune(r)
	}
	if code.Len() != 8 {
		return ""
	}
	return code.String()[:4] + "-" + code.String()[4:]
}
