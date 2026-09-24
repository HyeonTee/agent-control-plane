package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HyeonTee/agent-control-plane/internal/adapter/httpapi"
	"github.com/HyeonTee/agent-control-plane/internal/adapter/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkContinuityAcrossTokens(t *testing.T) {
	url := os.Getenv("HUB_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("HUB_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("work_test_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
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
	config, err := pgxpool.ParseConfig(url)
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
	owner, err := postgres.BootstrapOwner(ctx, pool, "personal", "first-agent")
	if err != nil {
		t.Fatalf("bootstrap owner in dedicated test database: %v", err)
	}
	server := httptest.NewServer(httpapi.NewHandler(pool, postgres.NewStore(pool)))
	defer server.Close()

	request := func(method, path, token, key string, body any, want int) map[string]any {
		t.Helper()
		var payload *bytes.Reader
		if body == nil {
			payload = bytes.NewReader(nil)
		} else {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			payload = bytes.NewReader(encoded)
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, payload)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
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

	ownerToken := owner.Secret
	spaceID := owner.SpaceID
	request(http.MethodPost, "/api/v1/spaces/"+spaceID+"/projects", ownerToken, "",
		map[string]any{"name": strings.Repeat("x", 70<<10), "sync_policy": "metadata_and_handoffs"}, http.StatusRequestEntityTooLarge)
	project := request(http.MethodPost, "/api/v1/spaces/"+spaceID+"/projects", ownerToken, "",
		map[string]any{"name": "the project", "sync_policy": "metadata_and_handoffs"}, http.StatusCreated)
	projectID := project["id"].(string)
	task := request(http.MethodPost, "/api/v1/projects/"+projectID+"/tasks", ownerToken, "",
		map[string]any{"title": "first task", "objective": "resume with another token"}, http.StatusCreated)
	taskID := task["id"].(string)
	handoff := map[string]any{
		"expected_version": 1, "kind": "handoff", "summary": "first agent stopped",
		"completed": []string{"task created"}, "remaining": []string{"continue work"},
	}
	path := "/api/v1/tasks/" + taskID + "/checkpoints"
	first := request(http.MethodPost, path, ownerToken, "same-request-key", handoff, http.StatusCreated)
	retry := request(http.MethodPost, path, ownerToken, "same-request-key", handoff, http.StatusCreated)
	if first["id"] != retry["id"] {
		t.Fatalf("idempotent retry produced a new checkpoint: %v vs %v", first["id"], retry["id"])
	}
	changed := map[string]any{
		"expected_version": 1, "kind": "handoff", "summary": "changed payload",
		"remaining": []string{"continue work"},
	}
	request(http.MethodPost, path, ownerToken, "same-request-key", changed, http.StatusConflict)
	request(http.MethodPost, path, ownerToken, "new-request-key", handoff, http.StatusConflict)

	reader, err := postgres.CreateToken(ctx, pool, spaceID, "second-agent", []string{"context:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	read := request(http.MethodGet, "/api/v1/tasks/"+taskID+"/handoff", reader.Secret, "", nil, http.StatusOK)
	if read["id"] != first["id"] {
		t.Fatalf("second token read a different handoff: %v", read)
	}
	request(http.MethodPost, "/api/v1/projects/"+projectID+"/tasks", reader.Secret, "",
		map[string]any{"title": "denied", "objective": "read-only"}, http.StatusForbidden)
	request(http.MethodGet, "/api/v1/tasks/"+taskID+"/timeline", "acp_invalid", "", nil, http.StatusUnauthorized)

	var otherSpaceID string
	if err := pool.QueryRow(ctx, "INSERT INTO spaces (name) VALUES ('other-space') RETURNING id::text").Scan(&otherSpaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO principal_spaces (principal_id, space_id) VALUES ($1::uuid, $2::uuid)", owner.PrincipalID, otherSpaceID); err != nil {
		t.Fatal(err)
	}
	otherToken, err := postgres.CreateToken(ctx, pool, otherSpaceID, "other-space-agent", []string{"context:read", "work:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, "/api/v1/tasks/"+taskID+"/handoff", otherToken.Secret, "", nil, http.StatusNotFound)
	request(http.MethodPost, path, otherToken.Secret, "foreign-space-key", handoff, http.StatusNotFound)
	request(http.MethodGet, "/api/v1/projects/"+projectID+"/tasks", otherToken.Secret, "", nil, http.StatusNotFound)

	if err := postgres.RevokeToken(ctx, pool, reader.ClientID); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, "/api/v1/tasks/"+taskID+"/handoff", reader.Secret, "", nil, http.StatusUnauthorized)
	var checkpointCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM checkpoints WHERE task_id = $1::uuid", taskID).Scan(&checkpointCount); err != nil {
		t.Fatal(err)
	}
	if checkpointCount != 1 {
		t.Fatalf("expected one checkpoint, got %d", checkpointCount)
	}
	var handoffReads int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action = 'handoff.read' AND client_id = $1::uuid", reader.ClientID).Scan(&handoffReads); err != nil {
		t.Fatal(err)
	}
	if handoffReads != 1 {
		t.Fatalf("expected one audited handoff read, got %d", handoffReads)
	}
}
