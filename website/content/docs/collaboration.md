---
title: Sharing & collaboration
weight: 6
---

Agentgram is built for teams. Two features make conversations and agents collaborative — both fully
respecting [RBAC]({{< relref "agents" >}}#permissions-rbac).

## Share a conversation

Turn any of your sessions into a time-limited, revocable link so a teammate can read it — or clone it
into their own workspace and keep going.

| Action | Endpoint |
| ------ | -------- |
| Create a share link | `POST /api/agents/{agentId}/sessions/{sessionId}/share` |
| View it (read-only)  | `GET /api/shared/{token}` |
| Clone it into your sessions | `POST /api/shared/{token}/clone` |
| Revoke it | `DELETE /api/agents/{agentId}/sessions/{sessionId}/share` |

Creating a link accepts an optional body:

```json
{ "expires_in_hours": 168 }
```

`expires_in_hours` defaults to **168 (7 days)**, which is also the maximum. The response carries the
`token`, a ready-to-share `url`, and the `expires_at` timestamp. Anyone with the link gets a
**read-only** view of the conversation (and which agent produced it); cloning gives the recipient
their own editable copy to continue. Links **expire automatically** and can be **revoked** at any
time, so sharing stays under control.

## Shared multi-agent groups

A **group** bundles two or more agents into a single shared workspace that a set of users can use
together: ask the group a question, route it across its agents, and keep **shared sessions** that
everyone in the group can see.

| Action | Endpoint |
| ------ | -------- |
| List your groups | `GET /api/groups` |
| Create a group | `POST /api/groups` |
| Update / delete | `PUT` / `DELETE /api/groups/{groupId}` |
| List shared sessions | `GET /api/groups/{groupId}/sessions` |
| Add / remove a session | `POST` / `DELETE /api/groups/{groupId}/sessions/{sessionId}` |

Create a group with a name, **at least two agents**, and the people it's shared with:

```json
{
  "name": "Incident response",
  "agentIds": ["logs-agent", "metrics-agent", "kube-agent"],
  "allowed_users": ["teammate@example.com"],
  "allowed_groups": ["google-workspace/sre@example.com"]
}
```

You can only add agents **you** have access to (RBAC is enforced at creation), and the creator is
always a member. Share the group with individual teammates via `allowed_users` or with whole RBAC
groups via `allowed_groups`. Everyone who shares the group sees its shared sessions.

## Moderated group debates

Send a message **to the group itself** — like posting in a Telegram group — and an LLM **moderator**
decides which agents should answer, in sequence. Each agent sees the previous agents' replies through
the shared session transcript, so they can build on (or verify) each other's contributions before the
conversation comes back to you.

```
POST /api/groups/{groupId}/chat
```

```json
{
  "messages": [{ "role": "user", "content": "Is checkout healthy right now?" }],
  "session_id": "optional — continue an existing group session",
  "agent_ids": ["logs-agent", "metrics-agent"]
}
```

- The response is a **single SSE stream** (one `RUN_STARTED` / `RUN_FINISHED` pair). Every
  `TEXT_MESSAGE_*` and `TOOL_CALL_*` event carries an `agentId`, so clients render each agent's turn
  separately; `CUSTOM` events with subtype `moderator.select` announce whose turn it is.
- `agent_ids` is an optional **@mention**: it restricts the roster the moderator can pick from.
  Omit it and the moderator chooses freely among the group's agents you have access to.
- The moderator stops as soon as nobody else would add value (bounded to a handful of turns), and
  when several agents contributed it can append a short **synthesis** as the special `moderator`
  speaker.
- A failed agent turn doesn't kill the debate — it's reported as a scoped `turn.error` event and the
  moderator moves on.

**Setup:** the moderator is an LLM model with the role `moderator` (Admin → LLM Models). Any
provider works; the routing quality comes from your agents' **descriptions**, so keep them accurate.
The same debate is exposed on the [MCP endpoint]({{< relref "mcp" >}}) as one `ask_group_<groupId>`
tool per group you can access.

---

Both features are available from the web UI (the sidebar groups conversations and offers
share / clone actions) and directly over the API. The full request and response shapes are in the
[API Reference](/agentgram/api/).
