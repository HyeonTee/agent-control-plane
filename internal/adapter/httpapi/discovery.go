package httpapi

import (
	"net/http"
	"strconv"

	appwork "github.com/HyeonTee/agent-control-plane/internal/application/work"
	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
)

func registerDiscoveryRoutes(mux *http.ServeMux, service *appwork.Service, withActor func(actorRoute) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/tasks", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		limit, cursor, status, ok := pageParams(w, r, true)
		if !ok {
			return
		}
		page, err := service.ListActiveTasks(r.Context(), actor, status, limit, cursor)
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	}))
	mux.HandleFunc("GET /api/v1/tasks/{task_id}", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		overview, err := service.TaskOverview(r.Context(), actor, r.PathValue("task_id"))
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, overview)
	}))
	mux.HandleFunc("GET /api/v1/tasks/{task_id}/sessions", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		limit, cursor, _, ok := pageParams(w, r, false)
		if !ok {
			return
		}
		page, err := service.ListTaskSessions(r.Context(), actor, r.PathValue("task_id"), limit, cursor)
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	}))
	mux.HandleFunc("POST /api/v1/tasks/{task_id}/sessions", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		var body struct {
			ClientLabel            *string `json:"client_label"`
			SourceSessionReference *string `json:"source_session_reference"`
			ResumedFromHandoffID   *string `json:"resumed_from_handoff_id"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		session, err := service.CreateWorkSession(r.Context(), actor, model.CreateSessionInput{
			TaskID: r.PathValue("task_id"), ClientLabel: body.ClientLabel,
			SourceSessionReference: body.SourceSessionReference,
			ResumedFromHandoffID:   body.ResumedFromHandoffID,
		})
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, session)
	}))
	mux.HandleFunc("GET /api/v1/tasks/{task_id}/sessions/{session_id}", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		detail, err := service.SessionDetail(r.Context(), actor, r.PathValue("task_id"), r.PathValue("session_id"))
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	}))
	mux.HandleFunc("GET /api/v1/tasks/{task_id}/sessions/{session_id}/entries", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		limit, cursor, _, ok := pageParams(w, r, false)
		if !ok {
			return
		}
		page, err := service.SessionEntries(r.Context(), actor, r.PathValue("task_id"), r.PathValue("session_id"), limit, cursor)
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	}))
	mux.HandleFunc("POST /api/v1/tasks/{task_id}/sessions/{session_id}/close", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		var body struct {
			Summary *string `json:"summary"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		session, err := service.CloseWorkSession(r.Context(), actor, model.CloseSessionInput{
			TaskID: r.PathValue("task_id"), SessionID: r.PathValue("session_id"), Summary: body.Summary,
		})
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, session)
	}))
	mux.HandleFunc("GET /api/v1/tasks/{task_id}/pre-session-history", withActor(func(w http.ResponseWriter, r *http.Request, actor model.Actor) {
		limit, cursor, _, ok := pageParams(w, r, false)
		if !ok {
			return
		}
		page, err := service.PreSessionHistory(r.Context(), actor, r.PathValue("task_id"), limit, cursor)
		if err != nil {
			domainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	}))
}

func pageParams(w http.ResponseWriter, r *http.Request, allowStatus bool) (int, string, string, bool) {
	query := r.URL.Query()
	for key, values := range query {
		if len(values) != 1 || (key != "limit" && key != "cursor" && (!allowStatus || key != "status")) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", "invalid query parameters")
			return 0, "", "", false
		}
	}
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 50 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_input", "limit must be between 1 and 50")
			return 0, "", "", false
		}
		limit = value
	}
	return limit, query.Get("cursor"), query.Get("status"), true
}
