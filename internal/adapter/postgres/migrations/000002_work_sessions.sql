CREATE TABLE work_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL REFERENCES tasks(id),
    created_by_principal uuid NOT NULL REFERENCES principals(id),
    created_by_client uuid NOT NULL REFERENCES client_tokens(id),
    client_label text CHECK (client_label IS NULL OR char_length(client_label) <= 120),
    source_session_reference text CHECK (source_session_reference IS NULL OR char_length(source_session_reference) <= 200),
    resumed_from_handoff_id uuid REFERENCES checkpoints(id),
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    last_activity_at timestamptz NOT NULL DEFAULT now(),
    summary text CHECK (summary IS NULL OR char_length(summary) <= 4000),
    entry_count bigint NOT NULL DEFAULT 0 CHECK (entry_count >= 0)
);
CREATE INDEX work_sessions_task_activity_idx ON work_sessions(task_id, last_activity_at DESC, id DESC);

ALTER TABLE checkpoints ADD COLUMN session_id uuid REFERENCES work_sessions(id);
CREATE INDEX checkpoints_session_id_idx ON checkpoints(session_id, task_version DESC)
    WHERE session_id IS NOT NULL;

CREATE TABLE session_entries (
    session_id uuid NOT NULL REFERENCES work_sessions(id),
    sequence bigint NOT NULL CHECK (sequence > 0),
    checkpoint_id uuid NOT NULL UNIQUE REFERENCES checkpoints(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, sequence)
);

CREATE INDEX tasks_activity_idx ON tasks(status, updated_at DESC, id DESC);
