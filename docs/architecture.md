# Architecture

## System context

Agent Control Plane consists of two applications with different trust and execution boundaries.

### Context Hub

The Context Hub runs on the user's server. It authenticates clients, stores durable context, assembles bootstrap responses, publishes versioned artifacts, and records audit events.

It never receives environment credentials and never operates a project workspace.

### Local client

The `ctx` client runs on the computer where work happens. It provides a CLI and a local MCP server. It can inspect the current workspace using locally available tools and credentials, merge those facts with remote context, and publish only data allowed by the project's sync policy.

```text
┌──────────────── Local computer ────────────────┐
│                                                │
│  AI agent ──> ctx CLI / local MCP              │
│                    |             |              │
│                    |             +--> git/files │
│                    |             +--> local CLI │
└────────────────────┼────────────────────────────┘
                     │ REST/JSON over HTTPS
┌────────────────────▼────────────────────────────┐
│ Context Hub                                     │
│                                                 │
│ inbound adapters -> application -> domain       │
│                              ^                  │
│                              | ports            │
│                    outbound adapters            │
│                    PostgreSQL / S3              │
└─────────────────────────────────────────────────┘
```

## Architectural style

The Hub and client are built as a modular monolith using hexagonal boundaries.

```text
Inbound adapters
HTTP / MCP / CLI
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

REST/JSON is the canonical network protocol. The API is versioned under `/api/v1`, documented by OpenAPI, and uses stable machine-readable error codes.

MCP maps tools and resources to the same application use cases. It must contain no independent authorization, selection, or persistence logic.

The initial MCP integration runs locally:

```text
agent -> stdio MCP (`ctx mcp serve`) -> Hub REST API
                                  └-> local workspace observations
```

A remote MCP endpoint can be added later without changing the domain or application layers.

## Local observation model

The Hub does not poll Git, Kubernetes, AWS, or other project systems. The local client may submit a typed observation after applying the project's sync policy.

Every observation includes:

```text
kind
schema_version
project_id
source_device_id
observed_at
expires_at optional
payload
payload_digest
```

Expired observations are marked stale. A failed refresh never silently presents old information as current.

## Planned Go layout

```text
cmd/
├── hub/
└── ctx/

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
    ├── mcp/
    ├── postgres/
    ├── s3/
    └── local/
```

Go interfaces should normally live next to the application code that consumes them. Adapter packages satisfy those interfaces implicitly.

## Deployment

The initial deployment has three containers:

```text
Caddy -> Hub -> PostgreSQL
               |
               +-> S3 artifact and backup buckets
```

- Caddy is the only public process.
- PostgreSQL is available only on the internal container network.
- The Hub's instance role is limited to its own parameters, artifacts, backups, image pulls, and logs.
- No role grants access to a project or company environment.

Redis, Kafka, Temporal, Kubernetes, Elasticsearch, and a separate worker service are intentionally excluded.
