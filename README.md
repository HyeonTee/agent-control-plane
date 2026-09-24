# Agent Control Plane

Vendor-neutral control plane for agent context, capabilities, policies, and future execution coordination.

The first module is a Context Hub. It does not give an agent memory; it stores enough durable, versioned context for a new agent session to reconstruct the current state of work.

## Why

Coding work is increasingly distributed across multiple computers and agents such as Claude, Codex, Grok, and future tools. A fresh session usually has to rediscover:

- what is currently being built;
- what the previous session completed;
- which decisions and warnings still apply;
- which skills and workflows should be used;
- what should happen next.

Agent Control Plane provides one user-owned platform from which those agents can pull the same context and capabilities and to which they can publish checkpoints and handoffs through a hosted API.

## Product boundary

Agent Control Plane is the product boundary. Its modules may eventually manage context, capabilities, policies, agent registration, and execution coordination.

The current MVP implements only the Context Hub and supporting registries. They store and serve:

- projects, tasks, checkpoints, and handoffs;
- decisions, rules, and context documents;
- versioned skills, workflows, and agent profiles;
- client-submitted observations;
- authentication metadata and audit events.

The current MVP does **not**:

- clone or modify working repositories;
- run `git`, `kubectl`, cloud CLIs, tests, or builds;
- hold credentials for company or project environments;
- execute AI agents or LLM inference;
- orchestrate autonomous production changes.

Local work is performed by the agent. An agent with an authenticated HTTP integration may call the Hub directly. An optional local adapter may later inspect the workspace, filter local facts, and publish an allowed checkpoint.

Future execution coordination belongs to an Agent Operations module. Even then, the Control Plane manages desired state, policy, assignments, and status; isolated runners in a separate Execution Plane perform workloads.

```text
Claude / Codex / other agents
      |                  |
      | local tools      | authenticated HTTPS
      v                  v
 local workspace   Agent Control Plane
                   ├── Context Hub       # current
                   ├── Capability Registry
                   ├── Policy & Identity
                   └── Agent Operations  # future

Optional later: local ctx adapter or remote MCP endpoint
```

## Design principles

1. **Vendor neutral**: provider names do not appear in domain rules. Vendors are clients or adapters.
2. **Local execution**: workspace inspection and environment commands run where the agent works, never in the Hub.
3. **Explicit data boundaries**: each project declares what may be synchronized to the personal hub.
4. **Versioned knowledge**: published skills, workflows, and durable decisions are traceable and reproducible.
5. **Append-oriented continuity**: checkpoints and handoffs form a history instead of overwriting prior work.
6. **Small operational footprint**: one Go application, PostgreSQL, object storage, and Caddy.
7. **No speculative infrastructure**: no orchestration engine, vector database, queue, or microservices in the MVP.

## Technology direction

- Go for the Hub server; optional local adapters may also use Go
- PostgreSQL for relational and searchable metadata
- S3-compatible object storage when skill and workflow artifacts are introduced
- REST/JSON as the canonical remote API
- Published OpenAPI contract for authenticated HTTP clients
- Remote MCP as a possible later adapter over application use cases
- Caddy and Docker Compose for deployment
- OpenTofu for AWS infrastructure

The exact dependency versions will be pinned when the executable skeleton is created. See [ADR 0002](docs/adr/0002-use-go.md).

## MVP

The first usable vertical slice is intentionally small:

1. provision a scoped client token and authenticate API requests;
2. register a project and task;
3. append a checkpoint or handoff;
4. build a compact bootstrap context;
5. publish an OpenAPI contract and exercise the flow with authenticated HTTP requests;
6. audit writes and back up durable data.

Success means that work started with one agent can be resumed from another computer or agent without re-explaining the task from scratch.

## Planned repository layout

```text
.
├── cmd/
│   └── hub/                    # hosted API server
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

`ctx` and MCP adapter packages are added only if their use cases justify them.

The repository currently contains design documentation only. Commands for building, testing, and deployment will be added with the first executable slice rather than documented speculatively.

## Documentation

- [Architecture](docs/architecture.md)
- [Domain model](docs/domain-model.md)
- [Security and data boundaries](docs/security.md)
- [Implementation roadmap](docs/roadmap.md)
- [Architecture decision records](docs/adr/README.md)

## Status

Design phase. No server or CLI implementation exists yet.
