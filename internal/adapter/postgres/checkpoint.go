package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) AppendCheckpoint(ctx context.Context, actor model.Actor, in model.AppendCheckpointInput) (model.Checkpoint, error) {
	var sessionID any
	if in.SessionID != nil {
		if _, err := parseUUID(*in.SessionID); err != nil {
			return model.Checkpoint{}, model.ErrInvalid
		}
		sessionID = *in.SessionID
	}
	canonical := in
	canonical.Completed = nonnil(in.Completed)
	canonical.Remaining = nonnil(in.Remaining)
	canonical.Warnings = nonnil(in.Warnings)
	canonical.ChangedPaths = nonnil(in.ChangedPaths)
	canonical.TestResults = nonnil(in.TestResults)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return model.Checkpoint{}, fmt.Errorf("encode checkpoint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Checkpoint{}, err
	}
	defer tx.Rollback(ctx)
	var currentVersion int64
	var spaceID, policy string
	err = tx.QueryRow(ctx, `SELECT t.version, p.space_id::text, p.sync_policy
		FROM tasks t JOIN projects p ON p.id = t.project_id
		WHERE t.id = $1::uuid FOR UPDATE OF t`, in.TaskID).Scan(&currentVersion, &spaceID, &policy)
	if err != nil {
		return model.Checkpoint{}, dbError(err)
	}
	if !actor.SpaceIDs[spaceID] || policy != model.PolicyHub {
		return model.Checkpoint{}, model.ErrNotFound
	}
	if !actor.Scopes[model.ScopeWrite] {
		return model.Checkpoint{}, model.ErrForbidden
	}
	previous, previousDigest, err := findCheckpointByKey(ctx, tx, actor.ClientID, in.IdempotencyKey)
	if err == nil {
		if previous.TaskID == in.TaskID && bytes.Equal(previousDigest, digest[:]) {
			return previous, nil
		}
		return model.Checkpoint{}, model.ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Checkpoint{}, err
	}
	if currentVersion != in.ExpectedVersion {
		return model.Checkpoint{}, model.ErrConflict
	}
	var nextSequence int64
	if in.SessionID != nil {
		var ownerClientID string
		var endedAt pgtype.Timestamptz
		err = tx.QueryRow(ctx, `SELECT created_by_client::text, ended_at, entry_count
			FROM work_sessions WHERE id = $1::uuid AND task_id = $2::uuid FOR UPDATE`,
			*in.SessionID, in.TaskID).Scan(&ownerClientID, &endedAt, &nextSequence)
		if err != nil {
			return model.Checkpoint{}, dbError(err)
		}
		if ownerClientID != actor.ClientID {
			return model.Checkpoint{}, model.ErrForbidden
		}
		if endedAt.Valid {
			return model.Checkpoint{}, model.ErrConflict
		}
		nextSequence++
	}
	var id string
	var createdAt time.Time
	var nextVersion int64
	err = tx.QueryRow(ctx, `INSERT INTO checkpoints
		(task_id, task_version, kind, summary, completed, remaining, warnings,
		 changed_paths, test_results, source_revision, created_by_principal,
		 created_by_client, idempotency_key, request_digest, session_id)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		        $11::uuid, $12::uuid, $13, $14, $15::uuid)
		RETURNING id::text, task_version, created_at`,
		in.TaskID, currentVersion+1, in.Kind, in.Summary,
		canonical.Completed, canonical.Remaining, canonical.Warnings,
		canonical.ChangedPaths, canonical.TestResults, optionalText(in.SourceRevision),
		actor.PrincipalID, actor.ClientID, in.IdempotencyKey, digest[:], sessionID,
	).Scan(&id, &nextVersion, &createdAt)
	if err != nil {
		return model.Checkpoint{}, dbError(err)
	}
	if in.SessionID != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO session_entries (session_id, sequence, checkpoint_id)
			VALUES ($1::uuid, $2, $3::uuid)`, *in.SessionID, nextSequence, id); err != nil {
			return model.Checkpoint{}, dbError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE work_sessions SET entry_count = $2, last_activity_at = now()
			WHERE id = $1::uuid`, *in.SessionID, nextSequence); err != nil {
			return model.Checkpoint{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET version = $2, status = 'in_progress',
		updated_at = now() WHERE id = $1::uuid`, in.TaskID, nextVersion); err != nil {
		return model.Checkpoint{}, err
	}
	if err := auditWrite(ctx, tx, actor, "checkpoint.append", "checkpoint", id); err != nil {
		return model.Checkpoint{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Checkpoint{}, err
	}
	return model.Checkpoint{ID: id, TaskID: in.TaskID, SessionID: in.SessionID, TaskVersion: nextVersion,
		Kind: in.Kind, Summary: in.Summary, Completed: canonical.Completed,
		Remaining: canonical.Remaining, Warnings: canonical.Warnings,
		ChangedPaths: canonical.ChangedPaths, TestResults: canonical.TestResults,
		SourceRevision: in.SourceRevision, CreatedByPrincipal: actor.PrincipalID,
		CreatedByClient: actor.ClientID, CreatedAt: createdAt}, nil
}

func findCheckpointByKey(ctx context.Context, tx pgx.Tx, clientID, key string) (model.Checkpoint, []byte, error) {
	var c model.Checkpoint
	var revision pgtype.Text
	var sessionID pgtype.Text
	var digest []byte
	err := tx.QueryRow(ctx, `SELECT id::text, task_id::text, session_id::text, task_version, kind, summary,
		completed, remaining, warnings, changed_paths, test_results,
		source_revision, created_by_principal::text, created_by_client::text,
		created_at, request_digest
		FROM checkpoints WHERE created_by_client = $1::uuid AND idempotency_key = $2`,
		clientID, key,
	).Scan(&c.ID, &c.TaskID, &sessionID, &c.TaskVersion, &c.Kind, &c.Summary,
		&c.Completed, &c.Remaining, &c.Warnings, &c.ChangedPaths,
		&c.TestResults, &revision, &c.CreatedByPrincipal, &c.CreatedByClient,
		&c.CreatedAt, &digest)
	if err != nil {
		return model.Checkpoint{}, nil, err
	}
	c.SourceRevision = textPointer(revision)
	c.SessionID = textPointer(sessionID)
	return c, digest, nil
}

func nonnil(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
