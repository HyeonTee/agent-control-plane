# ADR 0004: REST/JSON is the canonical remote API

- Status: Superseded by ADR 0008
- Date: 2026-09-24

## Context

Different agent vendors support different integration mechanisms. Making MCP the internal architecture or persistence contract would couple the product to one client protocol.

## Decision

Use a versioned REST/JSON API as the canonical Hub interface. Publish an OpenAPI contract and version durable payload schemas.

MCP, CLI commands, and future interfaces call the same application use cases. The initial MCP server runs locally as `ctx mcp serve` over stdio and talks to the Hub REST API.

## Consequences

- Clients without MCP support can use the CLI or REST directly.
- MCP protocol changes remain isolated to one adapter.
- The local MCP process can combine Hub context with local workspace observations.
- API DTOs are distinct from domain entities, creating some explicit mapping code.

## Alternatives considered

### MCP as the only remote interface

Rejected because MCP client support and configuration differ across vendors and because it is not the domain storage model.

### gRPC

Deferred because browser compatibility and generated clients are not currently needed, while JSON is useful for agent and CLI interoperability.
