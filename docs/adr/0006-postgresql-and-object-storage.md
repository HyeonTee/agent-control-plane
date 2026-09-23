# ADR 0006: Use PostgreSQL and object storage

- Status: Accepted
- Date: 2026-09-24

## Context

The Hub stores relational work state, searchable text, flexible versioned manifests, immutable packages, tokens, and audit events. The expected traffic is low, but durability and backup are core product requirements.

## Decision

Use PostgreSQL for domain metadata, text content, full-text search, authentication metadata, and audit events. Use S3-compatible object storage for immutable skill and workflow package content.

Use relational columns for identity, ownership, status, versions, and timestamps. Use JSONB only for genuinely flexible payloads such as manifests and typed observation bodies.

Do not introduce a vector database, Redis, Elasticsearch, or a message broker in the MVP.

## Consequences

- One database handles transactions and initial search requirements.
- Large immutable packages do not bloat database backups.
- Artifact metadata must include digest and size so database and object storage can be checked for consistency.
- Automated database backup and restore verification are required from the first production deployment.

## Alternatives considered

### PostgreSQL only

Reasonable for the smallest prototype, but package files and future assets are a better fit for object storage and independent retention.

### Git as the primary operational database

Rejected for tasks, tokens, audit, and concurrent checkpoints. Git import and export may still be added for human-authored configuration.
