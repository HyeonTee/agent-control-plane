package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	deviceauth "github.com/HyeonTee/agent-control-plane/internal/domain/deviceauth"
	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDevicePending  = deviceauth.ErrPending
	ErrDeviceSlowDown = deviceauth.ErrSlowDown
	ErrDeviceDenied   = deviceauth.ErrDenied
	ErrDeviceExpired  = deviceauth.ErrExpired
	ErrRefreshReplay  = deviceauth.ErrRefreshReplay
)

const accessLifetime = 15 * time.Minute

type DeviceAuthorization = deviceauth.Authorization
type DeviceTokens = deviceauth.Tokens
type DeviceClient = deviceauth.Client

type DeviceAuthStore struct{ pool *pgxpool.Pool }

func NewDeviceAuthStore(pool *pgxpool.Pool) *DeviceAuthStore { return &DeviceAuthStore{pool: pool} }

func randomSecret(prefix string) (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(data), nil
}

func randomUserCode() (string, error) {
	data := make([]byte, 5)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(data)
	return encoded[:4] + "-" + encoded[4:], nil
}

func validDeviceScopes(scopes []string) bool {
	if len(scopes) == 0 || len(scopes) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if (scope != model.ScopeRead && scope != model.ScopeWrite) || seen[scope] {
			return false
		}
		seen[scope] = true
	}
	return true
}

func (s *DeviceAuthStore) StartDevice(ctx context.Context, label string, scopes []string) (DeviceAuthorization, error) {
	label = strings.TrimSpace(label)
	if label == "" || len(label) > 120 || !validDeviceScopes(scopes) {
		return DeviceAuthorization{}, model.ErrInvalid
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM device_authorizations
		WHERE device_code_digest IN (SELECT device_code_digest FROM device_authorizations
		WHERE expires_at < now() ORDER BY expires_at LIMIT 1000)`); err != nil {
		return DeviceAuthorization{}, err
	}
	deviceCode, err := randomSecret("acd_")
	if err != nil {
		return DeviceAuthorization{}, err
	}
	userCode, err := randomUserCode()
	if err != nil {
		return DeviceAuthorization{}, err
	}
	deviceDigest := sha256.Sum256([]byte(deviceCode))
	userDigest := sha256.Sum256([]byte(userCode))
	var expires time.Time
	err = s.pool.QueryRow(ctx, `INSERT INTO device_authorizations
		(device_code_digest, user_code_digest, client_label, scopes, expires_at)
		VALUES ($1, $2, $3, $4, now() + interval '10 minutes') RETURNING expires_at`,
		deviceDigest[:], userDigest[:], label, scopes).Scan(&expires)
	if err != nil {
		return DeviceAuthorization{}, err
	}
	return DeviceAuthorization{DeviceCode: deviceCode, UserCode: userCode, Label: label, Scopes: scopes, ExpiresAt: expires}, nil
}

func (s *DeviceAuthStore) FindDevice(ctx context.Context, userCode string) (DeviceAuthorization, error) {
	userCode = strings.ToUpper(strings.TrimSpace(userCode))
	if len(userCode) != 9 {
		return DeviceAuthorization{}, model.ErrNotFound
	}
	digest := sha256.Sum256([]byte(userCode))
	var result DeviceAuthorization
	err := s.pool.QueryRow(ctx, `SELECT client_label, scopes, expires_at FROM device_authorizations
		WHERE user_code_digest = $1 AND status = 'pending' AND expires_at > now()`, digest[:]).
		Scan(&result.Label, &result.Scopes, &result.ExpiresAt)
	if err != nil {
		return DeviceAuthorization{}, dbError(err)
	}
	result.UserCode = userCode
	return result, nil
}

func (s *DeviceAuthStore) DecideDevice(ctx context.Context, userCode string, approve bool) error {
	userCode = strings.ToUpper(strings.TrimSpace(userCode))
	if len(userCode) != 9 {
		return model.ErrNotFound
	}
	digest := sha256.Sum256([]byte(userCode))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var principalID, spaceID string
	err = tx.QueryRow(ctx, `SELECT p.id::text, s.id::text FROM principals p
			JOIN principal_spaces ps ON ps.principal_id = p.id
			JOIN spaces s ON s.id = ps.space_id
			WHERE p.name = 'owner' AND s.name = 'personal'`,
	).Scan(&principalID, &spaceID)
	if err != nil {
		return dbError(err)
	}
	status := "denied"
	if approve {
		status = "approved"
	}
	command, err := tx.Exec(ctx, `UPDATE device_authorizations SET status = $2,
		principal_id = NULLIF($3, '')::uuid, space_id = NULLIF($4, '')::uuid
		WHERE user_code_digest = $1 AND status = 'pending' AND expires_at > now()`,
		digest[:], status, principalID, spaceID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return model.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (principal_id, action, resource_type, result)
		VALUES (NULLIF($1, '')::uuid, 'device.' || $2, 'device_authorization', 'success')`,
		principalID, status); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *DeviceAuthStore) ExchangeDevice(ctx context.Context, deviceCode string) (DeviceTokens, error) {
	if len(deviceCode) != 47 || !strings.HasPrefix(deviceCode, "acd_") {
		return DeviceTokens{}, ErrDeviceExpired
	}
	digest := sha256.Sum256([]byte(deviceCode))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceTokens{}, err
	}
	defer tx.Rollback(ctx)
	var status, principalID, spaceID, label string
	var scopes []string
	var expires time.Time
	var lastPolled *time.Time
	err = tx.QueryRow(ctx, `SELECT status, COALESCE(principal_id::text, ''),
		COALESCE(space_id::text, ''), client_label, scopes, expires_at, last_polled_at
		FROM device_authorizations WHERE device_code_digest = $1 FOR UPDATE`, digest[:]).
		Scan(&status, &principalID, &spaceID, &label, &scopes, &expires, &lastPolled)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeviceTokens{}, ErrDeviceExpired
	}
	if err != nil {
		return DeviceTokens{}, err
	}
	if !time.Now().Before(expires) || status == "consumed" {
		return DeviceTokens{}, ErrDeviceExpired
	}
	if status == "denied" {
		return DeviceTokens{}, ErrDeviceDenied
	}
	if status == "pending" {
		if lastPolled != nil && time.Since(*lastPolled) < 5*time.Second {
			return DeviceTokens{}, ErrDeviceSlowDown
		}
		if _, err := tx.Exec(ctx, `UPDATE device_authorizations SET last_polled_at = now()
			WHERE device_code_digest = $1`, digest[:]); err != nil {
			return DeviceTokens{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return DeviceTokens{}, err
		}
		return DeviceTokens{}, ErrDevicePending
	}
	issued, err := insertToken(ctx, tx, principalID, spaceID, label, scopes, accessLifetime)
	if err != nil {
		return DeviceTokens{}, err
	}
	refresh, err := randomSecret("acr_")
	if err != nil {
		return DeviceTokens{}, err
	}
	refreshDigest := sha256.Sum256([]byte(refresh))
	if _, err := tx.Exec(ctx, `INSERT INTO device_refresh_tokens (token_id, secret_digest, expires_at)
		VALUES ($1::uuid, $2, now() + interval '90 days')`, issued.ClientID, refreshDigest[:]); err != nil {
		return DeviceTokens{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE device_authorizations SET status = 'consumed'
		WHERE device_code_digest = $1`, digest[:]); err != nil {
		return DeviceTokens{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeviceTokens{}, err
	}
	return DeviceTokens{AccessToken: issued.Secret, RefreshToken: refresh,
		TokenType: "Bearer", ExpiresIn: int(accessLifetime.Seconds())}, nil
}

func (s *DeviceAuthStore) RefreshDevice(ctx context.Context, refresh string) (DeviceTokens, error) {
	if len(refresh) != 47 || !strings.HasPrefix(refresh, "acr_") {
		return DeviceTokens{}, model.ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(refresh))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeviceTokens{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM device_refresh_used
		WHERE secret_digest IN (SELECT secret_digest FROM device_refresh_used
		WHERE used_at < now() - interval '90 days' ORDER BY used_at LIMIT 1000)`); err != nil {
		return DeviceTokens{}, err
	}
	var tokenID string
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT token_id::text, expires_at FROM device_refresh_tokens
		WHERE secret_digest = $1 AND revoked_at IS NULL FOR UPDATE`, digest[:]).Scan(&tokenID, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		// A reused refresh token invalidates the entire device grant.
		err = tx.QueryRow(ctx, `SELECT token_id::text FROM device_refresh_used
			WHERE secret_digest = $1`, digest[:]).Scan(&tokenID)
		if errors.Is(err, pgx.ErrNoRows) {
			return DeviceTokens{}, model.ErrUnauthorized
		}
		if err != nil {
			return DeviceTokens{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE client_tokens SET revoked_at = now()
			WHERE id = $1::uuid AND revoked_at IS NULL`, tokenID); err != nil {
			return DeviceTokens{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE device_refresh_tokens SET revoked_at = now()
			WHERE token_id = $1::uuid`, tokenID); err != nil {
			return DeviceTokens{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_events
			(principal_id, client_id, action, resource_type, resource_id, result)
			SELECT principal_id, id, 'device.refresh_replay', 'client_token', id, 'revoked'
			FROM client_tokens WHERE id = $1::uuid`, tokenID); err != nil {
			return DeviceTokens{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return DeviceTokens{}, err
		}
		return DeviceTokens{}, ErrRefreshReplay
	}
	if err != nil {
		return DeviceTokens{}, err
	}
	if !time.Now().Before(expires) {
		return DeviceTokens{}, model.ErrUnauthorized
	}
	newAccess, err := randomSecret("acp_")
	if err != nil {
		return DeviceTokens{}, err
	}
	newRefresh, err := randomSecret("acr_")
	if err != nil {
		return DeviceTokens{}, err
	}
	accessDigest := sha256.Sum256([]byte(newAccess))
	refreshDigest := sha256.Sum256([]byte(newRefresh))
	updated, err := tx.Exec(ctx, `UPDATE client_tokens SET secret_digest = $2,
		expires_at = now() + interval '15 minutes' WHERE id = $1::uuid AND revoked_at IS NULL`,
		tokenID, accessDigest[:])
	if err != nil {
		return DeviceTokens{}, err
	}
	if updated.RowsAffected() != 1 {
		return DeviceTokens{}, model.ErrUnauthorized
	}
	if _, err := tx.Exec(ctx, `INSERT INTO device_refresh_used (secret_digest, token_id)
		VALUES ($1, $2::uuid)`, digest[:], tokenID); err != nil {
		return DeviceTokens{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE device_refresh_tokens SET secret_digest = $2,
		expires_at = now() + interval '90 days' WHERE token_id = $1::uuid`,
		tokenID, refreshDigest[:]); err != nil {
		return DeviceTokens{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events
		(principal_id, client_id, action, resource_type, resource_id, result)
		SELECT principal_id, id, 'device.refresh', 'client_token', id, 'success'
		FROM client_tokens WHERE id = $1::uuid`, tokenID); err != nil {
		return DeviceTokens{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeviceTokens{}, err
	}
	return DeviceTokens{AccessToken: newAccess, RefreshToken: newRefresh,
		TokenType: "Bearer", ExpiresIn: int(accessLifetime.Seconds())}, nil
}

func (s *DeviceAuthStore) ListDevices(ctx context.Context) ([]DeviceClient, error) {
	rows, err := s.pool.Query(ctx, `SELECT ct.id::text, ct.display_name, ct.scopes, ct.created_at,
		ct.last_used_at, dr.expires_at, COALESCE(ct.revoked_at, dr.revoked_at)
		FROM client_tokens ct JOIN device_refresh_tokens dr ON dr.token_id = ct.id
		JOIN principals p ON p.id = ct.principal_id
		JOIN client_token_spaces cts ON cts.token_id = ct.id
		JOIN spaces s ON s.id = cts.space_id
		WHERE p.name = 'owner' AND s.name = 'personal'
		ORDER BY ct.created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices := []DeviceClient{}
	for rows.Next() {
		var device DeviceClient
		if err := rows.Scan(&device.ID, &device.Label, &device.Scopes, &device.CreatedAt,
			&device.LastUsedAt, &device.ExpiresAt, &device.RevokedAt); err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (s *DeviceAuthStore) RevokeDevice(ctx context.Context, id string) error {
	if _, err := parseUUID(id); err != nil {
		return model.ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE client_tokens ct SET revoked_at = now()
		FROM device_refresh_tokens dr, principals p, client_token_spaces cts, spaces s
		WHERE ct.id = $1::uuid AND dr.token_id = ct.id AND p.id = ct.principal_id
		AND p.name = 'owner' AND cts.token_id = ct.id AND s.id = cts.space_id
		AND s.name = 'personal' AND ct.revoked_at IS NULL`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return model.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE device_refresh_tokens SET revoked_at = now()
		WHERE token_id = $1::uuid`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (action, resource_type, resource_id, result)
		VALUES ('device.revoke', 'client_token', $1::uuid, 'success')`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
