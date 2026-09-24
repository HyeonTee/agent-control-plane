package postgres

import (
	"context"
	"errors"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
	"github.com/HyeonTee/agent-control-plane/internal/requestid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const checkpointColumns = `c.id::text, c.task_id::text, c.session_id::text,
    c.task_version, c.kind, c.summary, c.completed, c.remaining,
    c.warnings, c.changed_paths, c.test_results, c.source_revision,
    c.created_by_principal::text, c.created_by_client::text, c.created_at`

func scanCheckpoint(row scanner) (model.Checkpoint, error) {
	var checkpoint model.Checkpoint
	var sessionID, sourceRevision pgtype.Text
	err := row.Scan(&checkpoint.ID, &checkpoint.TaskID, &sessionID,
		&checkpoint.TaskVersion, &checkpoint.Kind, &checkpoint.Summary,
		&checkpoint.Completed, &checkpoint.Remaining, &checkpoint.Warnings,
		&checkpoint.ChangedPaths, &checkpoint.TestResults, &sourceRevision,
		&checkpoint.CreatedByPrincipal, &checkpoint.CreatedByClient, &checkpoint.CreatedAt)
	if err != nil {
		return model.Checkpoint{}, err
	}
	checkpoint.SessionID = textPointer(sessionID)
	checkpoint.SourceRevision = textPointer(sourceRevision)
	return checkpoint, nil
}

func (s *Store) ListActiveTasks(ctx context.Context, actor model.Actor, status string, limit int, cursor string) (model.Page[model.TaskCard], error) {
	position, err := parseActivityCursor(cursor)
	if err != nil {
		return model.Page[model.TaskCard]{}, err
	}
	var beforeAt, beforeID any
	if !position.At.IsZero() {
		beforeAt, beforeID = position.At, position.ID
	}
	rows, err := s.pool.Query(ctx, `SELECT t.id::text, p.id::text, p.name,
		p.space_id::text, sp.name, t.title, t.status, t.version,
		left(latest.summary, 500), t.updated_at,
		(SELECT count(*) FROM work_sessions ws WHERE ws.task_id = t.id),
		EXISTS (SELECT 1 FROM checkpoints old WHERE old.task_id = t.id AND old.session_id IS NULL)
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		JOIN spaces sp ON sp.id = p.space_id
		JOIN client_token_spaces cts ON cts.space_id = p.space_id AND cts.token_id = $1::uuid
		JOIN client_tokens ct ON ct.id = cts.token_id AND ct.principal_id = $2::uuid
		JOIN principal_spaces ps ON ps.space_id = p.space_id AND ps.principal_id = $2::uuid
		LEFT JOIN LATERAL (SELECT summary FROM checkpoints c WHERE c.task_id = t.id
			ORDER BY c.task_version DESC LIMIT 1) latest ON true
		WHERE ct.revoked_at IS NULL AND ct.expires_at > now()
		AND p.sync_policy = 'metadata_and_handoffs'
		AND ($3 = 'all' OR ($3 = 'active' AND t.status IN ('todo','in_progress','blocked')) OR t.status = $3)
		AND ($4::timestamptz IS NULL OR (t.updated_at, t.id) < ($4::timestamptz, $5::uuid))
		ORDER BY t.updated_at DESC, t.id DESC LIMIT $6`,
		actor.ClientID, actor.PrincipalID, status, beforeAt, beforeID, limit+1)
	if err != nil {
		return model.Page[model.TaskCard]{}, err
	}
	defer rows.Close()
	page := model.Page[model.TaskCard]{Items: make([]model.TaskCard, 0, limit)}
	for rows.Next() {
		var card model.TaskCard
		var summary pgtype.Text
		err := rows.Scan(&card.ID, &card.ProjectID, &card.ProjectName,
			&card.SpaceID, &card.SpaceName, &card.Title, &card.Status,
			&card.Version, &summary, &card.LastActivityAt,
			&card.SessionCount, &card.HasPreSessionHistory)
		if err != nil {
			return model.Page[model.TaskCard]{}, err
		}
		card.LatestSummary = textPointer(summary)
		if len(page.Items) == limit {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = encodeCursor(activityCursor{At: last.LastActivityAt, ID: last.ID})
			break
		}
		page.Items = append(page.Items, card)
	}
	if err := rows.Err(); err != nil {
		return model.Page[model.TaskCard]{}, err
	}
	rows.Close()
	if _, err := s.pool.Exec(ctx, `INSERT INTO audit_events
		(principal_id, client_id, action, resource_type, result, request_id)
		VALUES ($1::uuid, $2::uuid, 'task.discover', 'task', 'success', $3)`,
		actor.PrincipalID, actor.ClientID, requestid.From(ctx)); err != nil {
		return model.Page[model.TaskCard]{}, err
	}
	return page, nil
}

func (s *Store) TaskOverview(ctx context.Context, actor model.Actor, taskID string) (model.TaskOverview, error) {
	if _, err := parseUUID(taskID); err != nil {
		return model.TaskOverview{}, model.ErrNotFound
	}
	var overview model.TaskOverview
	var baseRevision pgtype.Text
	err := s.pool.QueryRow(ctx, `SELECT t.id::text, t.project_id::text, t.title, t.objective,
		t.status, t.version, t.base_revision, t.created_at, t.updated_at,
		p.name, p.space_id::text, sp.name
		FROM tasks t JOIN projects p ON p.id = t.project_id
		JOIN spaces sp ON sp.id = p.space_id
		JOIN client_token_spaces cts ON cts.space_id = p.space_id AND cts.token_id = $2::uuid
		JOIN client_tokens ct ON ct.id = cts.token_id AND ct.principal_id = $3::uuid
		JOIN principal_spaces ps ON ps.space_id = p.space_id AND ps.principal_id = $3::uuid
		WHERE t.id = $1::uuid AND p.sync_policy = 'metadata_and_handoffs'
		AND ct.revoked_at IS NULL AND ct.expires_at > now()`,
		taskID, actor.ClientID, actor.PrincipalID).Scan(
		&overview.Task.ID, &overview.Task.ProjectID, &overview.Task.Title,
		&overview.Task.Objective, &overview.Task.Status, &overview.Task.Version,
		&baseRevision, &overview.Task.CreatedAt, &overview.Task.UpdatedAt,
		&overview.Project.Name, &overview.Project.SpaceID, &overview.Project.SpaceName)
	if err != nil {
		return model.TaskOverview{}, dbError(err)
	}
	overview.Task.BaseRevision = textPointer(baseRevision)
	overview.Project.ID = overview.Task.ProjectID

	recent, err := s.listRecentCheckpoints(ctx, taskID, 10)
	if err != nil {
		return model.TaskOverview{}, err
	}
	overview.RecentCheckpoints = recent
	if len(recent) > 0 {
		overview.LatestCheckpoint = &recent[0]
	}
	handoff, err := scanCheckpoint(s.pool.QueryRow(ctx, `SELECT `+checkpointColumns+`
		FROM checkpoints c WHERE c.task_id = $1::uuid AND c.kind = 'handoff'
		ORDER BY c.task_version DESC LIMIT 1`, taskID))
	if err == nil {
		overview.LatestHandoff = &handoff
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return model.TaskOverview{}, err
	}
	overview.Sessions, err = s.listTaskSessions(ctx, taskID, 10, "")
	if err != nil {
		return model.TaskOverview{}, err
	}
	overview.PreSessionHistory, err = s.listPreSessionHistory(ctx, taskID, 10, "")
	if err != nil {
		return model.TaskOverview{}, err
	}
	overview.HasPreSessionHistory = len(overview.PreSessionHistory.Items) > 0
	if err := s.auditRead(ctx, actor, "task.overview.read", "task", taskID); err != nil {
		return model.TaskOverview{}, err
	}
	return overview, nil
}

func (s *Store) listRecentCheckpoints(ctx context.Context, taskID string, limit int) ([]model.Checkpoint, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+checkpointColumns+`
		FROM checkpoints c WHERE c.task_id = $1::uuid
		ORDER BY c.task_version DESC LIMIT $2`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.Checkpoint, 0, limit)
	for rows.Next() {
		checkpoint, err := scanCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, checkpoint)
	}
	return items, rows.Err()
}

func (s *Store) PreSessionHistory(ctx context.Context, actor model.Actor, taskID string, limit int, cursor string) (model.Page[model.Checkpoint], error) {
	if _, err := parseUUID(taskID); err != nil {
		return model.Page[model.Checkpoint]{}, model.ErrNotFound
	}
	page, err := s.listPreSessionHistory(ctx, taskID, limit, cursor)
	if err != nil {
		return page, err
	}
	if err := s.auditRead(ctx, actor, "task.pre_session_history.read", "task", taskID); err != nil {
		return model.Page[model.Checkpoint]{}, err
	}
	return page, nil
}

func (s *Store) listPreSessionHistory(ctx context.Context, taskID string, limit int, cursor string) (model.Page[model.Checkpoint], error) {
	before, err := parseSequenceCursor(cursor)
	if err != nil {
		return model.Page[model.Checkpoint]{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+checkpointColumns+`
		FROM checkpoints c WHERE c.task_id = $1::uuid AND c.session_id IS NULL
		AND ($2::bigint = 0 OR c.task_version < $2::bigint)
		ORDER BY c.task_version DESC LIMIT $3`, taskID, before, limit+1)
	if err != nil {
		return model.Page[model.Checkpoint]{}, err
	}
	defer rows.Close()
	page := model.Page[model.Checkpoint]{Items: make([]model.Checkpoint, 0, limit)}
	for rows.Next() {
		checkpoint, err := scanCheckpoint(rows)
		if err != nil {
			return model.Page[model.Checkpoint]{}, err
		}
		if len(page.Items) == limit {
			page.NextCursor = encodeCursor(sequenceCursor{Sequence: page.Items[len(page.Items)-1].TaskVersion})
			break
		}
		page.Items = append(page.Items, checkpoint)
	}
	if err := rows.Err(); err != nil {
		return model.Page[model.Checkpoint]{}, err
	}
	return page, nil
}
