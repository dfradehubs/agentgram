"use client";

import { useState, useEffect } from "react";
import type { AdminGroup, AdminAgent } from "@/lib/types";
import { getAdminAgents } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { TagInput } from "./TagInput";
import { Bot, Check } from "lucide-react";
import { getEntityColor } from "@/lib/agent-colors";

interface GroupFormProps {
  group?: AdminGroup;
  onSave: (group: Partial<AdminGroup>) => void;
  onCancel: () => void;
}

export function GroupForm({ group, onSave, onCancel }: GroupFormProps) {
  const [agents, setAgents] = useState<AdminAgent[]>([]);
  const [form, setForm] = useState({
    id: group?.id || "",
    name: group?.name || "",
    description: group?.description || "",
    agent_ids: group?.agent_ids || [],
    allowed_users: group?.allowed_users || [],
    allowed_groups: group?.allowed_groups || [],
    max_turns: group?.max_turns ?? 0,
  });

  useEffect(() => {
    getAdminAgents().then(setAgents).catch(() => {});
  }, []);

  const toggleAgent = (id: string) => {
    setForm((prev) => ({
      ...prev,
      agent_ids: prev.agent_ids.includes(id)
        ? prev.agent_ids.filter((x) => x !== id)
        : [...prev.agent_ids, id],
    }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      id: form.id,
      name: form.name,
      description: form.description,
      agent_ids: form.agent_ids,
      allowed_users: form.allowed_users,
      allowed_groups: form.allowed_groups,
      max_turns: form.max_turns,
    });
  };

  return (
    <form onSubmit={handleSubmit} className="max-w-2xl space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 block text-sm font-medium">ID</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.id}
            onChange={(e) => setForm((prev) => ({ ...prev, id: e.target.value }))}
            disabled={!!group}
            pattern="[A-Za-z0-9][A-Za-z0-9_-]{0,55}"
            maxLength={56}
            title="Start with a letter or digit; use only ASCII letters, digits, underscores, or hyphens (max 56)."
            required
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">Name</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.name}
            onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
            required
          />
        </div>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">Description</label>
        <textarea
          className="min-h-24 w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.description}
          onChange={(e) => setForm((prev) => ({ ...prev, description: e.target.value }))}
          maxLength={2000}
          placeholder="Explain what this group contributes and when an MCP client should use it."
        />
        <p className="mt-1 text-xs text-muted-foreground">
          This purpose is included in the group MCP tool description so clients can decide when to call it.
        </p>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">
          Members ({form.agent_ids.length} selected, min. 2)
        </label>
        <div className="max-h-48 space-y-1 overflow-y-auto rounded-md border p-2">
          {agents.map((agent) => {
            const selected = form.agent_ids.includes(agent.id);
            const color = getEntityColor(agent.id);
            return (
              <button
                key={agent.id}
                type="button"
                onClick={() => toggleAgent(agent.id)}
                className={`flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                  selected ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
                }`}
              >
                <div
                  className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-white"
                  style={{ background: `linear-gradient(135deg, ${color.avatarFrom}, ${color.avatarTo})` }}
                >
                  <Bot className="h-3 w-3" />
                </div>
                <span className="flex-1 truncate">{agent.name}</span>
                <span className="text-xs text-muted-foreground">{agent.id}</span>
                {selected && <Check className="h-4 w-4 text-emerald-500" />}
              </button>
            );
          })}
        </div>
        {form.agent_ids.length < 2 && (
          <p className="mt-1 text-xs text-destructive">Select at least 2 agents</p>
        )}
      </div>

      <TagInput
        label="Allowed users"
        values={form.allowed_users}
        onChange={(v) => setForm((prev) => ({ ...prev, allowed_users: v }))}
        placeholder="* for everyone, or emails"
      />

      <TagInput
        label="Allowed groups"
        values={form.allowed_groups}
        onChange={(v) => setForm((prev) => ({ ...prev, allowed_groups: v }))}
        placeholder="/google-workspace/group@example.com"
      />

      <div>
        <label className="mb-1 block text-sm font-medium">Max debate turns</label>
        <input
          type="number"
          min={0}
          max={50}
          className="w-32 rounded-md border bg-background px-3 py-2 text-sm"
          value={form.max_turns}
          onChange={(e) => setForm((prev) => ({ ...prev, max_turns: Math.max(0, parseInt(e.target.value, 10) || 0) }))}
        />
        <p className="mt-1 text-xs text-muted-foreground">
          Maximum number of turns. The same agent may be selected more than once. 0 = default (6 via API/web, 3 via MCP).
        </p>
      </div>

      <div className="flex gap-2 pt-4">
        <Button type="submit" disabled={form.agent_ids.length < 2}>
          {group ? "Save changes" : "Create group"}
        </Button>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
