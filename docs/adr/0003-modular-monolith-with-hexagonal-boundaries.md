# ADR 0003: Use a modular monolith with hexagonal boundaries

- Status: Accepted
- Date: 2026-09-24

## Context

The system has meaningful domain, protocol, persistence, and local-execution boundaries, but its scale does not justify distributed services. A traditional controller/service/repository layout would make it easy for HTTP, MCP, and database concerns to leak into domain rules.

## Decision

Build one Hub application and one local client in a single repository. Organize each using inward dependency direction:

```text
adapters -> application -> domain
```

Domain modules cover work continuity, knowledge, capability catalog, and governance. Interfaces are defined by consuming application use cases and implemented by adapters.

Read models may use optimized database projections. This decision does not require event sourcing, separate CQRS infrastructure, or microservices.

## Consequences

- REST, MCP, and CLI reuse the same application behavior.
- PostgreSQL and S3 can be replaced or tested behind narrow ports.
- Module boundaries are enforceable with ordinary Go packages and import rules.
- Some mapping between transport, application, domain, and persistence types is intentional.
- Generic repositories, base services, and speculative plugin abstractions are discouraged.

## Alternatives considered

### Microservices

Rejected because they add deployment, consistency, and observability cost without an independent scaling need.

### Framework-style layered CRUD

Rejected because the important operations are task claiming, checkpoint appending, context assembly, and artifact publishing rather than entity-shaped CRUD.
