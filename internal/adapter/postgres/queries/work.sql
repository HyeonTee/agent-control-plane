-- name: GetTokenByDigest :one
SELECT id::text AS id, principal_id::text AS principal_id, scopes
FROM client_tokens
WHERE secret_digest = sqlc.arg(secret_digest)
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: GetTokenSpaces :many
SELECT s.id::text AS id, s.name
FROM client_token_spaces cts
JOIN client_tokens ct ON ct.id = cts.token_id
JOIN principal_spaces ps ON ps.principal_id = ct.principal_id AND ps.space_id = cts.space_id
JOIN spaces s ON s.id = cts.space_id
WHERE cts.token_id = sqlc.arg(token_id)::uuid
ORDER BY s.name;

-- name: TouchToken :exec
UPDATE client_tokens SET last_used_at = now()
WHERE id = sqlc.arg(token_id)::uuid
  AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes');

-- name: GetProjectSpace :one
SELECT space_id::text AS space_id, sync_policy
FROM projects WHERE id = sqlc.arg(project_id)::uuid;

-- name: GetTaskSpace :one
SELECT p.space_id::text AS space_id, p.sync_policy
FROM tasks t JOIN projects p ON p.id = t.project_id
WHERE t.id = sqlc.arg(task_id)::uuid;

-- name: CreateProject :one
INSERT INTO projects (space_id, name, description, repository_reference, sync_policy)
VALUES (sqlc.arg(space_id)::uuid, sqlc.arg(name), sqlc.arg(description), sqlc.narg(repository_reference), sqlc.arg(sync_policy))
RETURNING id::text AS id, space_id::text AS space_id, name, description,
    repository_reference, sync_policy, created_at, updated_at;

-- name: CreateTask :one
INSERT INTO tasks (project_id, title, objective, status, base_revision)
VALUES (sqlc.arg(project_id)::uuid, sqlc.arg(title), sqlc.arg(objective), 'todo', sqlc.narg(base_revision))
RETURNING id::text AS id, project_id::text AS project_id, title, objective,
    status, version, base_revision, created_at, updated_at;

-- name: ListTasks :many
SELECT id::text AS id, project_id::text AS project_id, title, objective,
    status, version, base_revision, created_at, updated_at
FROM tasks WHERE project_id = sqlc.arg(project_id)::uuid
ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: TaskTimeline :many
SELECT id::text AS id, task_id::text AS task_id, task_version, kind, summary,
    completed, remaining, warnings, changed_paths, test_results,
    source_revision, created_by_principal::text AS created_by_principal,
    created_by_client::text AS created_by_client, created_at
FROM checkpoints WHERE task_id = sqlc.arg(task_id)::uuid
ORDER BY task_version DESC LIMIT 50;

-- name: LatestHandoff :one
SELECT id::text AS id, task_id::text AS task_id, task_version, kind, summary,
    completed, remaining, warnings, changed_paths, test_results,
    source_revision, created_by_principal::text AS created_by_principal,
    created_by_client::text AS created_by_client, created_at
FROM checkpoints WHERE task_id = sqlc.arg(task_id)::uuid AND kind = 'handoff'
ORDER BY task_version DESC LIMIT 1;
