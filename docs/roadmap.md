# Implementation roadmap

The roadmap is organized as vertical slices. Each phase must leave the repository buildable, tested, and documented.

## Phase 0: executable skeleton

- initialize the Go module;
- add `cmd/hub` and `cmd/ctx`;
- structured logging and configuration;
- local Docker Compose with PostgreSQL;
- migration runner;
- `/health` and `/ready`;
- formatting, vetting, tests, and CI.

Exit condition: a clean checkout can start the Hub and verify database readiness.

## Phase 1: work continuity

- spaces and projects;
- tasks with optimistic versions;
- progress and handoff checkpoints;
- device tokens and scoped authentication;
- append-only audit events;
- REST endpoints for the above;
- `ctx login`, `ctx task`, and `ctx checkpoint`.

Exit condition: one computer can create a task and another can retrieve its latest handoff safely.

## Phase 2: bootstrap context

- decisions and rules;
- deterministic rule selection;
- bounded `ContextPack` assembly;
- bootstrap REST endpoint;
- `ctx bootstrap` in JSON and Markdown formats;
- concurrency and cross-space isolation tests.

Exit condition: a new session can retrieve the active task, latest handoff, decisions, and effective rules in one request.

## Phase 3: skill registry

- vendor-neutral skill manifest schema;
- immutable skill versions;
- S3-compatible artifact storage;
- digest and archive validation;
- publish, list, fetch, and sync commands;
- local installation adapters kept outside the domain.

Exit condition: a skill published from one computer can be verified and installed on another without depending on an agent vendor.

## Phase 4: local MCP

- `ctx mcp serve` over stdio;
- MCP tools mapped to existing application operations;
- local observation collection;
- sync-policy filtering before upload;
- integration tests comparing REST, CLI, and MCP behavior.

Exit condition: at least two supported agent clients can bootstrap and checkpoint the same project through the local MCP server.

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
- OAuth/OIDC provider integration;
- vector or embedding search;
- remote MCP endpoint;
- multi-user administration;
- background queue;
- workflow execution engine;
- agent-to-agent protocol;
- autonomous agent execution.

## MVP acceptance scenario

1. Start a task from computer A using one agent.
2. Record checkpoints and a final handoff.
3. Open computer B with a different agent vendor.
4. Run `ctx bootstrap <project>` or the equivalent MCP tool.
5. Receive the current objective, latest handoff, relevant decisions, rules, and skill references.
6. Continue without granting the Hub access to either computer's repository or environment credentials.
