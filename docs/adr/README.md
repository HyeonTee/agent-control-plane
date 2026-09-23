# Architecture decision records

ADRs record decisions that constrain future implementation. They are append-only: when a decision changes, add a new ADR that supersedes the old one instead of rewriting history.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-context-hub-boundary.md) | The hosted service is a passive context hub | Superseded by 0007 |
| [0002](0002-use-go.md) | Use Go for the Hub and local client | Accepted |
| [0003](0003-modular-monolith-with-hexagonal-boundaries.md) | Use a modular monolith with hexagonal boundaries | Accepted |
| [0004](0004-rest-is-the-canonical-api.md) | REST/JSON is canonical; MCP is an adapter | Accepted |
| [0005](0005-local-clients-produce-observations.md) | Local clients, not the Hub, inspect work environments | Accepted |
| [0006](0006-postgresql-and-object-storage.md) | Use PostgreSQL and object storage | Accepted |
| [0007](0007-agent-control-plane-is-the-product-boundary.md) | Agent Control Plane is the product boundary; Context Hub is its first module | Accepted |

## Format

Each ADR contains status, context, decision, consequences, and alternatives considered. Dates use the local project timezone.
