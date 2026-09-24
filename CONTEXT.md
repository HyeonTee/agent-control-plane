# Agent Control Plane

Agent Control Plane is the long-term product. Its first module, Context Hub, preserves approved work context so different agent sessions can continue the same work.

## Language

**Agent Control Plane**:
The broader product that may eventually coordinate agent work through distinct modules.

**Context Hub**:
A passive module that holds and serves client-submitted work context. It does not initiate agent activity, perform project work, or author summaries.
_Avoid_: Agent memory, agent runner

**WorkSession**:
One continuous stretch of work on one task, independent of the agent product or conversation that performed it.
_Avoid_: Client token, agent conversation

**Checkpoint**:
An immutable record of progress and remaining work within a task.

**Handoff**:
A checkpoint intended to help a later work session continue the task.

**SummarySnapshot**:
An agent-authored, derived summary of a cited range of work records. Its source records remain authoritative and available.
_Avoid_: Compacted source, replacement history

**Agent Operations**:
A possible future module for coordinating agent assignments and run state within the broader product. It is distinct from Context Hub.

**Execution Plane**:
The environment where agents and tools actually perform project work.
