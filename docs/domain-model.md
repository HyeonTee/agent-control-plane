# Domain model

## Spaces and projects

A `Space` is a data and authorization boundary. It can represent personal work, a company, or another context that must not be mixed with others.

```text
Space
- id
- name
- default_sync_policy
- created_at
```

A `Project` is a logical body of work. It may refer to a repository, but the Hub does not own or clone that repository.

```text
Project
- id
- space_id
- name
- description
- repository_reference optional
- default_branch optional
- sync_policy
- created_at
- updated_at
```

Local paths are deliberately absent. A path belongs to a device-local workspace binding managed by `ctx`, not to the shared project.

## Task

```text
Task
- id
- project_id
- title
- objective
- status
- priority
- version
- claimed_by optional
- lease_expires_at optional
- base_revision optional
- created_at
- updated_at
- completed_at optional
```

Statuses:

```text
todo
in_progress
blocked
completed
cancelled
```

The version is used for optimistic concurrency. A client must supply the version it observed when making a conflicting update.

## Checkpoint and handoff

A checkpoint is an immutable entry in a task timeline.

```text
Checkpoint
- id
- task_id
- kind
- summary
- completed
- remaining
- warnings
- changed_paths
- test_results
- source_revision optional
- client_session_id optional
- created_by
- created_at
```

Kinds:

```text
progress
handoff
```

A handoff is a checkpoint intended to initialize the next session. It should contain a concrete next action and enough provenance to detect when the workspace has moved since it was created.

Checkpoints are appended, never edited in place. Corrections create a new checkpoint that supersedes an earlier one.

## Decisions and documents

```text
Decision
- id
- project_id
- task_id optional
- title
- decision
- rationale
- source
- supersedes_id optional
- created_by
- created_at
```

```text
ContextDocument
- id
- space_id
- project_id optional
- kind
- trust_level
- title
- content
- source
- content_digest
- schema_version
- created_at
- updated_at
```

Document kinds include instruction, architecture, domain, runbook, reference, and note. Only documents explicitly classified as instructions can contribute behavioral rules to an agent.

## Rules

```text
Rule
- id
- space_id
- project_id optional
- title
- content
- severity
- priority
- enabled
- created_at
- updated_at
```

Project rules may narrow a global rule but cannot silently override a higher-priority prohibition. Rule conflict resolution must be deterministic and included in tests.

## Skills

A skill is a stable identity. A skill version is an immutable published package.

```text
Skill
- id
- space_id
- name
- description
- latest_version optional
- created_at
```

```text
SkillVersion
- skill_id
- version
- manifest
- artifact_key
- artifact_digest
- artifact_size
- published_by
- published_at
```

The canonical package is vendor neutral and may contain instructions, scripts, templates, and references. Vendor adapters transform or install it locally without changing the stored source package.

Publishing the same `(skill_id, version)` with different content is rejected.

## Workflows

A workflow describes repeatable steps and their input/output contracts. It does not execute agents on the Hub.

```text
Workflow
- id
- space_id
- name
- description
- latest_version optional
```

```text
WorkflowVersion
- workflow_id
- version
- definition
- artifact_digest optional
- published_by
- published_at
```

Execution remains local and user-controlled.

## Agent profile

An agent profile is a role bundle, not a running agent and not an authenticated principal.

```text
AgentProfile
- id
- space_id
- name
- role
- recommended_skills
- workflow_rules
- output_contract optional
```

Selecting a profile can only narrow the caller's permissions. It can never grant access that the authenticated principal does not already have.

## Observation

An observation is a client-submitted fact measured outside the Hub.

```text
Observation
- id
- project_id
- kind
- schema_version
- source_device_id
- observed_at
- expires_at optional
- payload
- payload_digest
- created_at
```

Examples include Git revision, working-tree state, test results, or a sanitized runtime summary. Raw credentials and prohibited project data are not observations.

## Context pack

A `ContextPack` is assembled for a particular request. It is not a source of truth.

```text
ContextPack
- schema_version
- generated_at
- project
- selected_task optional
- effective_rules
- latest_handoff optional
- recent_checkpoints
- relevant_decisions
- skill_references
- allowed_observations
- freshness
- source_versions
```

The response is bounded by item counts and content size. Large documents and artifacts are returned as references that the client can fetch explicitly.
