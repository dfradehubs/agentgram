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

---

Both features are available from the web UI (the sidebar groups conversations and offers
share / clone actions) and directly over the API. The full request and response shapes are in the
[API Reference](/agentgram/api/).
