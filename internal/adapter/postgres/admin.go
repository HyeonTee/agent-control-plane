package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IssuedToken struct {
	ClientID  string    `json:"client_id"`
	Secret    string    `json:"secret"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BootstrapResult struct {
	PrincipalID string `json:"principal_id"`
	SpaceID     string `json:"space_id"`
	IssuedToken
}

func BootstrapOwner(ctx context.Context, pool *pgxpool.Pool, spaceName, tokenName string) (BootstrapResult, error) {
	if !validAdminName(spaceName) || !validAdminName(tokenName) {
		return BootstrapResult{}, model.ErrInvalid
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(17432806901235)); err != nil {
		return BootstrapResult{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM principals").Scan(&count); err != nil {
		return BootstrapResult{}, err
	}
	if count != 0 {
		return BootstrapResult{}, model.ErrConflict
	}
	var result BootstrapResult
	if err := tx.QueryRow(ctx, "INSERT INTO principals (name) VALUES ('owner') RETURNING id::text").Scan(&result.PrincipalID); err != nil {
		return BootstrapResult{}, err
	}
	if err := tx.QueryRow(ctx, "INSERT INTO spaces (name) VALUES ($1) RETURNING id::text", strings.TrimSpace(spaceName)).Scan(&result.SpaceID); err != nil {
		return BootstrapResult{}, dbError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO principal_spaces (principal_id, space_id)
		VALUES ($1::uuid, $2::uuid)`, result.PrincipalID, result.SpaceID); err != nil {
		return BootstrapResult{}, err
	}
	result.IssuedToken, err = insertToken(ctx, tx, result.PrincipalID, result.SpaceID, tokenName,
		[]string{model.ScopeRead, model.ScopeWrite}, 90*24*time.Hour)
	if err != nil {
		return BootstrapResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BootstrapResult{}, err
	}
	return result, nil
}

func CreateToken(ctx context.Context, pool *pgxpool.Pool, spaceID, name string, scopes []string, ttl time.Duration) (IssuedToken, error) {
	if !validAdminName(name) || ttl <= 0 || ttl > 365*24*time.Hour || len(scopes) == 0 {
		return IssuedToken{}, model.ErrInvalid
	}
	seen := make(map[string]bool)
	for _, scope := range scopes {
		if (scope != model.ScopeRead && scope != model.ScopeWrite) || seen[scope] {
			return IssuedToken{}, model.ErrInvalid
		}
		seen[scope] = true
	}
	if _, err := parseUUID(spaceID); err != nil {
		return IssuedToken{}, model.ErrInvalid
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return IssuedToken{}, err
	}
	defer tx.Rollback(ctx)
	var principalID string
	err = tx.QueryRow(ctx, `SELECT p.id::text FROM principals p
		JOIN principal_spaces ps ON ps.principal_id = p.id
		WHERE p.name = 'owner' AND ps.space_id = $1::uuid`, spaceID).Scan(&principalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return IssuedToken{}, model.ErrNotFound
	}
	if err != nil {
		return IssuedToken{}, err
	}
	issued, err := insertToken(ctx, tx, principalID, spaceID, name, scopes, ttl)
	if err != nil {
		return IssuedToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedToken{}, err
	}
	return issued, nil
}

func RevokeToken(ctx context.Context, pool *pgxpool.Pool, clientID string) error {
	if _, err := parseUUID(clientID); err != nil {
		return model.ErrInvalid
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var principalID string
	err = tx.QueryRow(ctx, `UPDATE client_tokens SET revoked_at = now()
		WHERE id = $1::uuid AND revoked_at IS NULL RETURNING principal_id::text`, clientID).Scan(&principalID)
	if err != nil {
		return dbError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events
		(principal_id, action, resource_type, resource_id, result)
		VALUES ($1::uuid, 'token.revoke', 'client_token', $2::uuid, 'success')`,
		principalID, clientID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertToken(ctx context.Context, tx pgx.Tx, principalID, spaceID, name string, scopes []string, ttl time.Duration) (IssuedToken, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return IssuedToken{}, fmt.Errorf("generate token: %w", err)
	}
	secret := "acp_" + base64.RawURLEncoding.EncodeToString(random)
	digest := sha256.Sum256([]byte(secret))
	issued := IssuedToken{Secret: secret}
	err := tx.QueryRow(ctx, `INSERT INTO client_tokens
		(principal_id, display_name, secret_digest, scopes, expires_at)
		VALUES ($1::uuid, $2, $3, $4, now() + $5::interval)
		RETURNING id::text, expires_at`, principalID, strings.TrimSpace(name), digest[:], scopes,
		fmt.Sprintf("%d seconds", int64(ttl.Seconds())),
	).Scan(&issued.ClientID, &issued.ExpiresAt)
	if err != nil {
		return IssuedToken{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO client_token_spaces (token_id, space_id)
		VALUES ($1::uuid, $2::uuid)`, issued.ClientID, spaceID); err != nil {
		return IssuedToken{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events
		(principal_id, client_id, action, resource_type, resource_id, result)
		VALUES ($1::uuid, $2::uuid, 'token.create', 'client_token', $2::uuid, 'success')`,
		principalID, issued.ClientID); err != nil {
		return IssuedToken{}, err
	}
	return issued, nil
}

func validAdminName(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && len(trimmed) <= 120
}
