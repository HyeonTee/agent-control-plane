package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/HyeonTee/agent-control-plane/internal/adapter/httpapi"
	"github.com/HyeonTee/agent-control-plane/internal/adapter/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDiscoverWorkAndSessionHistory(t *testing.T) {
	databaseURL := os.Getenv("HUB_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUB_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("discovery_test_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer adminPool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	owner, err := postgres.BootstrapOwner(ctx, pool, "personal", "writer")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.NewHandler(pool, postgres.NewStore(pool)))
	defer server.Close()
	request := func(method, path, token, key string, body any, want int) map[string]any {
		t.Helper()
		var payload []byte
		if body != nil {
			var err error
			payload, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result map[string]any
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: got %d, want %d: %v", method, path, response.StatusCode, want, result)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s omitted no-store", method, path)
		}
		return result
	}
	items := func(page map[string]any) []any {
		t.Helper()
		entries, ok := page["items"].([]any)
		if !ok {
			t.Fatalf("missing items: %v", page)
		}
		return entries
	}

	project := request(http.MethodPost, "/api/v1/spaces/"+owner.SpaceID+"/projects", owner.Secret, "",
		map[string]any{"name": "continuity", "sync_policy": "metadata_and_handoffs"}, http.StatusCreated)
	projectID := project["id"].(string)
	task := request(http.MethodPost, "/api/v1/projects/"+projectID+"/tasks", owner.Secret, "",
		map[string]any{"title": "discover me", "objective": "resume through a session index"}, http.StatusCreated)
	taskID := task["id"].(string)
	checkpointPath := "/api/v1/tasks/" + taskID + "/checkpoints"
	legacy := request(http.MethodPost, checkpointPath, owner.Secret, "legacy-entry-1",
		map[string]any{"expected_version": 1, "kind": "progress", "summary": "before sessions"}, http.StatusCreated)
	if _, hasSession := legacy["session_id"]; hasSession {
		t.Fatalf("legacy checkpoint unexpectedly has a session: %v", legacy)
	}
	listed := request(http.MethodGet, "/api/v1/tasks?status=in_progress", owner.Secret, "", nil, http.StatusOK)
	if len(items(listed)) != 1 || items(listed)[0].(map[string]any)["id"] != taskID {
		t.Fatalf("active task not discoverable: %v", listed)
	}
	if items(listed)[0].(map[string]any)["has_pre_session_history"] != true {
		t.Fatalf("legacy history not marked on task card: %v", listed)
	}
	overview := request(http.MethodGet, "/api/v1/tasks/"+taskID, owner.Secret, "", nil, http.StatusOK)
	if overview["latest_checkpoint"].(map[string]any)["id"] != legacy["id"] || len(items(overview["pre_session_history"].(map[string]any))) != 1 {
		t.Fatalf("overview lost legacy checkpoint: %v", overview)
	}
	request(http.MethodGet, "/api/v1/tasks/"+taskID+"/pre-session-history", owner.Secret, "", nil, http.StatusOK)

	sessionPath := "/api/v1/tasks/" + taskID + "/sessions"
	session := request(http.MethodPost, sessionPath, owner.Secret, "",
		map[string]any{"client_label": "codex"}, http.StatusCreated)
	sessionID := session["id"].(string)
	first := request(http.MethodPost, checkpointPath, owner.Secret, "session-entry-1",
		map[string]any{"session_id": sessionID, "expected_version": 2, "kind": "progress", "summary": "first session entry"}, http.StatusCreated)
	secondBody := map[string]any{"session_id": sessionID, "expected_version": 3, "kind": "handoff",
		"summary": "ready for the next session", "remaining": []string{"continue"}}
	second := request(http.MethodPost, checkpointPath, owner.Secret, "session-entry-2", secondBody, http.StatusCreated)
	detailPath := sessionPath + "/" + sessionID
	detail := request(http.MethodGet, detailPath, owner.Secret, "", nil, http.StatusOK)
	if detail["session"].(map[string]any)["entry_count"] != float64(2) || len(items(detail["entries"].(map[string]any))) != 2 {
		t.Fatalf("session detail omitted entries: %v", detail)
	}
	entryPage := request(http.MethodGet, detailPath+"/entries?limit=1", owner.Secret, "", nil, http.StatusOK)
	if len(items(entryPage)) != 1 || items(entryPage)[0].(map[string]any)["checkpoint"].(map[string]any)["id"] != first["id"] {
		t.Fatalf("first session entry missing: %v", entryPage)
	}
	next := entryPage["next_cursor"].(string)
	entryPage2 := request(http.MethodGet, detailPath+"/entries?limit=1&cursor="+url.QueryEscape(next), owner.Secret, "", nil, http.StatusOK)
	if len(items(entryPage2)) != 1 || items(entryPage2)[0].(map[string]any)["checkpoint"].(map[string]any)["id"] != second["id"] {
		t.Fatalf("second session entry missing: %v", entryPage2)
	}
	request(http.MethodGet, detailPath+"/entries?cursor=invalid", owner.Secret, "", nil, http.StatusUnprocessableEntity)
	closed := request(http.MethodPost, detailPath+"/close", owner.Secret, "",
		map[string]any{"summary": "two entries saved"}, http.StatusOK)
	if closed["ended_at"] == nil || closed["summary"] != "two entries saved" {
		t.Fatalf("session did not close: %v", closed)
	}
	request(http.MethodPost, checkpointPath, owner.Secret, "session-entry-3",
		map[string]any{"session_id": sessionID, "expected_version": 4, "kind": "progress", "summary": "too late"}, http.StatusConflict)
	retry := request(http.MethodPost, checkpointPath, owner.Secret, "session-entry-2", secondBody, http.StatusCreated)
	if retry["id"] != second["id"] {
		t.Fatalf("exact retry changed checkpoint: %v", retry)
	}
	request(http.MethodPost, sessionPath, owner.Secret, "",
		map[string]any{"client_label": "next agent", "resumed_from_handoff_id": second["id"]}, http.StatusCreated)
	sessionsPage := request(http.MethodGet, sessionPath+"?limit=1", owner.Secret, "", nil, http.StatusOK)
	if len(items(sessionsPage)) != 1 || sessionsPage["next_cursor"] == nil {
		t.Fatalf("session index did not paginate: %v", sessionsPage)
	}
	sessionsPage2 := request(http.MethodGet, sessionPath+"?limit=1&cursor="+url.QueryEscape(sessionsPage["next_cursor"].(string)), owner.Secret, "", nil, http.StatusOK)
	if len(items(sessionsPage2)) != 1 || items(sessionsPage2)[0].(map[string]any)["id"] == items(sessionsPage)[0].(map[string]any)["id"] {
		t.Fatalf("session cursor repeated a session: %v", sessionsPage2)
	}

	secondTask := request(http.MethodPost, "/api/v1/projects/"+projectID+"/tasks", owner.Secret, "",
		map[string]any{"title": "second task", "objective": "test task pagination"}, http.StatusCreated)
	secondTaskPath := "/api/v1/tasks/" + secondTask["id"].(string)
	briefSession := request(http.MethodPost, secondTaskPath+"/sessions", owner.Secret, "", map[string]any{}, http.StatusCreated)
	briefClose := request(http.MethodPost, secondTaskPath+"/sessions/"+briefSession["id"].(string)+"/close",
		owner.Secret, "", map[string]any{}, http.StatusOK)
	if _, hasSummary := briefClose["summary"]; hasSummary {
		t.Fatalf("optional session summary unexpectedly present: %v", briefClose)
	}
	secondOverview := request(http.MethodGet, secondTaskPath, owner.Secret, "", nil, http.StatusOK)
	if secondOverview["task"].(map[string]any)["status"] != "todo" || secondOverview["task"].(map[string]any)["version"] != float64(1) {
		t.Fatalf("starting a session changed task state or version: %v", secondOverview)
	}
	request(http.MethodPost, "/api/v1/tasks/"+secondTask["id"].(string)+"/checkpoints", owner.Secret, "wrong-task-session",
		map[string]any{"session_id": sessionID, "expected_version": 1, "kind": "progress", "summary": "wrong task"}, http.StatusNotFound)
	request(http.MethodPost, "/api/v1/tasks/"+secondTask["id"].(string)+"/checkpoints", owner.Secret, "second-task-1",
		map[string]any{"expected_version": 1, "kind": "progress", "summary": "also active"}, http.StatusCreated)
	tasksPage := request(http.MethodGet, "/api/v1/tasks?status=in_progress&limit=1", owner.Secret, "", nil, http.StatusOK)
	if len(items(tasksPage)) != 1 || tasksPage["next_cursor"] == nil {
		t.Fatalf("task discovery did not paginate: %v", tasksPage)
	}
	tasksPage2 := request(http.MethodGet, "/api/v1/tasks?status=in_progress&limit=1&cursor="+url.QueryEscape(tasksPage["next_cursor"].(string)), owner.Secret, "", nil, http.StatusOK)
	if len(items(tasksPage2)) != 1 || items(tasksPage2)[0].(map[string]any)["id"] == items(tasksPage)[0].(map[string]any)["id"] {
		t.Fatalf("task cursor repeated a task: %v", tasksPage2)
	}

	reader, err := postgres.CreateToken(ctx, pool, owner.SpaceID, "reader", []string{"context:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, "/api/v1/tasks/"+taskID, reader.Secret, "", nil, http.StatusOK)
	request(http.MethodGet, detailPath, reader.Secret, "", nil, http.StatusOK)
	request(http.MethodPost, sessionPath, reader.Secret, "", map[string]any{}, http.StatusForbidden)
	otherWriter, err := postgres.CreateToken(ctx, pool, owner.SpaceID, "other writer", []string{"context:read", "work:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, checkpointPath, otherWriter.Secret, "other-writer-entry",
		map[string]any{"session_id": sessionID, "expected_version": 4, "kind": "progress", "summary": "wrong client"}, http.StatusForbidden)
	request(http.MethodPost, detailPath+"/close", otherWriter.Secret, "",
		map[string]any{"summary": "wrong client"}, http.StatusForbidden)
	var otherSpaceID string
	if err := pool.QueryRow(ctx, "INSERT INTO spaces (name) VALUES ('other') RETURNING id::text").Scan(&otherSpaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO principal_spaces (principal_id, space_id) VALUES ($1::uuid, $2::uuid)", owner.PrincipalID, otherSpaceID); err != nil {
		t.Fatal(err)
	}
	foreign, err := postgres.CreateToken(ctx, pool, otherSpaceID, "foreign", []string{"context:read", "work:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(items(request(http.MethodGet, "/api/v1/tasks", foreign.Secret, "", nil, http.StatusOK))) != 0 {
		t.Fatal("foreign token discovered a task")
	}
	request(http.MethodGet, "/api/v1/tasks/"+taskID, foreign.Secret, "", nil, http.StatusNotFound)
	request(http.MethodGet, detailPath, foreign.Secret, "", nil, http.StatusNotFound)
	request(http.MethodPost, sessionPath, foreign.Secret, "", map[string]any{}, http.StatusNotFound)
}
