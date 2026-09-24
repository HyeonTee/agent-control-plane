CREATE TABLE principals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE spaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    default_sync_policy text NOT NULL DEFAULT 'metadata_and_handoffs'
        CHECK (default_sync_policy IN ('metadata_and_handoffs', 'local_only')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE principal_spaces (
    principal_id uuid NOT NULL REFERENCES principals(id),
    space_id uuid NOT NULL REFERENCES spaces(id),
    PRIMARY KEY (principal_id, space_id)
);

CREATE TABLE client_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL REFERENCES principals(id),
    display_name text NOT NULL,
    secret_digest bytea NOT NULL UNIQUE,
    scopes text[] NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz,
    CHECK (array_length(scopes, 1) IS NOT NULL)
);

CREATE TABLE client_token_spaces (
    token_id uuid NOT NULL REFERENCES client_tokens(id),
    space_id uuid NOT NULL REFERENCES spaces(id),
    PRIMARY KEY (token_id, space_id)
);

CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id uuid NOT NULL REFERENCES spaces(id),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    repository_reference text,
    sync_policy text NOT NULL CHECK (sync_policy IN ('metadata_and_handoffs', 'local_only')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (space_id, name)
);
CREATE INDEX projects_space_id_idx ON projects(space_id);

CREATE TABLE tasks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    title text NOT NULL,
    objective text NOT NULL,
    status text NOT NULL DEFAULT 'todo'
        CHECK (status IN ('todo', 'in_progress', 'blocked', 'completed', 'cancelled')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    base_revision text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_project_id_idx ON tasks(project_id, created_at DESC);

CREATE TABLE checkpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES tasks(id),
    task_version bigint NOT NULL,
    kind text NOT NULL CHECK (kind IN ('progress', 'handoff')),
    summary text NOT NULL,
    completed text[] NOT NULL DEFAULT '{}',
    remaining text[] NOT NULL DEFAULT '{}',
    warnings text[] NOT NULL DEFAULT '{}',
    changed_paths text[] NOT NULL DEFAULT '{}',
    test_results text[] NOT NULL DEFAULT '{}',
    source_revision text,
    created_by_principal uuid NOT NULL REFERENCES principals(id),
    created_by_client uuid NOT NULL REFERENCES client_tokens(id),
    idempotency_key text NOT NULL,
    request_digest bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (created_by_client, idempotency_key),
    UNIQUE (task_id, task_version)
);
CREATE INDEX checkpoints_task_timeline_idx ON checkpoints(task_id, task_version DESC);
CREATE INDEX checkpoints_task_handoff_idx ON checkpoints(task_id, task_version DESC) WHERE kind = 'handoff';

CREATE TABLE audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    principal_id uuid REFERENCES principals(id),
    client_id uuid REFERENCES client_tokens(id),
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    result text NOT NULL,
    request_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);
