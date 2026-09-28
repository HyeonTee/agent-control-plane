# Production cross-agent handoff

The first Agent Control Plane work task is stored at `https://agent.gwinam.com` in the `personal` space and `agent-control-plane` project. Its task ID is `d76adfcd-bc0b-4399-82b8-2a562344f847`. The latest handoff is checkpoint `f7f5d40d-5355-411d-ac33-a80afb4ccb69` at task version 9. Four sessions contain eight structured checkpoint entries: the first two sessions replay the original local records, and the later Codex and Claude Code sessions document production deployment and continuation. Original local timestamps and source references are retained in the imported summaries. Production IDs and write timestamps differ from the local rows. No raw conversation transcript or local token hash was imported.

Any HTTP-capable agent can resume this task without checking out the repository or installing a dedicated CLI. Give each installation its own scoped token from a secure credential store, then follow this sequence:

```sh
curl -fsS -H "Authorization: Bearer $HUB_TOKEN" \
  'https://agent.gwinam.com/api/v1/tasks?status=active&limit=20'
curl -fsS -H "Authorization: Bearer $HUB_TOKEN" \
  'https://agent.gwinam.com/api/v1/tasks/d76adfcd-bc0b-4399-82b8-2a562344f847'
curl -fsS -H "Authorization: Bearer $HUB_TOKEN" \
  'https://agent.gwinam.com/api/v1/tasks/d76adfcd-bc0b-4399-82b8-2a562344f847/sessions?limit=20'
curl -fsS -H "Authorization: Bearer $HUB_TOKEN" \
  'https://agent.gwinam.com/api/v1/tasks/d76adfcd-bc0b-4399-82b8-2a562344f847/sessions/SESSION_ID/entries?limit=50'
```

Use the `next_cursor` returned by a list response to fetch later pages. Read the latest handoff and its warnings in the task overview, then inspect the relevant session entries before acting. Treat old `remaining` lists as historical; the latest handoff supersedes completed items. The Hub serves client-authored records and never runs an agent or summarizes them itself.

On 2026-09-28, Claude Code used an independent read-only token to discover the task, inspect its overview and session index, and read current and older entries. A write with that token returned HTTP 403. A separate one-day writer token then let Claude author checkpoint `33ae5d10-d161-428e-80ab-a41e1b40cb0c` in its own closed session; that writer token was revoked, and a later request returned HTTP 401. Both agents ran on the same physical computer, so a second-computer network and credential setup has not yet been tested.

The owner API token and 30-day Claude read token are stored as `SecureString` parameters `/operator/agent-control-plane/owner_api_token` and `/operator/agent-control-plane/claude_read_token` in the production AWS account. Their values are not in this repository or the Hub records. Only an administrator should retrieve one, move it into the target integration's credential store, and rotate or revoke it when appropriate. The EC2 instance role can read `/agent-control-plane/*` deployment parameters but not this `/operator/` prefix.

The latest S3 archive, `backups/hub-20260928T011544Z.dump`, restored into a disposable PostgreSQL database with the same live counts: two migrations, one task, eight checkpoints, four sessions, and eight entries. It is encrypted with SSE-S3. Scheduled backups and restore checks remain enabled; a full EC2 rebuild drill is still separate work.
