package work

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalid      = errors.New("invalid input")
)

const (
	ScopeRead  = "context:read"
	ScopeWrite = "work:write"
	PolicyHub  = "metadata_and_handoffs"
)

type Actor struct {
	PrincipalID string
	ClientID    string
	Scopes      map[string]bool
	SpaceIDs    map[string]bool
}

func (a Actor) Can(scope, spaceID string) bool {
	return a.Scopes[scope] && a.SpaceIDs[spaceID]
}

type Space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Project struct {
	ID                  string    `json:"id"`
	SpaceID             string    `json:"space_id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	RepositoryReference *string   `json:"repository_reference,omitempty"`
	SyncPolicy          string    `json:"sync_policy"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Task struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Title        string    `json:"title"`
	Objective    string    `json:"objective"`
	Status       string    `json:"status"`
	Version      int64     `json:"version"`
	BaseRevision *string   `json:"base_revision,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Checkpoint struct {
	ID                 string    `json:"id"`
	TaskID             string    `json:"task_id"`
	SessionID          *string   `json:"session_id,omitempty"`
	TaskVersion        int64     `json:"task_version"`
	Kind               string    `json:"kind"`
	Summary            string    `json:"summary"`
	Completed          []string  `json:"completed"`
	Remaining          []string  `json:"remaining"`
	Warnings           []string  `json:"warnings"`
	ChangedPaths       []string  `json:"changed_paths"`
	TestResults        []string  `json:"test_results"`
	SourceRevision     *string   `json:"source_revision,omitempty"`
	CreatedByPrincipal string    `json:"created_by_principal"`
	CreatedByClient    string    `json:"created_by_client"`
	CreatedAt          time.Time `json:"created_at"`
}

type CreateProjectInput struct {
	SpaceID             string
	Name                string
	Description         string
	RepositoryReference *string
	SyncPolicy          string
}

type CreateTaskInput struct {
	ProjectID    string
	Title        string
	Objective    string
	BaseRevision *string
}

type AppendCheckpointInput struct {
	TaskID          string
	SessionID       *string
	ExpectedVersion int64
	IdempotencyKey  string
	Kind            string
	Summary         string
	Completed       []string
	Remaining       []string
	Warnings        []string
	ChangedPaths    []string
	TestResults     []string
	SourceRevision  *string
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type TaskCard struct {
	ID                   string    `json:"id"`
	ProjectID            string    `json:"project_id"`
	ProjectName          string    `json:"project_name"`
	SpaceID              string    `json:"space_id"`
	SpaceName            string    `json:"space_name"`
	Title                string    `json:"title"`
	Status               string    `json:"status"`
	Version              int64     `json:"version"`
	LatestSummary        *string   `json:"latest_summary,omitempty"`
	LastActivityAt       time.Time `json:"last_activity_at"`
	SessionCount         int64     `json:"session_count"`
	HasPreSessionHistory bool      `json:"has_pre_session_history"`
}

type ProjectRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SpaceID   string `json:"space_id"`
	SpaceName string `json:"space_name"`
}

type TaskOverview struct {
	Task                 Task              `json:"task"`
	Project              ProjectRef        `json:"project"`
	LatestCheckpoint     *Checkpoint       `json:"latest_checkpoint,omitempty"`
	LatestHandoff        *Checkpoint       `json:"latest_handoff,omitempty"`
	RecentCheckpoints    []Checkpoint      `json:"recent_checkpoints"`
	Sessions             Page[WorkSession] `json:"sessions"`
	PreSessionHistory    Page[Checkpoint]  `json:"pre_session_history"`
	HasPreSessionHistory bool              `json:"has_pre_session_history"`
}

type WorkSession struct {
	ID                     string     `json:"id"`
	TaskID                 string     `json:"task_id"`
	CreatedByPrincipal     string     `json:"created_by_principal"`
	CreatedByClient        string     `json:"created_by_client"`
	ClientLabel            *string    `json:"client_label,omitempty"`
	SourceSessionReference *string    `json:"source_session_reference,omitempty"`
	ResumedFromHandoffID   *string    `json:"resumed_from_handoff_id,omitempty"`
	StartedAt              time.Time  `json:"started_at"`
	EndedAt                *time.Time `json:"ended_at,omitempty"`
	LastActivityAt         time.Time  `json:"last_activity_at"`
	Summary                *string    `json:"summary,omitempty"`
	EntryCount             int64      `json:"entry_count"`
}

type SessionEntry struct {
	Sequence   int64      `json:"sequence"`
	Checkpoint Checkpoint `json:"checkpoint"`
}

type SessionDetail struct {
	Session WorkSession        `json:"session"`
	Entries Page[SessionEntry] `json:"entries"`
}

type CreateSessionInput struct {
	TaskID                 string
	ClientLabel            *string
	SourceSessionReference *string
	ResumedFromHandoffID   *string
}

type CloseSessionInput struct {
	TaskID    string
	SessionID string
	Summary   *string
}
