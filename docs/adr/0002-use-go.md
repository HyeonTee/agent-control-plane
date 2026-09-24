# ADR 0002: Use Go for the Hub and local client

- Status: Superseded by ADR 0008
- Date: 2026-09-24

## Context

The system needs a small HTTP service, a cross-platform CLI, a local MCP process, PostgreSQL access, object-storage integration, and straightforward deployment to a small ARM64 EC2 instance.

The workload is I/O-bound. Fast implementation and simple distribution are more important than maximum runtime performance.

## Decision

Use Go for both the hosted Hub and the local `ctx` client.

Initial technology choices:

- standard `net/http` server and middleware where practical;
- `pgx` with `sqlc` for PostgreSQL access;
- SQL migrations managed by a small migration tool;
- Cobra for the CLI;
- the official MCP Go SDK;
- standard `log/slog` structured logging;
- AWS SDK for Go v2 for S3 integration.

Dependencies and toolchain versions will be pinned in source control when the executable skeleton is created.

## Consequences

- Hub and client are distributed as small standalone binaries.
- Shared protocol and manifest packages can be reused without introducing a second implementation language.
- Compilation and test feedback remain fast.
- Domain invariants require disciplined types and tests because Go's type system expresses fewer illegal-state constraints than Rust.
- Framework-specific conventions are avoided in favor of explicit application boundaries.

## Alternatives considered

### Rust

Strong domain types and resource efficiency were attractive, but the MVP benefits more from Go's implementation speed and simpler async and integration model.

### Java

Java has excellent security, persistence, and observability ecosystems, but adds runtime and CLI distribution complexity that is not justified for the initial personal service.
