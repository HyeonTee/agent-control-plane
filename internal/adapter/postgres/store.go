package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/HyeonTee/agent-control-plane/internal/adapter/postgres/db"
	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
	"github.com/HyeonTee/agent-control-plane/internal/requestid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, q: db.New(pool)} }

func (s *Store) Authenticate(ctx context.Context, secret string) (model.Actor, error) {
	if len(secret) != 47 || !strings.HasPrefix(secret, "acp_") {
		return model.Actor{}, model.ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(secret))
	token, err := s.q.GetTokenByDigest(ctx, digest[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Actor{}, model.ErrUnauthorized
	}
	if err != nil {
		return model.Actor{}, fmt.Errorf("find token: %w", err)
	}
	spaces, err := s.q.GetTokenSpaces(ctx, mustUUID(token.ID))
	if err != nil {
		return model.Actor{}, fmt.Errorf("find token spaces: %w", err)
	}
	if len(spaces) == 0 {
		return model.Actor{}, model.ErrUnauthorized
	}
	actor := model.Actor{
		PrincipalID: token.PrincipalID,
		ClientID:    token.ID,
		Scopes:      make(map[string]bool, len(token.Scopes)),
		SpaceIDs:    make(map[string]bool, len(spaces)),
	}
	for _, scope := range token.Scopes {
		actor.Scopes[scope] = true
	}
	for _, space := range spaces {
		actor.SpaceIDs[space.ID] = true
	}
	if err := s.q.TouchToken(ctx, mustUUID(token.ID)); err != nil {
		return model.Actor{}, fmt.Errorf("touch token: %w", err)
	}
	return actor, nil
}

func (s *Store) ListSpaces(ctx context.Context, actor model.Actor) ([]model.Space, error) {
	rows, err := s.q.GetTokenSpaces(ctx, mustUUID(actor.ClientID))
	if err != nil {
		return nil, err
	}
	spaces := make([]model.Space, 0, len(rows))
	for _, row := range rows {
		spaces = append(spaces, model.Space{ID: row.ID, Name: row.Name})
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO audit_events
		(principal_id, client_id, action, resource_type, result, request_id)
		VALUES ($1::uuid, $2::uuid, 'space.list', 'space', 'success', $3)`,
		actor.PrincipalID, actor.ClientID, requestid.From(ctx)); err != nil {
		return nil, err
	}
	return spaces, nil
}

func (s *Store) ProjectSpace(ctx context.Context, projectID string) (string, string, error) {
	id, err := parseUUID(projectID)
	if err != nil {
		return "", "", model.ErrNotFound
	}
	row, err := s.q.GetProjectSpace(ctx, id)
	if err != nil {
		return "", "", dbError(err)
	}
	return row.SpaceID, row.SyncPolicy, nil
}

func (s *Store) TaskSpace(ctx context.Context, taskID string) (string, string, error) {
	id, err := parseUUID(taskID)
	if err != nil {
		return "", "", model.ErrNotFound
	}
	row, err := s.q.GetTaskSpace(ctx, id)
	if err != nil {
		return "", "", dbError(err)
	}
	return row.SpaceID, row.SyncPolicy, nil
}

func (s *Store) CreateProject(ctx context.Context, actor model.Actor, in model.CreateProjectInput) (model.Project, error) {
	spaceID, err := parseUUID(in.SpaceID)
	if err != nil {
		return model.Project{}, model.ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Project{}, err
	}
	defer tx.Rollback(ctx)
	row, err := db.New(tx).CreateProject(ctx, db.CreateProjectParams{
		SpaceID: spaceID, Name: in.Name, Description: in.Description,
		RepositoryReference: optionalText(in.RepositoryReference), SyncPolicy: in.SyncPolicy,
	})
	if err != nil {
		return model.Project{}, dbError(err)
	}
	if err := auditWrite(ctx, tx, actor, "project.create", "project", row.ID); err != nil {
		return model.Project{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Project{}, err
	}
	return model.Project{ID: row.ID, SpaceID: row.SpaceID, Name: row.Name,
		Description: row.Description, RepositoryReference: textPointer(row.RepositoryReference),
		SyncPolicy: row.SyncPolicy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

func (s *Store) CreateTask(ctx context.Context, actor model.Actor, in model.CreateTaskInput) (model.Task, error) {
	projectID, err := parseUUID(in.ProjectID)
	if err != nil {
		return model.Task{}, model.ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Task{}, err
	}
	defer tx.Rollback(ctx)
	row, err := db.New(tx).CreateTask(ctx, db.CreateTaskParams{
		ProjectID: projectID, Title: in.Title, Objective: in.Objective,
		BaseRevision: optionalText(in.BaseRevision),
	})
	if err != nil {
		return model.Task{}, dbError(err)
	}
	if err := auditWrite(ctx, tx, actor, "task.create", "task", row.ID); err != nil {
		return model.Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Task{}, err
	}
	return model.Task{ID: row.ID, ProjectID: row.ProjectID, Title: row.Title,
		Objective: row.Objective, Status: row.Status, Version: row.Version,
		BaseRevision: textPointer(row.BaseRevision), CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time}, nil
}

func (s *Store) ListTasks(ctx context.Context, actor model.Actor, projectID string) ([]model.Task, error) {
	id, err := parseUUID(projectID)
	if err != nil {
		return nil, model.ErrNotFound
	}
	rows, err := s.q.ListTasks(ctx, id)
	if err != nil {
		return nil, err
	}
	tasks := make([]model.Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, model.Task{ID: row.ID, ProjectID: row.ProjectID,
			Title: row.Title, Objective: row.Objective, Status: row.Status,
			Version: row.Version, BaseRevision: textPointer(row.BaseRevision),
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time})
	}
	if err := s.auditRead(ctx, actor, "task.list", "project", projectID); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Store) TaskTimeline(ctx context.Context, actor model.Actor, taskID string) ([]model.Checkpoint, error) {
	id, err := parseUUID(taskID)
	if err != nil {
		return nil, model.ErrNotFound
	}
	rows, err := s.q.TaskTimeline(ctx, id)
	if err != nil {
		return nil, err
	}
	items := make([]model.Checkpoint, 0, len(rows))
	for _, row := range rows {
		sessionID, err := sessionPointer(row.SessionID)
		if err != nil {
			return nil, err
		}
		items = append(items, model.Checkpoint{ID: row.ID, TaskID: row.TaskID,
			SessionID:   sessionID,
			TaskVersion: row.TaskVersion, Kind: row.Kind, Summary: row.Summary,
			Completed: row.Completed, Remaining: row.Remaining, Warnings: row.Warnings,
			ChangedPaths: row.ChangedPaths, TestResults: row.TestResults,
			SourceRevision: textPointer(row.SourceRevision), CreatedByPrincipal: row.CreatedByPrincipal,
			CreatedByClient: row.CreatedByClient, CreatedAt: row.CreatedAt.Time})
	}
	if err := s.auditRead(ctx, actor, "checkpoint.timeline.read", "task", taskID); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) LatestHandoff(ctx context.Context, actor model.Actor, taskID string) (model.Checkpoint, error) {
	id, err := parseUUID(taskID)
	if err != nil {
		return model.Checkpoint{}, model.ErrNotFound
	}
	row, err := s.q.LatestHandoff(ctx, id)
	if err != nil {
		return model.Checkpoint{}, dbError(err)
	}
	sessionID, err := sessionPointer(row.SessionID)
	if err != nil {
		return model.Checkpoint{}, err
	}
	if err := s.auditRead(ctx, actor, "handoff.read", "task", taskID); err != nil {
		return model.Checkpoint{}, err
	}
	return model.Checkpoint{ID: row.ID, TaskID: row.TaskID,
		SessionID:   sessionID,
		TaskVersion: row.TaskVersion, Kind: row.Kind, Summary: row.Summary,
		Completed: row.Completed, Remaining: row.Remaining, Warnings: row.Warnings,
		ChangedPaths: row.ChangedPaths, TestResults: row.TestResults,
		SourceRevision: textPointer(row.SourceRevision), CreatedByPrincipal: row.CreatedByPrincipal,
		CreatedByClient: row.CreatedByClient, CreatedAt: row.CreatedAt.Time}, nil
}

func (s *Store) auditRead(ctx context.Context, actor model.Actor, action, resourceType, resourceID string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO audit_events
		(principal_id, client_id, action, resource_type, resource_id, result, request_id)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::uuid, 'success', $6)`,
		actor.PrincipalID, actor.ClientID, action, resourceType, resourceID, requestid.From(ctx))
	return err
}

func (s *Store) AuditFailure(ctx context.Context, actor model.Actor, action string, status int) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO audit_events
		(principal_id, client_id, action, resource_type, result, request_id)
		VALUES ($1::uuid, $2::uuid, $3, 'http_request', $4, $5)`,
		actor.PrincipalID, actor.ClientID, action, fmt.Sprintf("http_%d", status), requestid.From(ctx))
	return err
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return pgtype.UUID{}, model.ErrInvalid
	}
	return id, nil
}

func mustUUID(value string) pgtype.UUID {
	id, _ := parseUUID(value)
	return id
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nonemptyPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func sessionPointer(value any) (*string, error) {
	switch typed := value.(type) {
	case string:
		return nonemptyPointer(typed), nil
	case []byte:
		return nonemptyPointer(string(typed)), nil
	default:
		return nil, fmt.Errorf("unexpected session ID type %T", value)
	}
}

func dbError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return model.ErrConflict
		case "23503":
			return model.ErrNotFound
		}
	}
	return err
}

func auditWrite(ctx context.Context, tx pgx.Tx, actor model.Actor, action, resourceType, resourceID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_events
		(principal_id, client_id, action, resource_type, resource_id, result, request_id)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::uuid, 'success', $6)`,
		actor.PrincipalID, actor.ClientID, action, resourceType, resourceID, requestid.From(ctx))
	return err
}
