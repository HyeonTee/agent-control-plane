# Security and data boundaries

## Threat model

The Hub is reachable from multiple computers and may hold context about unrelated personal and company work. The primary risks are:

- a leaked client token;
- a third-party agent integration storing or exposing a token;
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

Every project has an explicit sync policy. An agent or integration must check it before sending data, and the Hub enforces allowed request types, declared classification, schema, and size. The Hub cannot prove that arbitrary free text was sanitized before upload. A policy that requires reliable local filtering needs a reviewed local adapter or a human review step; it must not be represented as server-enforced redaction.

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

Supported policies must include a `local_only` mode. Such a project has no remotely synchronized project content or remote bootstrap; a hosted API cannot provide continuity for data that must stay local. Company policy and contractual obligations take precedence over the user's desire for continuity. Data that is not approved for personal infrastructure stays in the local repository or an organization-approved store.

## Authentication

The first REST API uses client-specific opaque bearer tokens. A token identifies an integration or installation acting for a principal; a self-reported agent name is not an identity. Initial owner access and token provisioning use an operator-only path on the host. Public self-registration and unauthenticated token minting are excluded.

Each token has:

```text
id, display name, and principal id
secret hash
allowed spaces
scopes
created_at
expires_at
last_used_at
revoked_at
```

Tokens are high-entropy random values, shown once, and sent only in an HTTPS `Authorization` header. They are never placed in a URL, OpenAPI description, repository file, agent prompt, or log. Each supported integration must have a secure credential configuration; clients without one are not given a token. A hosted agent integration that stores a token becomes part of the trust boundary, so issue a separate token per integration with only its required spaces and scopes. The Hub stores only a hash. Tokens expire and can be rotated or revoked independently. Read, checkpoint-write, and artifact-publish scopes are separate.

OAuth or passkeys may replace this REST flow later without changing the domain authorization model. A future protected remote MCP endpoint must implement the authorization mechanism required by its selected MCP specification; a static REST token alone does not establish MCP client interoperability.

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

Every request must check both the action scope and the ownership chain of its specific resource (space, project, task, checkpoint, or artifact). Knowing an object ID never grants access. Cross-space reads, writes, searches, and bootstrap assembly require isolation tests.

## Content trust

Stored content carries a trust level and provenance. Reference documents, repository content, and observations are treated as data, not instructions.

Only explicit rules and approved skill instructions may influence agent behavior as instructions. Bootstrap output preserves that distinction instead of concatenating all content into one prompt.

## Artifact safety

Skill and workflow packages are immutable and addressed by digest. The Hub verifies size, archive paths, declared manifest, and digest before publishing.

Downloading a package does not authorize execution. The agent integration or optional local adapter must display requested capabilities and apply local approval policy before installing or running scripts.

## Transport and storage

- HTTPS is mandatory outside local development.
- `/openapi.json` describes the API but includes no private data or credential values. All project data and writes require authentication.
- PostgreSQL is not exposed publicly.
- Secrets are read from the Hub's own secret store at startup.
- S3 buckets block public access and use encryption at rest.
- Sensitive fields are excluded from logs and audit metadata.
- Sensitive context responses use `Cache-Control: no-store`, and proxy logs never record authorization headers or request bodies.
- Request bodies, result sizes, page sizes, and request rates have explicit limits. Unexpected fields and unsupported content types are rejected.
- Browser cross-origin access is disabled unless a specific trusted origin and credential flow are designed.

## Audit

Security-relevant operations create append-only audit events containing:

```text
principal_id
client_id
action
resource type and ID
result
request_id
timestamp
redacted metadata
```

The actor and client ID are derived from authentication. A caller-provided value such as `codex` or `claude` may be recorded as client metadata but is never trusted as identity. Authentication failures, token changes, writes, and sensitive context reads are auditable without storing request or response bodies or bearer tokens.

## Backup and recovery

Automated backup is part of the MVP because the Hub exists to preserve continuity.

- daily PostgreSQL dump to a versioned S3 bucket;
- object lifecycle retention;
- documented restore procedure;
- periodic restore verification;
- artifact metadata and object digests included in consistency checks.

An EBS snapshot can supplement these backups but does not replace a tested database restore.

## Security references

- [OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)
- [OWASP API Security Top 10](https://devguide.owasp.org/en/07-training-education/07-api-top-ten/)
- [Bearer token usage, RFC 6750](https://www.rfc-editor.org/rfc/rfc6750)
- [MCP authorization specification](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)
