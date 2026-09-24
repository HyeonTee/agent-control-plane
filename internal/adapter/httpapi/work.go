package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	appwork "github.com/HyeonTee/agent-control-plane/internal/application/work"
	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type actorRoute func(http.ResponseWriter, *http.Request, model.Actor)

func registerWorkRoutes(mux *http.ServeMux, backend Backend) {
	service := appwork.New(backend)
	withActor := func(next actorRoute) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			authorizations := r.Header.Values("Authorization")
			if len(authorizations) != 1 || !strings.HasPrefix(authorizations[0], "Bearer ") {
				unauthorized(w)
				return
			}
			actor, err := backend.Authenticate(r.Context(), strings.TrimPrefix(authorizations[0], "Bearer "))
			if err != nil {
				if errors.Is(err, model.ErrUnauthorized) {
					slog.Warn("authentication rejected", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
					unauthorized(w)
				} else {
					slog.Error("authentication failed", "error", err)
					writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
				}
				return
			}
			next(w, r, actor)
		}
	}
	mux.HandleFunc("GET /api/v1/spaces", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		spaces, err := service.ListSpaces(r.Context(), actor)
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": spaces})
	}))
	mux.HandleFunc("POST /api/v1/spaces/{space_id}/projects", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		var body struct {
			Name                string  `json:"name"`
			Description         string  `json:"description"`
			RepositoryReference *string `json:"repository_reference"`
			SyncPolicy          string  `json:"sync_policy"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		project, err := service.CreateProject(r.Context(), actor, model.CreateProjectInput{
			SpaceID: r.PathValue("space_id"), Name: body.Name, Description: body.Description,
			RepositoryReference: body.RepositoryReference, SyncPolicy: body.SyncPolicy,
		})
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, project)
	}))
	mux.HandleFunc("POST /api/v1/projects/{project_id}/tasks", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		var body struct {
			Title        string  `json:"title"`
			Objective    string  `json:"objective"`
			BaseRevision *string `json:"base_revision"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		task, err := service.CreateTask(r.Context(), actor, model.CreateTaskInput{
			ProjectID: r.PathValue("project_id"), Title: body.Title,
			Objective: body.Objective, BaseRevision: body.BaseRevision,
		})
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, task)
	}))
	mux.HandleFunc("GET /api/v1/projects/{project_id}/tasks", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		tasks, err := service.ListTasks(r.Context(), actor, r.PathValue("project_id"))
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": tasks})
	}))
	mux.HandleFunc("POST /api/v1/tasks/{task_id}/checkpoints", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		var body struct {
			SessionID       *string  `json:"session_id"`
			ExpectedVersion int64    `json:"expected_version"`
			Kind            string   `json:"kind"`
			Summary         string   `json:"summary"`
			Completed       []string `json:"completed"`
			Remaining       []string `json:"remaining"`
			Warnings        []string `json:"warnings"`
			ChangedPaths    []string `json:"changed_paths"`
			TestResults     []string `json:"test_results"`
			SourceRevision  *string  `json:"source_revision"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", "Idempotency-Key is required")
			return
		}
		checkpoint, err := service.AppendCheckpoint(r.Context(), actor, model.AppendCheckpointInput{
			TaskID: r.PathValue("task_id"), SessionID: body.SessionID, ExpectedVersion: body.ExpectedVersion,
			IdempotencyKey: keys[0], Kind: body.Kind, Summary: body.Summary,
			Completed: body.Completed, Remaining: body.Remaining, Warnings: body.Warnings,
			ChangedPaths: body.ChangedPaths, TestResults: body.TestResults,
			SourceRevision: body.SourceRevision,
		})
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, checkpoint)
	}))
	mux.HandleFunc("GET /api/v1/tasks/{task_id}/timeline", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		items, err := service.TaskTimeline(r.Context(), actor, r.PathValue("task_id"))
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}))
	mux.HandleFunc("GET /api/v1/tasks/{task_id}/handoff", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		item, err := service.LatestHandoff(r.Context(), actor, r.PathValue("task_id"))
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}))
	registerDiscoveryRoutes(mux, service, withActor)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "application/json is required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "JSON body exceeds 64 KiB")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "JSON body exceeds 64 KiB")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "one JSON object is required")
		return false
	}
	return true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthorized", "valid bearer token required")
}

func domainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "operation not allowed")
	case errors.Is(err, model.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, model.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource changed or request key was reused")
	case errors.Is(err, model.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_input", "invalid input")
	default:
		slog.Error("API request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
