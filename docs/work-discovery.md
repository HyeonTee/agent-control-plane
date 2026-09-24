# Work discovery and session history

## Problem

The first Hub API required a pre-shared task ID. A new agent session can now discover active work, understand a selected task, and inspect the stored history behind a particular work session. Loading every session's content into the first response would make discovery slow and noisy.

## Three levels of reading

| Level | Caller gets | Route |
| --- | --- | --- |
| Task list | Authorized, active task cards: ID, project and space, title, status, latest summary, last activity, session count | `GET /api/v1/tasks?status=in_progress&limit=20&cursor=...` |
| Task overview | Objective, version, latest handoff, recent checkpoints, and a paged index of work sessions with their summaries | `GET /api/v1/tasks/{task_id}`; older sessions via `GET /api/v1/tasks/{task_id}/sessions?cursor=...` |
| Session detail | One session's metadata and its stored entries, paged in stable order | `GET /api/v1/tasks/{task_id}/sessions/{session_id}` and `/entries?cursor=...` |

The first response never fetches full session bodies. List routes use keyset cursors, bounded page sizes, deterministic ordering, and `Cache-Control: no-store`. A list response contains only data from spaces allowed by the authenticated token. A task or session outside those spaces returns 404. The default task filter is `active`, covering `todo`, `in_progress`, and `blocked`; clients can request one status or `all`.

For the initial task card, `latest_summary` is a projection of the newest checkpoint summary. The Hub does not generate it with an LLM. A later, explicitly published task summary can replace this projection if the excerpt proves insufficient. The overview assembles a bounded read model from the task, latest handoff, and session index; it does not persist a second copy of the whole history. It exposes both the latest checkpoint version and latest handoff version so a client can see whether work continued after the handoff.

## Meaning of a work session

A `WorkSession` is one continuous stretch of work on one task. It belongs to the task even when an external agent conversation happens to discuss multiple tasks. The Hub assigns its ID. The authenticated client token identifies the integration that published it; a vendor label or external conversation ID is optional provenance, not authorization identity.

```text
WorkSession
- id
- task_id
- created_by_principal
- created_by_client
- client_label optional
- source_session_reference optional
- started_at
- ended_at optional
- last_activity_at
- summary optional
- resumed_from_handoff_id optional
```

Each new work session can point to the handoff it resumed from. Its entries are append-only and ordered by a server-assigned sequence. The first entry type is an existing checkpoint or handoff. A session may later include an explicitly submitted decision or note. A client can append a handoff checkpoint before closing a session with a final summary; an abruptly interrupted session remains readable without pretending it closed cleanly.

The word **full** means all structured work records the client actually submitted for that session. Raw agent conversations are out of scope. The Hub cannot retrieve an agent product's private chat history by knowing its vendor name or session ID, and it does not store conversation transcripts.

Checkpoints written before session support have no session ID. The migration keeps them accessible as `pre-session history` rather than inventing session boundaries. Task cards and overviews expose `has_pre_session_history` so the first stored task does not appear empty. New clients attach a Hub session ID to checkpoint writes; omitting it remains supported for existing clients.

## Write path and read implementation

The write path creates a work session for a task, appends checkpoints to it, and optionally closes it with a summary. The application module owns the authorization and invariants: the session and checkpoint must belong to the same task, the token must be allowed in the task's space, and optimistic task versions and idempotency still apply.

In PostgreSQL, new checkpoints have a nullable `session_id` foreign key. A `session_entries` table gives each stored item a sequence within its session; checkpoint entries refer to the existing checkpoint row instead of copying its content. Later entry types can carry their own bounded payloads. Appending a checkpoint, adding its session entry, and advancing task and session activity happen in one transaction.

The read interface is a small set of user-intent queries: `ListActiveWork`, `GetTaskOverview`, `ListTaskSessions`, and `GetSessionHistory`. A PostgreSQL adapter can assemble list and overview projections with joins and indexed lookups; clients do not need to reconstruct them through many entity calls. This keeps the session/indexing implementation behind the work-continuity module's interface.

B-tree indexes cover task activity, `(task_id, last_activity_at DESC, id DESC)` for sessions, and `(session_id, sequence)` for entries. Cursor pagination covers unbounded session and legacy history. Full-text search is a separate feature and only needs an index when searching session content is actually offered; a vector database is unnecessary for this navigation flow.

## Future compact view

An agent may later publish a `SummarySnapshot` for a contiguous range of session entries. The agent writes the summary; the Hub authenticates the request, validates its shape and source references, stores it, and returns it on request. The Hub never invokes an LLM or launches a summarization job. A snapshot is a derived, versioned view, never a replacement for the entries it covers. The Hub keeps the original checkpoints, handoffs, decisions, and notes immutable and fetchable. Replacing a poor summary creates a new snapshot that supersedes the old snapshot without changing its sources.

```text
SummarySnapshot
- id, session_id, version
- first_entry_sequence, last_entry_sequence
- summary
- key_facts with source entry references
- open_actions, warnings, and uncertainties
- created_by_client, created_at
- supersedes_id optional
```

A compact response can contain the latest valid snapshot plus entries added after its covered range. It must expose that range and source references so a client can expand any claim into the original record. Decisions, unresolved actions, warnings, and the latest handoff remain separately visible in task overviews and bootstrap responses; compaction must not make them disappear because they were omitted from prose. The server can validate coverage and references but cannot prove that an agent's summary preserved every important meaning. Clients should label the snapshot as derived and stale when new entries fall outside its range.

This feature is deferred until session histories are long enough to justify it. The first discovery release reads original records and existing checkpoint summaries directly.

## Acceptance example

1. A fresh authenticated client lists active tasks without knowing project or task IDs.
2. It selects a task and sees the current objective, latest handoff, and a bounded session index.
3. It selects a session and pages through every stored entry in order.
4. A token for another space cannot discover the task or its sessions.
5. The existing first task and its checkpoints remain readable after migration as pre-session history.

The local Hub implements this discovery and session flow. Cross-computer agent integration and public deployment still need separate verification.
