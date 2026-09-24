# Implementation roadmap

The roadmap is organized as vertical slices. Each phase must leave the repository buildable, tested, and documented.

## Phase 0: executable skeleton

- initialize the Go module;
- add `cmd/hub`;
- structured logging and configuration;
- local Docker Compose with PostgreSQL;
- migration runner;
- `/health` and `/ready`;
- versioned REST routing and a published `/openapi.json` contract;
- formatting, vetting, tests, and CI.

Exit condition: a clean checkout can start the Hub and verify database readiness.

## Phase 1: work continuity

- spaces and projects;
- tasks with optimistic versions;
- progress and handoff checkpoints;
- scoped client tokens with an operator-only initial provisioning path, expiration, and revocation;
- per-resource authorization on every read and write;
- idempotent checkpoint writes so retries cannot create duplicate handoffs;
- append-only audit events;
- REST endpoints for the above.

Exit condition: authenticated HTTP clients on two computers can create a task, append a handoff, and retrieve it without installing a project-specific binary. Cross-space access and duplicate retry attempts are rejected.

## Phase 2: bootstrap context

- decisions and rules;
- deterministic rule selection;
- bounded `ContextPack` assembly;
- bootstrap REST endpoint;
- a JSON bootstrap response with a documented selection order, size budget, provenance, and freshness markers;
- authenticated bootstrap and checkpoint calls exercised from at least two agent products;
- concurrency and cross-space isolation tests.

Exit condition: a new session using a different supported agent can retrieve the active task, latest handoff, decisions, and effective rules in one request.

## Phase 3: skill registry

- vendor-neutral skill manifest schema;
- immutable skill versions;
- S3-compatible artifact storage;
- digest and archive validation;
- publish, list, and fetch REST operations;
- installation adapters kept outside the domain and added only for supported clients.

Exit condition: a skill published from one computer can be verified and fetched on another without depending on an agent vendor.

## Phase 4: agent integrations

- document supported agent connection paths and their credential configuration;
- add remote MCP over HTTPS if it materially improves client access;
- map any MCP tools to existing application operations;
- implement the authentication and discovery required by the selected MCP specification;
- keep local workspace observations optional and policy-gated;
- compare REST and MCP behavior in integration tests if MCP is added.

Exit condition: at least two supported agent clients can bootstrap and checkpoint the same project through their supported integration paths.

## Phase 5: workflow and operations

- immutable workflow versions;
- search using PostgreSQL full-text search;
- automated PostgreSQL backup to S3;
- restore runbook and restore test;
- production Docker Compose, Caddy, ECR, SSM deployment;
- resource limits and basic operational metrics.

Exit condition: the Hub can be safely operated on the existing small EC2 footprint and recovered from a fresh instance.

## Deferred

The following require demonstrated need:

- web UI;
- OAuth/OIDC provider integration for the REST API unless a supported integration requires it earlier;
- vector or embedding search;
- local `ctx` CLI or MCP adapter;
- multi-user administration;
- background queue;
- workflow execution engine;
- agent-to-agent protocol;
- autonomous agent execution.

## MVP acceptance scenario

1. Start a task from computer A using one agent.
2. Record checkpoints and a final handoff.
3. Open computer B with a different agent vendor.
4. Make an authenticated bootstrap API request from an HTTP-capable integration.
5. Receive the current objective, latest handoff, relevant decisions, rules, and any published skill references.
6. Continue without granting the Hub access to either computer's repository or environment credentials.
