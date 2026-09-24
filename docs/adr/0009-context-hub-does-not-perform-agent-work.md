# ADR 0009: Context Hub does not perform agent work

- Status: Accepted
- Date: 2026-09-24

Context Hub is the passive context module inside the broader Agent Control Plane, as established by ADR 0007. Agents or their integrations decide what work to do and author checkpoints, handoffs, decisions, observations, and any future compact summaries. The Hub may authenticate, authorize, validate, version, index, persist, and assemble bounded views of that submitted data; it does not initiate agent or project work, run an agent or project command, invoke an LLM, or generate summaries. Routine data operations such as migrations and backups do not perform project work. This keeps project execution and model credentials out of the context store. If the product later coordinates agents, that belongs to Agent Operations, with execution in a separate Execution Plane; it does not change the Context Hub's responsibility.
