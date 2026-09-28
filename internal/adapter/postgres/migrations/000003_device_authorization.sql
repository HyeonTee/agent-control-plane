CREATE TABLE device_authorizations (
    id uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    device_code_digest bytea PRIMARY KEY,
    user_code_digest bytea NOT NULL UNIQUE,
    client_label text NOT NULL CHECK (char_length(client_label) BETWEEN 1 AND 120),
    scopes text[] NOT NULL CHECK (array_length(scopes, 1) IS NOT NULL),
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'denied', 'consumed')),
    principal_id uuid REFERENCES principals(id),
    space_id uuid REFERENCES spaces(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_polled_at timestamptz,
    CHECK ((status = 'approved' AND principal_id IS NOT NULL AND space_id IS NOT NULL)
        OR status <> 'approved')
);
CREATE INDEX device_authorizations_expires_idx ON device_authorizations(expires_at);

CREATE TABLE device_refresh_tokens (
    token_id uuid PRIMARY KEY REFERENCES client_tokens(id),
    secret_digest bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE TABLE device_refresh_used (
    secret_digest bytea PRIMARY KEY,
    token_id uuid NOT NULL REFERENCES client_tokens(id),
    used_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX device_refresh_used_token_idx ON device_refresh_used(token_id);
