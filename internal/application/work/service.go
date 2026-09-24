package work

import (
	"context"
	"fmt"
	"strings"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
)

type Store interface {
	ListSpaces(context.Context, model.Actor) ([]model.Space, error)
	ProjectSpace(context.Context, string) (string, string, error)
	TaskSpace(context.Context, string) (string, string, error)
	CreateProject(context.Context, model.Actor, model.CreateProjectInput) (model.Project, error)
	CreateTask(context.Context, model.Actor, model.CreateTaskInput) (model.Task, error)
	ListTasks(context.Context, model.Actor, string) ([]model.Task, error)
	AppendCheckpoint(context.Context, model.Actor, model.AppendCheckpointInput) (model.Checkpoint, error)
	TaskTimeline(context.Context, model.Actor, string) ([]model.Checkpoint, error)
	LatestHandoff(context.Context, model.Actor, string) (model.Checkpoint, error)
	ListActiveTasks(context.Context, model.Actor, string, int, string) (model.Page[model.TaskCard], error)
	TaskOverview(context.Context, model.Actor, string) (model.TaskOverview, error)
	ListTaskSessions(context.Context, model.Actor, string, int, string) (model.Page[model.WorkSession], error)
	SessionDetail(context.Context, model.Actor, string, string) (model.SessionDetail, error)
	SessionEntries(context.Context, model.Actor, string, string, int, string) (model.Page[model.SessionEntry], error)
	PreSessionHistory(context.Context, model.Actor, string, int, string) (model.Page[model.Checkpoint], error)
	CreateWorkSession(context.Context, model.Actor, model.CreateSessionInput) (model.WorkSession, error)
	CloseWorkSession(context.Context, model.Actor, model.CloseSessionInput) (model.WorkSession, error)
}

type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

func (s *Service) ListSpaces(ctx context.Context, actor model.Actor) ([]model.Space, error) {
	if !actor.Scopes[model.ScopeRead] {
		return nil, model.ErrForbidden
	}
	return s.store.ListSpaces(ctx, actor)
}

func (s *Service) CreateProject(ctx context.Context, actor model.Actor, in model.CreateProjectInput) (model.Project, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.SyncPolicy != model.PolicyHub || !validText(in.Name, 120) || len(in.Description) > 4000 || !validOptional(in.RepositoryReference, 500) {
		return model.Project{}, model.ErrInvalid
	}
	if !actor.SpaceIDs[in.SpaceID] {
		return model.Project{}, model.ErrNotFound
	}
	if !actor.Scopes[model.ScopeWrite] {
		return model.Project{}, model.ErrForbidden
	}
	return s.store.CreateProject(ctx, actor, in)
}

func (s *Service) CreateTask(ctx context.Context, actor model.Actor, in model.CreateTaskInput) (model.Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Objective = strings.TrimSpace(in.Objective)
	if !validText(in.Title, 200) || !validText(in.Objective, 4000) || !validOptional(in.BaseRevision, 200) {
		return model.Task{}, model.ErrInvalid
	}
	_, err := s.requireProject(ctx, actor, in.ProjectID, model.ScopeWrite)
	if err != nil {
		return model.Task{}, err
	}
	return s.store.CreateTask(ctx, actor, in)
}

func (s *Service) ListTasks(ctx context.Context, actor model.Actor, projectID string) ([]model.Task, error) {
	if _, err := s.requireProject(ctx, actor, projectID, model.ScopeRead); err != nil {
		return nil, err
	}
	return s.store.ListTasks(ctx, actor, projectID)
}

func (s *Service) AppendCheckpoint(ctx context.Context, actor model.Actor, in model.AppendCheckpointInput) (model.Checkpoint, error) {
	in.Summary = strings.TrimSpace(in.Summary)
	if in.SessionID != nil && strings.TrimSpace(*in.SessionID) == "" {
		return model.Checkpoint{}, model.ErrInvalid
	}
	if in.ExpectedVersion < 1 || !validText(in.Summary, 4000) || (in.Kind != "progress" && in.Kind != "handoff") || len(in.IdempotencyKey) < 8 || len(in.IdempotencyKey) > 128 || !validOptional(in.SourceRevision, 200) {
		return model.Checkpoint{}, model.ErrInvalid
	}
	if !validItems(in.Completed, 20, 500) || !validItems(in.Remaining, 20, 500) || !validItems(in.Warnings, 20, 500) || !validItems(in.ChangedPaths, 50, 500) || !validItems(in.TestResults, 20, 500) {
		return model.Checkpoint{}, model.ErrInvalid
	}
	if in.Kind == "handoff" && len(in.Remaining) == 0 {
		return model.Checkpoint{}, fmt.Errorf("%w: handoff needs a next action", model.ErrInvalid)
	}
	if _, err := s.requireTask(ctx, actor, in.TaskID, model.ScopeWrite); err != nil {
		return model.Checkpoint{}, err
	}
	return s.store.AppendCheckpoint(ctx, actor, in)
}

func (s *Service) TaskTimeline(ctx context.Context, actor model.Actor, taskID string) ([]model.Checkpoint, error) {
	if _, err := s.requireTask(ctx, actor, taskID, model.ScopeRead); err != nil {
		return nil, err
	}
	return s.store.TaskTimeline(ctx, actor, taskID)
}

func (s *Service) LatestHandoff(ctx context.Context, actor model.Actor, taskID string) (model.Checkpoint, error) {
	if _, err := s.requireTask(ctx, actor, taskID, model.ScopeRead); err != nil {
		return model.Checkpoint{}, err
	}
	return s.store.LatestHandoff(ctx, actor, taskID)
}

func (s *Service) requireProject(ctx context.Context, actor model.Actor, projectID, scope string) (string, error) {
	spaceID, policy, err := s.store.ProjectSpace(ctx, projectID)
	if err != nil {
		return "", err
	}
	return require(actor, spaceID, policy, scope)
}

func (s *Service) requireTask(ctx context.Context, actor model.Actor, taskID, scope string) (string, error) {
	spaceID, policy, err := s.store.TaskSpace(ctx, taskID)
	if err != nil {
		return "", err
	}
	return require(actor, spaceID, policy, scope)
}

func require(actor model.Actor, spaceID, policy, scope string) (string, error) {
	if !actor.SpaceIDs[spaceID] || policy != model.PolicyHub {
		return "", model.ErrNotFound
	}
	if !actor.Scopes[scope] {
		return "", model.ErrForbidden
	}
	return spaceID, nil
}

func validText(s string, max int) bool { return s != "" && len(s) <= max }

func validOptional(s *string, max int) bool { return s == nil || len(*s) <= max }

func validItems(items []string, maxCount, maxLen int) bool {
	if len(items) > maxCount {
		return false
	}
	for _, item := range items {
		if !validText(strings.TrimSpace(item), maxLen) {
			return false
		}
	}
	return true
}
