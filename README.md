# Agent Control Plane

Vendor-neutral control plane for agent context, capabilities, policies, and future execution coordination.

The first module is a Context Hub. It does not give an agent memory; it stores enough durable, versioned context for a new agent session to reconstruct the current state of work.

Context Hub is passive with respect to project work: agents create checkpoints, handoffs, and any future summaries; the Hub authenticates, validates, stores, indexes, and returns those records. It does not run agents, invoke an LLM, inspect a workspace, or initiate task changes on its own. [The project glossary](CONTEXT.md) names these responsibilities.

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
- generate summaries or compact records on behalf of an agent;
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
- Docker Compose on the existing EC2 instance, with a separate CloudFront distribution planned for the public API
- OpenTofu for AWS infrastructure

The Go toolchain and dependencies are pinned in `go.mod` and `go.sum`. See [ADR 0008](docs/adr/0008-hosted-api-first.md).

## MVP

The first usable vertical slice is intentionally small:

1. provision a scoped client token and authenticate API requests;
2. register a project and task;
3. append a checkpoint or handoff;
4. build a compact bootstrap context;
5. publish an OpenAPI contract and exercise the flow with authenticated HTTP requests;
6. audit writes and back up durable data.

Success means that work started with one agent can be resumed from another computer or agent without re-explaining the task from scratch.

## Repository layout

```text
.
├── cmd/
│   └── hub/                    # hosted API server
├── internal/
│   ├── adapter/
│   │   ├── httpapi/
│   │   └── postgres/
│   │       └── migrations/     # embedded SQL migrations
│   └── config/
├── api/                        # OpenAPI contract
├── compose.yaml                # local development
├── Dockerfile
└── docs/
```

`ctx` and MCP adapter packages are added only if their use cases justify them.

The repository now contains the work-continuity and work-discovery APIs. Bootstrap context assembly, skill registry, and production deployment remain on the roadmap.

## Run locally

Docker Compose starts a private PostgreSQL container and binds the Hub to `127.0.0.1:8081` on the host:

```sh
docker compose up --build -d
curl -i http://127.0.0.1:8081/health
curl -i http://127.0.0.1:8081/ready
curl http://127.0.0.1:8081/openapi.json
```

The development database is also bound to `127.0.0.1:15433` for isolated integration tests. Do not use the local Compose credentials in production.

Provision the first owner and personal space once. The command prints a token secret once; store it in a password manager. Provisioning and later token issuance require operator access to the database, not a public HTTP endpoint.

```sh
docker compose run --rm -T hub admin bootstrap -space personal -token-name my-agent
```

Enter that secret without adding it to shell history, then create a project and task:

```sh
read -rs HUB_TOKEN
export HUB_TOKEN
curl -H "Authorization: Bearer $HUB_TOKEN" http://127.0.0.1:8081/api/v1/spaces
curl -X POST http://127.0.0.1:8081/api/v1/spaces/SPACE_ID/projects \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"my-project","sync_policy":"metadata_and_handoffs"}'
curl -X POST http://127.0.0.1:8081/api/v1/projects/PROJECT_ID/tasks \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"First task","objective":"Record and resume the work"}'
curl -X POST http://127.0.0.1:8081/api/v1/tasks/TASK_ID/sessions \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"client_label":"my-agent"}'
curl -X POST http://127.0.0.1:8081/api/v1/tasks/TASK_ID/checkpoints \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: first-handoff-001' \
  -d '{"session_id":"SESSION_ID","expected_version":1,"kind":"handoff","summary":"Ready to continue","remaining":["Take the next step"]}'
curl -X POST http://127.0.0.1:8081/api/v1/tasks/TASK_ID/sessions/SESSION_ID/close \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"summary":"Handoff published"}'
```

Issue a separate token for another integration, with only the scope it needs. The second token can discover active tasks, inspect one task, and read a session through the same HTTP API without installing this repository:

```sh
docker compose run --rm -T hub admin issue-token -space-id SPACE_ID \
  -name second-agent -scopes context:read -days 30
read -rs SECOND_TOKEN
export SECOND_TOKEN
curl -H "Authorization: Bearer $SECOND_TOKEN" \
  'http://127.0.0.1:8081/api/v1/tasks?status=active'
curl -H "Authorization: Bearer $SECOND_TOKEN" \
  http://127.0.0.1:8081/api/v1/tasks/TASK_ID
curl -H "Authorization: Bearer $SECOND_TOKEN" \
  http://127.0.0.1:8081/api/v1/tasks/TASK_ID/sessions/SESSION_ID
```

Replace `SPACE_ID`, `PROJECT_ID`, `TASK_ID`, and `SESSION_ID` with IDs returned by the API. The [OpenAPI contract](api/openapi.json) describes all endpoints and errors. Task and session lists return bounded pages with `next_cursor`; pass it back as the `cursor` query parameter to continue. A handoff requires at least one `remaining` action. Checkpoint retries use the same `Idempotency-Key` and request body; a changed body or stale task version returns 409. Older checkpoints without a session ID remain available through the task overview and `/pre-session-history`.

`/health` reports process liveness. `/ready` returns 204 only when PostgreSQL responds. The Hub runs embedded SQL migrations before accepting requests. The local Compose password is for development only and is not used for deployment.

To run the Hub without Compose, set `HUB_DATABASE_URL` to a PostgreSQL connection string and optionally set `HUB_HTTP_ADDR` (default `:8080`), then run `go run ./cmd/hub`. Run `go test ./...` and `go vet ./...` before committing. To run the PostgreSQL integration test locally, create a separate `hub_test` database and set `HUB_TEST_DATABASE_URL` to its connection URL; CI provisions one automatically. The test bootstraps an owner, so never point it at a database containing real data.

The first deployment target is the existing EC2 instance described in [the deployment note](docs/deployment-existing-ec2.md). Production Compose, HTTPS origin, backup/restore scripts, and a manual deploy workflow are separate from the local Compose file.

## Documentation

- [Architecture](docs/architecture.md)
- [Domain model](docs/domain-model.md)
- [Work discovery and session history](docs/work-discovery.md)
- [Security and data boundaries](docs/security.md)
- [Implementation roadmap](docs/roadmap.md)
- [Architecture decision records](docs/adr/README.md)

## Status

Work continuity and discovery are implemented locally: scoped tokens, spaces, projects, task discovery and overviews, task-scoped sessions, paged session entries, checkpoints, handoffs, optimistic versions, idempotent retries, and read/write audit events. PostgreSQL integration tests cover cross-space isolation and pagination. Production exposure still requires HTTPS origin configuration, rate limiting, database backup and restore testing, resource checks on the existing EC2 host, and a cross-computer agent integration check.
