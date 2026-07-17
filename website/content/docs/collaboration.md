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

## Agent groups

A **group** bundles two or more agents into one conversational surface. Groups are **created by
administrators** — like agents and MCP servers, they're platform configuration, not per-user
artifacts — and each user sees, in their sidebar, only the groups they're allowed to use.

| Action | Endpoint |
| ------ | -------- |
| List the groups you can use | `GET /api/groups` |
| List your sessions in a group | `GET /api/groups/{groupId}/sessions` |
| Create / edit / delete a group (admin) | `POST` / `PUT` / `DELETE /api/admin/groups[/{id}]` |

Admins create a group from the **admin panel** (Agents → Groups) with a name, **at least two
agents**, and who it's shared with:

```json
{
  "id": "incident-response",
  "name": "Incident response",
  "agent_ids": ["logs-agent", "metrics-agent", "kube-agent"],
  "allowed_users": ["oncall@example.com"],
  "allowed_groups": ["google-workspace/sre@example.com"]
}
```

Grant access to individual users via `allowed_users` (or `*` for everyone) and to whole RBAC groups
via `allowed_groups`. **Sessions are personal**: each member has their own private conversations in
the group — you never see anyone else's.

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
- `agent_ids` optionally restricts the roster the moderator can pick from. In the web UI you don't
  pick agents manually — just type; to steer the moderator, **@mention** agents inline
  (`@logs-agent how's checkout?`) and only those answer. Omit it and the moderator chooses freely.
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

Conversation sharing is available from the web UI and over the API; groups are managed from the
admin panel and used from every user's sidebar. The full request and response shapes are in the
[API Reference](/api/).
