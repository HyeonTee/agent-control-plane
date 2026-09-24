package work

import (
	"context"
	"strings"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
)

func (s *Service) ListActiveTasks(ctx context.Context, actor model.Actor, status string, limit int, cursor string) (model.Page[model.TaskCard], error) {
	if !actor.Scopes[model.ScopeRead] {
		return model.Page[model.TaskCard]{}, model.ErrForbidden
	}
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "all", "todo", "in_progress", "blocked", "completed", "cancelled":
	default:
		return model.Page[model.TaskCard]{}, model.ErrInvalid
	}
	if !validPage(limit, cursor) {
		return model.Page[model.TaskCard]{}, model.ErrInvalid
	}
	return s.store.ListActiveTasks(ctx, actor, status, limit, cursor)
}

func (s *Service) TaskOverview(ctx context.Context, actor model.Actor, taskID string) (model.TaskOverview, error) {
	if _, err := s.requireTask(ctx, actor, taskID, model.ScopeRead); err != nil {
		return model.TaskOverview{}, err
	}
	return s.store.TaskOverview(ctx, actor, taskID)
}

func (s *Service) ListTaskSessions(ctx context.Context, actor model.Actor, taskID string, limit int, cursor string) (model.Page[model.WorkSession], error) {
	if _, err := s.requireTask(ctx, actor, taskID, model.ScopeRead); err != nil {
		return model.Page[model.WorkSession]{}, err
	}
	if !validPage(limit, cursor) {
		return model.Page[model.WorkSession]{}, model.ErrInvalid
	}
	return s.store.ListTaskSessions(ctx, actor, taskID, limit, cursor)
}

func (s *Service) SessionDetail(ctx context.Context, actor model.Actor, taskID, sessionID string) (model.SessionDetail, error) {
	if _, err := s.requireTask(ctx, actor, taskID, model.ScopeRead); err != nil {
		return model.SessionDetail{}, err
	}
	return s.store.SessionDetail(ctx, actor, taskID, sessionID)
}

func (s *Service) SessionEntries(ctx context.Context, actor model.Actor, taskID, sessionID string, limit int, cursor string) (model.Page[model.SessionEntry], error) {
	if _, err := s.requireTask(ctx, actor, taskID, model.ScopeRead); err != nil {
		return model.Page[model.SessionEntry]{}, err
	}
	if !validPage(limit, cursor) {
		return model.Page[model.SessionEntry]{}, model.ErrInvalid
	}
	return s.store.SessionEntries(ctx, actor, taskID, sessionID, limit, cursor)
}

func (s *Service) PreSessionHistory(ctx context.Context, actor model.Actor, taskID string, limit int, cursor string) (model.Page[model.Checkpoint], error) {
	if _, err := s.requireTask(ctx, actor, taskID, model.ScopeRead); err != nil {
		return model.Page[model.Checkpoint]{}, err
	}
	if !validPage(limit, cursor) {
		return model.Page[model.Checkpoint]{}, model.ErrInvalid
	}
	return s.store.PreSessionHistory(ctx, actor, taskID, limit, cursor)
}

func (s *Service) CreateWorkSession(ctx context.Context, actor model.Actor, in model.CreateSessionInput) (model.WorkSession, error) {
	if !validOptional(in.ClientLabel, 120) || !validOptional(in.SourceSessionReference, 200) {
		return model.WorkSession{}, model.ErrInvalid
	}
	if in.ClientLabel != nil && strings.TrimSpace(*in.ClientLabel) == "" {
		return model.WorkSession{}, model.ErrInvalid
	}
	if in.SourceSessionReference != nil && strings.TrimSpace(*in.SourceSessionReference) == "" {
		return model.WorkSession{}, model.ErrInvalid
	}
	if _, err := s.requireTask(ctx, actor, in.TaskID, model.ScopeWrite); err != nil {
		return model.WorkSession{}, err
	}
	return s.store.CreateWorkSession(ctx, actor, in)
}

func (s *Service) CloseWorkSession(ctx context.Context, actor model.Actor, in model.CloseSessionInput) (model.WorkSession, error) {
	in.Summary = strings.TrimSpace(in.Summary)
	if !validText(in.Summary, 4000) {
		return model.WorkSession{}, model.ErrInvalid
	}
	if _, err := s.requireTask(ctx, actor, in.TaskID, model.ScopeWrite); err != nil {
		return model.WorkSession{}, err
	}
	return s.store.CloseWorkSession(ctx, actor, in)
}

func validPage(limit int, cursor string) bool {
	return limit >= 1 && limit <= 50 && len(cursor) <= 512
}
