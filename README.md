# Agent Control Plane

Vendor-neutral personal context hub for continuing work across computers, sessions, and AI coding agents.

The system does not give an agent memory. It stores enough durable, versioned context for a new agent session to reconstruct the current state of work.

## Why

Coding work is increasingly distributed across multiple computers and agents such as Claude, Codex, Grok, and future tools. A fresh session usually has to rediscover:

- what is currently being built;
- what the previous session completed;
- which decisions and warnings still apply;
- which skills and workflows should be used;
- what should happen next.

Agent Control Plane provides one user-owned place from which those agents can pull the same context and to which they can publish checkpoints and handoffs.

## Product boundary

The hosted service is a passive context registry. It stores and serves:

- projects, tasks, checkpoints, and handoffs;
- decisions, rules, and context documents;
- versioned skills, workflows, and agent profiles;
- client-submitted observations;
- authentication metadata and audit events.

It does **not**:

- clone or modify working repositories;
- run `git`, `kubectl`, cloud CLIs, tests, or builds;
- hold credentials for company or project environments;
- execute AI agents or LLM inference;
- orchestrate autonomous production changes.

Local work is performed by the agent and a local `ctx` client. The client may inspect the current workspace, combine local facts with remote context, and publish an allowed checkpoint back to the hub.

```text
Claude / Codex / Grok
          |
          v
  ctx CLI / local MCP
    |             |
    |             +--- local repository and tools
    |
    +--- HTTPS ---> Context Hub
                    - work state
                    - knowledge
                    - skills/workflows
                    - audit
```

## Design principles

1. **Vendor neutral**: provider names do not appear in domain rules. Vendors are clients or adapters.
2. **Local execution**: workspace inspection and environment commands run on the user's computer.
3. **Explicit data boundaries**: each project declares what may be synchronized to the personal hub.
4. **Versioned knowledge**: published skills, workflows, and durable decisions are traceable and reproducible.
5. **Append-oriented continuity**: checkpoints and handoffs form a history instead of overwriting prior work.
6. **Small operational footprint**: one Go application, PostgreSQL, object storage, and Caddy.
7. **No speculative infrastructure**: no orchestration engine, vector database, queue, or microservices in the MVP.

## Technology direction

- Go for the hub server, CLI, and local MCP adapter
- PostgreSQL for relational and searchable metadata
- S3-compatible object storage for skill and workflow artifacts
- REST/JSON as the canonical remote API
- MCP as an adapter over application use cases
- Caddy and Docker Compose for deployment
- OpenTofu for AWS infrastructure

The exact dependency versions will be pinned when the executable skeleton is created. See [ADR 0002](docs/adr/0002-use-go.md).

## MVP

The first usable vertical slice is intentionally small:

1. authenticate a device;
2. register a project and task;
3. append a checkpoint or handoff;
4. publish and retrieve a versioned skill;
5. build a compact bootstrap context;
6. access the same use cases from REST, `ctx`, and local MCP;
7. audit writes and back up durable data.

Success means that work started with one agent can be resumed from another computer or agent without re-explaining the task from scratch.

## Planned repository layout

```text
.
├── cmd/
│   ├── hub/                    # hosted API server
│   └── ctx/                    # local CLI and MCP process
├── internal/
│   ├── domain/
│   ├── application/
│   └── adapter/
├── api/                        # OpenAPI contract
├── schemas/                    # skill/workflow/context schemas
├── migrations/
├── deploy/
└── docs/
```

The repository currently contains design documentation only. Commands for building, testing, and deployment will be added with the first executable slice rather than documented speculatively.

## Documentation

- [Architecture](docs/architecture.md)
- [Domain model](docs/domain-model.md)
- [Security and data boundaries](docs/security.md)
- [Implementation roadmap](docs/roadmap.md)
- [Architecture decision records](docs/adr/README.md)

## Status

Design phase. No server or CLI implementation exists yet.
