# Architecture

## System context

Agent Control Plane is the product boundary. The first implementation is a hosted API. Agents with authenticated HTTP integrations call it directly; workspace execution remains in the agent's environment.

The first delivered module is the Context Hub. Capability Registry and Policy & Identity support it. Agent Operations is a future module and is not part of the MVP.

```text
Agent Control Plane
├── Context Hub                 # current MVP
├── Capability Registry         # current MVP
├── Policy & Identity           # current MVP
└── Agent Operations            # future
    ├── Agent Registry
    ├── Run Coordination
    ├── Runner Management
    └── Scheduling

Execution Plane                 # separate trust boundary
├── coding agents and their local tools
├── CI runners
├── optional local ctx adapter
└── future isolated hosted runners
```

The Control Plane owns desired state, policy, assignments, and status. It does not execute untrusted project workloads inside its API process.

### Context Hub

The Context Hub runs on the user's server. It authenticates clients, stores durable context, assembles bootstrap responses, publishes versioned artifacts, and records audit events.

It never receives environment credentials and never operates a project workspace.

### Agent environment

An agent may inspect its current workspace using tools available in its environment. An authenticated HTTP integration can retrieve Hub context and publish permitted data. The agent or integration must check the project's sync policy before upload. A local `ctx` adapter may be added later to automate this policy check and workspace observation; it is not required for the first usable flow.

```text
┌──────────────── Local computer ────────────────┐
│                                                │
│  AI agent ──> local repository and tools        │
│       |                                        │
│       +--> authenticated HTTP integration       │
└────────────────────┼────────────────────────────┘
                     │ REST/JSON over HTTPS
┌────────────────────▼────────────────────────────┐
│ Agent Control Plane                             │
│ Context Hub / Capability / Governance           │
│                                                 │
│ inbound adapters -> application -> domain       │
│                              ^                  │
│                              | ports            │
│                    outbound adapters            │
│                    PostgreSQL / S3              │
└─────────────────────────────────────────────────┘
```

## Architectural style

The Hub is built as a modular monolith using hexagonal seams. An optional local adapter can follow the same dependency direction if introduced.

```text
Inbound adapters
HTTP (MCP or CLI if added later)
        |
        v
Application
use cases, authorization, context assembly
        |
        v
Domain
entities, value objects, policies, invariants
        ^
        | application-defined ports
Outbound adapters
PostgreSQL / S3 / local process / local filesystem
```

Dependencies point inward:

```text
adapter -> application -> domain
```

The domain must not import HTTP, MCP, PostgreSQL, AWS, filesystem, or vendor-specific packages.

## Modules

The modules are logical boundaries inside one deployable application, not microservices.

### Context Hub

Context Hub is the first product module. It contains work continuity, knowledge, observations, and context assembly. It remains a passive context module even if Agent Operations is added later.

### Work continuity

Owns projects, tasks, checkpoints, and handoffs. It answers what is being done, what changed, and what should happen next.

### Knowledge

Owns context documents, decisions, and rules. It distinguishes trusted instructions from reference material and tracks provenance.

### Capability catalog

Owns immutable versions of skills, workflows, and agent profiles. It stores metadata in PostgreSQL and package content in object storage.

### Governance

Owns spaces, principals, tokens, access scopes, sync policies, and audit events. It prevents data from one organizational boundary from being returned in another.

### Context assembly

Builds a bounded `ContextPack` from the other modules. A context pack is an application response, not a domain entity or a permanent copy of all source data.

### Agent Operations

Reserved as a future product module for agent registration, run coordination, runner management, and scheduling. It has no code or interfaces in the MVP. When introduced, workload execution must occur in a separate Execution Plane rather than in the Hub API process.

## Application use cases

Commands change durable state:

```text
CreateProject
CreateTask
ClaimTask
AppendCheckpoint
RecordDecision
PublishSkillVersion
PublishWorkflowVersion
RecordObservation
RevokeToken
```

Queries return views:

```text
ListActiveWork
GetTaskOverview
ListTaskSessions
GetSessionHistory
BootstrapContext
SearchContext
GetCurrentTask
GetTaskTimeline
GetLatestHandoff
DownloadSkillVersion
ListSkillVersions
```

Use cases are named after user intentions. Entity-shaped CRUD services such as `TaskService` or a generic `Repository<T>` are avoided.

## Ports and adapters

Interfaces are introduced only at real volatility or trust boundaries. Application packages define the smallest interface they consume.

Representative ports:

```text
TaskStore
CheckpointStore
KnowledgeCatalog
BootstrapViewStore
ArtifactStore
ContextSearch
AuditSink
Clock
IDGenerator
```

Atomic domain operations should be reflected by atomic ports. For example, appending a checkpoint and advancing a task version should be one storage operation rather than a generic save sequence.

```text
AppendCheckpoint(taskID, expectedVersion, checkpoint)
```

This allows PostgreSQL to enforce the operation in one transaction and makes concurrent changes return a conflict.

## Write and read models

Commands load domain state and enforce invariants. Queries may use optimized projections and joins rather than reconstructing every aggregate.

`BootstrapContext`, for example, may use `BootstrapViewStore` to load candidate data efficiently, then apply authorization, sync policy, freshness, relevance, and size limits in the application layer.

This is a light command/query separation, not a separate CQRS infrastructure.

## Canonical protocol

REST/JSON is the canonical network protocol. The API is versioned under `/api/v1`, documented by an OpenAPI description served at `/openapi.json`, and uses stable machine-readable error codes. The API description contains no project data or credentials. Project data and all writes require authentication and per-resource authorization.

The first workflow needs endpoints to create a project and task, append a checkpoint, read a task timeline, and bootstrap a project. For example, a checkpoint append targets `POST /api/v1/tasks/{task_id}/checkpoints`, includes the task version observed by the caller, and carries an `Idempotency-Key` header. Retrying the same key and payload returns the original result; reusing a key for different content fails. A stale task version returns a conflict. The OpenAPI contract will define the final route names, payload schemas, and error codes before implementation.

An OpenAPI description tells clients how to call the API; it does not give an agent an HTTP tool or credential store. Each supported agent integration must be exercised with authenticated reads and writes.

Remote MCP may be added later for clients that support it:

```text
agent -> remote MCP over HTTPS -> same application use cases
```

MCP must contain no independent authorization, selection, or persistence logic. Its authentication and transport behavior must follow the MCP specification selected at implementation time. A local MCP adapter remains possible if local capabilities require it.

## Local observation model

The Hub does not poll Git, Kubernetes, AWS, or other project systems. An agent or optional local adapter may submit a typed observation after checking the project's sync policy. The Hub validates authorization, declared classification, schema, and size but cannot prove that free text was sanitized at its source.

Every observation includes:

```text
kind
schema_version
project_id
source_client_id (derived from authentication)
source_device_id optional
observed_at
expires_at optional
payload
payload_digest
```

Expired observations are marked stale. A failed refresh never silently presents old information as current.

## Planned Go layout

```text
cmd/
└── hub/

internal/
├── domain/
│   ├── work/
│   ├── knowledge/
│   ├── capability/
│   └── governance/
├── application/
│   ├── command/
│   └── query/
└── adapter/
    ├── httpapi/
    ├── postgres/
    └── s3/                    # when artifact storage is introduced
```

Go interfaces should normally live next to the application code that consumes them. Adapter packages satisfy those interfaces implicitly. Optional CLI or MCP adapter packages are added only when implemented.

## Deployment

The initial deployment reuses the existing EC2 instance. Its proposed public route is:

```text
agent.gwinam.com -> CloudFront -> dedicated Hub origin port on EC2
                                  |
                                  +-> Hub Compose project -> private PostgreSQL
                                  +-> S3 backup bucket when deployed
```

- Justice already owns host port 80; the Hub uses a separate origin listener and Compose project.
- The origin transport and certificate renewal path must be decided before private project data is exposed. See [the existing EC2 deployment note](deployment-existing-ec2.md).
- PostgreSQL is available only on the internal container network.
- The Hub's instance role is limited to its own parameters, artifacts, backups, image pulls, and logs.
- No role grants access to a project or company environment.

Redis, Kafka, Temporal, Kubernetes, Elasticsearch, and a separate worker service are intentionally excluded.
