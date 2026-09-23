# Security and data boundaries

## Threat model

The Hub is reachable from multiple computers and may hold context about unrelated personal and company work. The primary risks are:

- a leaked device token;
- accidental synchronization of prohibited company data;
- cross-space data disclosure;
- prompt injection being stored as a trusted instruction;
- malicious or malformed skill packages;
- stale observations being treated as current facts;
- loss or corruption of the central context store.

## No environment credentials

The Hub must not store or receive:

- cloud access keys or session credentials;
- kubeconfig files or Kubernetes tokens;
- Git hosting credentials;
- LLM provider credentials used by local agents;
- credentials copied from command output.

The Hub instance role grants access only to the Hub's own infrastructure.

## Sync policy

Every project has an explicit sync policy. The local client applies it before sending data, and the Hub validates the declared classification again.

Example:

```yaml
schema_version: 1
source_code: never
diffs: never
command_output: sanitized
task_metadata: allowed
handoffs: allowed
decisions: allowed
observations: metadata_only
retention_days: 30
```

Supported policies must include a `local_only` mode. Company policy and contractual obligations take precedence over the user's desire for continuity. Data that is not approved for personal infrastructure stays in the local repository or an organization-approved store.

## Authentication

The MVP uses user-created, device-specific opaque bearer tokens.

Each token has:

```text
id and display name
secret hash
allowed spaces
scopes
created_at
expires_at
last_used_at
revoked_at
```

Tokens are high-entropy random values, shown once, and stored locally in the operating-system credential store when available. The Hub stores only a hash. Tokens can be revoked independently without affecting other devices.

OAuth or passkeys may replace this flow later without changing the domain authorization model.

## Authorization

Authorization is based on the authenticated principal, not on a vendor name or agent profile.

```text
effective permission
  = principal scopes
  ∩ space access
  ∩ project sync policy
  ∩ requested agent-profile restrictions
```

An agent profile cannot expand permissions.

## Content trust

Stored content carries a trust level and provenance. Reference documents, repository content, and observations are treated as data, not instructions.

Only explicit rules and approved skill instructions may influence agent behavior as instructions. Bootstrap output preserves that distinction instead of concatenating all content into one prompt.

## Artifact safety

Skill and workflow packages are immutable and addressed by digest. The Hub verifies size, archive paths, declared manifest, and digest before publishing.

Downloading a package does not authorize execution. The local client displays requested capabilities and applies local approval policy before installing or running scripts.

## Transport and storage

- HTTPS is mandatory outside local development.
- PostgreSQL is not exposed publicly.
- Secrets are read from the Hub's own secret store at startup.
- S3 buckets block public access and use encryption at rest.
- Sensitive fields are excluded from logs and audit metadata.
- Request bodies have explicit size limits.

## Audit

Security-relevant operations create append-only audit events containing:

```text
principal_id
device_id
action
resource type and ID
result
request_id
timestamp
redacted metadata
```

The actor is derived from authentication. A caller-provided value such as `codex` or `claude` may be recorded as client metadata but is never trusted as identity.

## Backup and recovery

Automated backup is part of the MVP because the Hub exists to preserve continuity.

- daily PostgreSQL dump to a versioned S3 bucket;
- object lifecycle retention;
- documented restore procedure;
- periodic restore verification;
- artifact metadata and object digests included in consistency checks.

An EBS snapshot can supplement these backups but does not replace a tested database restore.
