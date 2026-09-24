package postgres

import (
	"context"
	"errors"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const sessionColumns = `s.id::text, s.task_id::text, s.created_by_principal::text,
    s.created_by_client::text, s.client_label, s.source_session_reference,
    s.resumed_from_handoff_id::text, s.started_at, s.ended_at,
    s.last_activity_at, s.summary, s.entry_count`

type scanner interface{ Scan(...any) error }

func scanWorkSession(row scanner) (model.WorkSession, error) {
	var session model.WorkSession
	var label, sourceRef, handoffID, summary pgtype.Text
	var endedAt pgtype.Timestamptz
	err := row.Scan(&session.ID, &session.TaskID, &session.CreatedByPrincipal,
		&session.CreatedByClient, &label, &sourceRef, &handoffID,
		&session.StartedAt, &endedAt, &session.LastActivityAt, &summary, &session.EntryCount)
	if err != nil {
		return model.WorkSession{}, err
	}
	session.ClientLabel = textPointer(label)
	session.SourceSessionReference = textPointer(sourceRef)
	session.ResumedFromHandoffID = textPointer(handoffID)
	session.Summary = textPointer(summary)
	if endedAt.Valid {
		session.EndedAt = &endedAt.Time
	}
	return session, nil
}

func (s *Store) CreateWorkSession(ctx context.Context, actor model.Actor, in model.CreateSessionInput) (model.WorkSession, error) {
	if _, err := parseUUID(in.TaskID); err != nil {
		return model.WorkSession{}, model.ErrNotFound
	}
	var handoffID any
	if in.ResumedFromHandoffID != nil {
		if _, err := parseUUID(*in.ResumedFromHandoffID); err != nil {
			return model.WorkSession{}, model.ErrInvalid
		}
		handoffID = *in.ResumedFromHandoffID
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.WorkSession{}, err
	}
	defer tx.Rollback(ctx)
	if err := authorizeLockedTask(ctx, tx, actor, in.TaskID); err != nil {
		return model.WorkSession{}, err
	}
	if handoffID != nil {
		var found int
		err := tx.QueryRow(ctx, `SELECT 1 FROM checkpoints
			WHERE id = $1::uuid AND task_id = $2::uuid AND kind = 'handoff'`,
			handoffID, in.TaskID).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.WorkSession{}, model.ErrInvalid
		}
		if err != nil {
			return model.WorkSession{}, err
		}
	}
	row := tx.QueryRow(ctx, `INSERT INTO work_sessions
		(task_id, created_by_principal, created_by_client, client_label,
		 source_session_reference, resumed_from_handoff_id)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6::uuid)
		RETURNING id::text, task_id::text, created_by_principal::text,
		created_by_client::text, client_label, source_session_reference,
		resumed_from_handoff_id::text, started_at, ended_at,
		last_activity_at, summary, entry_count`,
		in.TaskID, actor.PrincipalID, actor.ClientID,
		optionalText(in.ClientLabel), optionalText(in.SourceSessionReference), handoffID)
	session, err := scanWorkSession(row)
	if err != nil {
		return model.WorkSession{}, dbError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'in_progress', updated_at = now()
		WHERE id = $1::uuid`, in.TaskID); err != nil {
		return model.WorkSession{}, err
	}
	if err := auditWrite(ctx, tx, actor, "session.create", "work_session", session.ID); err != nil {
		return model.WorkSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.WorkSession{}, err
	}
	return session, nil
}

func (s *Store) CloseWorkSession(ctx context.Context, actor model.Actor, in model.CloseSessionInput) (model.WorkSession, error) {
	if _, err := parseUUID(in.TaskID); err != nil {
		return model.WorkSession{}, model.ErrNotFound
	}
	if _, err := parseUUID(in.SessionID); err != nil {
		return model.WorkSession{}, model.ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.WorkSession{}, err
	}
	defer tx.Rollback(ctx)
	if err := authorizeLockedTask(ctx, tx, actor, in.TaskID); err != nil {
		return model.WorkSession{}, err
	}
	var ownerClientID string
	var endedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT created_by_client::text, ended_at
		FROM work_sessions WHERE id = $1::uuid AND task_id = $2::uuid FOR UPDATE`,
		in.SessionID, in.TaskID).Scan(&ownerClientID, &endedAt)
	if err != nil {
		return model.WorkSession{}, dbError(err)
	}
	if ownerClientID != actor.ClientID {
		return model.WorkSession{}, model.ErrForbidden
	}
	if endedAt.Valid {
		return model.WorkSession{}, model.ErrConflict
	}
	row := tx.QueryRow(ctx, `UPDATE work_sessions AS s
		SET summary = $3, ended_at = now(), last_activity_at = now()
		WHERE s.id = $1::uuid AND s.task_id = $2::uuid
		RETURNING `+sessionColumns, in.SessionID, in.TaskID, in.Summary)
	session, err := scanWorkSession(row)
	if err != nil {
		return model.WorkSession{}, dbError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET updated_at = now() WHERE id = $1::uuid`, in.TaskID); err != nil {
		return model.WorkSession{}, err
	}
	if err := auditWrite(ctx, tx, actor, "session.close", "work_session", session.ID); err != nil {
		return model.WorkSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.WorkSession{}, err
	}
	return session, nil
}

func authorizeLockedTask(ctx context.Context, tx pgx.Tx, actor model.Actor, taskID string) error {
	var spaceID, policy string
	err := tx.QueryRow(ctx, `SELECT p.space_id::text, p.sync_policy
		FROM tasks t JOIN projects p ON p.id = t.project_id
		WHERE t.id = $1::uuid FOR UPDATE OF t`, taskID).Scan(&spaceID, &policy)
	if err != nil {
		return dbError(err)
	}
	if !actor.SpaceIDs[spaceID] || policy != model.PolicyHub {
		return model.ErrNotFound
	}
	if !actor.Scopes[model.ScopeWrite] {
		return model.ErrForbidden
	}
	return nil
}

func (s *Store) ListTaskSessions(ctx context.Context, actor model.Actor, taskID string, limit int, cursor string) (model.Page[model.WorkSession], error) {
	if _, err := parseUUID(taskID); err != nil {
		return model.Page[model.WorkSession]{}, model.ErrNotFound
	}
	page, err := s.listTaskSessions(ctx, taskID, limit, cursor)
	if err != nil {
		return page, err
	}
	if err := s.auditRead(ctx, actor, "session.list", "task", taskID); err != nil {
		return model.Page[model.WorkSession]{}, err
	}
	return page, nil
}

func (s *Store) listTaskSessions(ctx context.Context, taskID string, limit int, cursor string) (model.Page[model.WorkSession], error) {
	position, err := parseActivityCursor(cursor)
	if err != nil {
		return model.Page[model.WorkSession]{}, err
	}
	var beforeAt, beforeID any
	if !position.At.IsZero() {
		beforeAt, beforeID = position.At, position.ID
	}
	rows, err := s.pool.Query(ctx, `SELECT `+sessionColumns+`
		FROM work_sessions s WHERE s.task_id = $1::uuid
		AND ($2::timestamptz IS NULL OR (s.last_activity_at, s.id) < ($2::timestamptz, $3::uuid))
		ORDER BY s.last_activity_at DESC, s.id DESC LIMIT $4`,
		taskID, beforeAt, beforeID, limit+1)
	if err != nil {
		return model.Page[model.WorkSession]{}, err
	}
	defer rows.Close()
	page := model.Page[model.WorkSession]{Items: make([]model.WorkSession, 0, limit)}
	for rows.Next() {
		session, err := scanWorkSession(rows)
		if err != nil {
			return model.Page[model.WorkSession]{}, err
		}
		if len(page.Items) == limit {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = encodeCursor(activityCursor{At: last.LastActivityAt, ID: last.ID})
			break
		}
		page.Items = append(page.Items, session)
	}
	if err := rows.Err(); err != nil {
		return model.Page[model.WorkSession]{}, err
	}
	return page, nil
}

func (s *Store) SessionDetail(ctx context.Context, actor model.Actor, taskID, sessionID string) (model.SessionDetail, error) {
	session, err := s.getSession(ctx, taskID, sessionID)
	if err != nil {
		return model.SessionDetail{}, err
	}
	entries, err := s.listSessionEntries(ctx, sessionID, 20, "")
	if err != nil {
		return model.SessionDetail{}, err
	}
	if err := s.auditRead(ctx, actor, "session.read", "work_session", sessionID); err != nil {
		return model.SessionDetail{}, err
	}
	return model.SessionDetail{Session: session, Entries: entries}, nil
}

func (s *Store) SessionEntries(ctx context.Context, actor model.Actor, taskID, sessionID string, limit int, cursor string) (model.Page[model.SessionEntry], error) {
	if _, err := s.getSession(ctx, taskID, sessionID); err != nil {
		return model.Page[model.SessionEntry]{}, err
	}
	page, err := s.listSessionEntries(ctx, sessionID, limit, cursor)
	if err != nil {
		return page, err
	}
	if err := s.auditRead(ctx, actor, "session.entries.read", "work_session", sessionID); err != nil {
		return model.Page[model.SessionEntry]{}, err
	}
	return page, nil
}

func (s *Store) getSession(ctx context.Context, taskID, sessionID string) (model.WorkSession, error) {
	if _, err := parseUUID(taskID); err != nil {
		return model.WorkSession{}, model.ErrNotFound
	}
	if _, err := parseUUID(sessionID); err != nil {
		return model.WorkSession{}, model.ErrNotFound
	}
	session, err := scanWorkSession(s.pool.QueryRow(ctx, `SELECT `+sessionColumns+`
		FROM work_sessions s WHERE s.id = $1::uuid AND s.task_id = $2::uuid`,
		sessionID, taskID))
	if err != nil {
		return model.WorkSession{}, dbError(err)
	}
	return session, nil
}

func (s *Store) listSessionEntries(ctx context.Context, sessionID string, limit int, cursor string) (model.Page[model.SessionEntry], error) {
	after, err := parseSequenceCursor(cursor)
	if err != nil {
		return model.Page[model.SessionEntry]{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT se.sequence,
		c.id::text, c.task_id::text, c.session_id::text, c.task_version, c.kind,
		c.summary, c.completed, c.remaining, c.warnings, c.changed_paths,
		c.test_results, c.source_revision, c.created_by_principal::text,
		c.created_by_client::text, c.created_at
		FROM session_entries se JOIN checkpoints c ON c.id = se.checkpoint_id
		WHERE se.session_id = $1::uuid AND se.sequence > $2
		ORDER BY se.sequence ASC LIMIT $3`, sessionID, after, limit+1)
	if err != nil {
		return model.Page[model.SessionEntry]{}, err
	}
	defer rows.Close()
	page := model.Page[model.SessionEntry]{Items: make([]model.SessionEntry, 0, limit)}
	for rows.Next() {
		entry, err := scanSessionEntry(rows)
		if err != nil {
			return model.Page[model.SessionEntry]{}, err
		}
		if len(page.Items) == limit {
			page.NextCursor = encodeCursor(sequenceCursor{Sequence: page.Items[len(page.Items)-1].Sequence})
			break
		}
		page.Items = append(page.Items, entry)
	}
	if err := rows.Err(); err != nil {
		return model.Page[model.SessionEntry]{}, err
	}
	return page, nil
}

func scanSessionEntry(row scanner) (model.SessionEntry, error) {
	var entry model.SessionEntry
	var sessionID string
	var revision pgtype.Text
	c := &entry.Checkpoint
	err := row.Scan(&entry.Sequence, &c.ID, &c.TaskID, &sessionID,
		&c.TaskVersion, &c.Kind, &c.Summary, &c.Completed, &c.Remaining,
		&c.Warnings, &c.ChangedPaths, &c.TestResults, &revision,
		&c.CreatedByPrincipal, &c.CreatedByClient, &c.CreatedAt)
	if err != nil {
		return model.SessionEntry{}, err
	}
	c.SessionID = &sessionID
	c.SourceRevision = textPointer(revision)
	return entry, nil
}
