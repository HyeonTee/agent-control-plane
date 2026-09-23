# ADR 0007: Agent Control Plane is the product boundary

- Status: Accepted
- Date: 2026-09-24
- Supersedes: ADR 0001

## Context

The initial architecture described the entire hosted product as a passive Context Hub. That accurately limits the MVP but makes the repository name `agent-control-plane` appear broader than the product.

The project should retain room for future agent registration and execution coordination without expanding today's implementation scope or allowing the Hub API process to execute project workloads.

## Decision

`Agent Control Plane` is the long-term product and repository boundary. `Context Hub` is its first module.

The product is organized conceptually as:

```text
Agent Control Plane
├── Context Hub
├── Capability Registry
├── Policy & Identity
└── Agent Operations        # future
```

Context Hub remains passive: it stores, versions, searches, and serves work context but does not operate repositories or project environments.

Future Agent Operations may manage desired state, agent and runner registration, assignments, scheduling, and run status. Workloads execute in a separate Execution Plane such as a local client, CI runner, or isolated hosted runner. The Control Plane API process does not directly execute untrusted work.

No Agent Operations packages, ports, database tables, or deployment components are created until a concrete use case requires them.

## Consequences

- The existing repository name remains accurate as the product grows.
- Current documentation distinguishes long-term product boundaries from MVP scope.
- Context Hub security properties remain valid when new modules are added.
- Future coordination features have a defined architectural location without speculative implementation.
- Execution credentials remain scoped to runners rather than the Control Plane or Context Hub.

## Alternatives considered

### Rename the product to Work Context Hub

Rejected because it accurately names the MVP but would become too narrow if agent and runner coordination are added later.

### Add execution directly to Context Hub

Rejected because it mixes durable context storage with untrusted workload execution and greatly increases the blast radius.
