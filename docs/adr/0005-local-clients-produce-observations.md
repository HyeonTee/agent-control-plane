# ADR 0005: Local clients produce environment observations

- Status: Accepted
- Date: 2026-09-24

## Context

Git working-tree state and project runtime facts exist in the environment where work is performed. That environment may be a personal computer, a company computer, or a network inaccessible to the personal Hub.

Server-side collection would require credentials, network access, and provider-specific behavior in the Hub.

## Decision

The `ctx` client collects local observations using local tools and credentials. It applies the project's sync policy before optionally publishing a typed, time-stamped observation to the Hub.

The Hub stores observations but does not execute the commands that produce them.

## Consequences

- Credentials stay within their original security boundary.
- The same Hub works with local, company, and future environments.
- Observations need provenance, schema versions, timestamps, expiration, and digests.
- Bootstrap context may contain no runtime observation when policy prohibits upload; this is an expected state.

## Alternatives considered

### Hub-side Git, Kubernetes, and cloud adapters

Rejected because the Hub is a context registry, not an environment operator.

### Never store observations

Rejected because sanitized metadata such as revision and test status can materially improve continuity when policy allows it.
