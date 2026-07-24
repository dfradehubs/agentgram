"use client";

import { useState } from "react";
import type { AdminSkill } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { TagInput } from "./TagInput";

interface SkillFormProps {
  skill?: AdminSkill;
  onSave: (skill: Partial<AdminSkill>) => void;
  onCancel: () => void;
}

export function SkillForm({ skill, onSave, onCancel }: SkillFormProps) {
  const [form, setForm] = useState({
    id: skill?.id || "",
    name: skill?.name || "",
    description: skill?.description || "",
    content: skill?.content || "",
    allowed_users: skill?.allowed_users || [],
    allowed_groups: skill?.allowed_groups || [],
  });

  const update = (field: keyof typeof form, value: string) =>
    setForm(prev => ({ ...prev, [field]: value }));

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      id: form.id,
      name: form.name,
      description: form.description,
      content: form.content,
      allowed_users: form.allowed_users,
      allowed_groups: form.allowed_groups,
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
            onChange={e => update("id", e.target.value)}
            disabled={!!skill}
            placeholder="debug-prod"
            required
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">Name</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.name}
            onChange={e => update("name", e.target.value)}
            required
          />
        </div>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">Description</label>
        <textarea
          className="w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.description}
          onChange={e => update("description", e.target.value)}
          rows={2}
          placeholder="Shown to the agent so it knows when to load this skill"
        />
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">Content</label>
        <textarea
          className="w-full rounded-md border bg-background px-3 py-2 font-mono text-sm"
          value={form.content}
          onChange={e => update("content", e.target.value)}
          rows={16}
          placeholder="Markdown instructions returned verbatim when the skill tool is called"
          required
        />
      </div>

      <TagInput
        label="Allowed users"
        values={form.allowed_users}
        onChange={v => setForm(prev => ({ ...prev, allowed_users: v }))}
        placeholder="* for everyone, or emails"
      />

      <TagInput
        label="Allowed groups"
        values={form.allowed_groups}
        onChange={v => setForm(prev => ({ ...prev, allowed_groups: v }))}
        placeholder="/google-workspace/group@example.com"
      />

      <div className="flex gap-2 pt-4">
        <Button type="submit">{skill ? "Save changes" : "Create skill"}</Button>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </form>
  );
}
