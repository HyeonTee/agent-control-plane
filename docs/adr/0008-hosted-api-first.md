# ADR 0008: Hosted API is the first client interface

- Status: Accepted
- Date: 2026-09-24
- Supersedes: ADR 0002, ADR 0003, ADR 0004, ADR 0005

## Context

The first useful outcome is a handoff written by one agent and retrieved by another. Requiring every computer to install `ctx` before this works adds a distribution and update burden. Agents have different HTTP and MCP capabilities, so a published API contract is necessary but does not by itself make every agent an automatic API client.

## Decision

The first supported access path is the hosted REST/JSON API at `agent.gwinam.com`. The Hub publishes a versioned OpenAPI description at `/openapi.json`. The description contains no project data or credentials; data operations require authentication and authorization. Agents or their configured HTTP integrations call the API directly.

The Hub remains a Go modular monolith with inward dependencies, `pgx` and `sqlc` for PostgreSQL persistence, and object storage when artifact packages are introduced. A local `ctx` CLI and local MCP process are optional future adapters, introduced only if local policy enforcement, workspace observation, or client compatibility justifies their installation cost. If implemented, they use the same hosted REST use cases and Go remains the preferred implementation language.

The Hub never inspects a local workspace or runs project commands. An agent or optional local adapter may submit an observation measured in its own environment, subject to the project's sync policy. Observations are optional and carry provenance and freshness metadata.

Remote MCP may later expose the same application use cases for clients that support it. It is not required for the first handoff flow. Its authentication and protocol behavior must be implemented and tested according to the applicable MCP specification; an OpenAPI endpoint is not an MCP endpoint.

## Consequences

- A clean agent environment needs an authenticated HTTP integration, but no project-specific local binary.
- Client compatibility must be verified per agent; reading OpenAPI alone does not grant an agent HTTP tools or credential storage.
- Without a local adapter, the Hub cannot prove that arbitrary text was sanitized before upload. Server-side schema, classification, authorization, and size checks reduce exposure but cannot replace source-side review.
- Local observation automation and offline support are deferred until their value is demonstrated.
- REST remains the canonical contract. CLI and MCP are adapters when added, not independent sources of business rules.

## Alternatives considered

### Require `ctx` on every computer

Deferred because installation and updates are unnecessary for the first cross-agent handoff. A local helper remains useful when automatic workspace inspection or local filtering becomes necessary.

### Make OpenAPI discovery the only integration mechanism

Rejected because an agent must also be able to make authenticated HTTP calls. A future remote MCP endpoint can provide tool discovery for clients that support it.
