# ADR 0001: The hosted service is a passive context hub

- Status: Accepted
- Date: 2026-09-24

## Context

Work must continue across computers, sessions, and AI agent vendors. Centralizing execution credentials and remote-control capabilities would increase risk and couple the system to every environment in which work occurs.

## Decision

The hosted service stores, searches, versions, and serves work context. It does not operate repositories, run environment commands, hold project credentials, execute agents, or perform LLM inference.

Execution remains local to the computer and agent session where the user is working.

## Consequences

- The Hub has a small and auditable permission surface.
- Company and personal environment credentials never need to reach personal infrastructure.
- A local client is required to combine remote context with current workspace facts.
- The name “control plane” refers to context governance and distribution, not remote execution.

## Alternatives considered

### Server-side runtime adapters

Rejected as the default because they require network reachability and credentials for every source environment and create a large blast radius.

### Full remote development environment

Rejected because the goal is continuity across existing environments, not replacing them.
