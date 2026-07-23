"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import type { AdminLLMModel, AdminLLMProvider } from "@/lib/types";
import { getAdminLLMProviders } from "@/lib/api";
import { Button } from "@/components/ui/button";

interface LLMFormProps {
  model?: AdminLLMModel;
  onSave: (model: Partial<AdminLLMModel>) => void;
  onCancel: () => void;
}

export function LLMForm({ model, onSave, onCancel }: LLMFormProps) {
  const [providers, setProviders] = useState<AdminLLMProvider[]>([]);
  const [form, setForm] = useState({
    id: model?.id || "",
    name: model?.name || "",
    provider_id: model?.provider_id || "",
    model_id: model?.model || "",
    role: model?.role || "chat",
    enabled: model?.enabled ?? true,
    is_default: model?.is_default ?? false,
    max_tokens: model?.max_tokens ?? 0,
  });

  useEffect(() => {
    getAdminLLMProviders().then((items) => {
      setProviders(items);
      setForm((current) => current.provider_id || items.length === 0
        ? current
        : { ...current, provider_id: items[0].id });
    }).catch(() => setProviders([]));
  }, []);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      id: form.id,
      name: form.name,
      provider_id: form.provider_id,
      model: form.model_id,
      role: form.role,
      enabled: form.enabled,
      is_default: form.is_default,
      max_tokens: form.max_tokens,
    });
  };

  const update = (field: string, value: string | boolean | number) =>
    setForm(prev => ({ ...prev, [field]: value }));

  return (
    <form onSubmit={handleSubmit} className="max-w-2xl space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 block text-sm font-medium">ID</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.id}
            onChange={e => update("id", e.target.value)}
            disabled={!!model}
            required
            placeholder="claude-sonnet"
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">Name</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.name}
            onChange={e => update("name", e.target.value)}
            required
            placeholder="Claude Sonnet"
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 block text-sm font-medium">Provider</label>
          <select
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.provider_id}
            onChange={e => update("provider_id", e.target.value)}
            required
          >
            <option value="" disabled>Select a provider</option>
            {providers.filter((provider) => provider.enabled || provider.id === model?.provider_id).map((provider) => (
              <option key={provider.id} value={provider.id}>{provider.name} ({provider.provider_type})</option>
            ))}
          </select>
          {providers.length === 0 && <p className="mt-1 text-xs text-muted-foreground">Create a <Link className="underline" href="/admin/providers">Provider</Link> first.</p>}
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">Model</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.model_id}
            onChange={e => update("model_id", e.target.value)}
            required
            placeholder="claude-sonnet-4-20250514"
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 block text-sm font-medium">Role</label>
          <select
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.role}
            onChange={e => update("role", e.target.value)}
          >
            <option value="chat">Chat</option>
            <option value="summarizer">Summarizer</option>
            <option value="file_processor">File Processor</option>
            <option value="chart_extractor">Chart Extractor</option>
            <option value="session_namer">Session Namer</option>
            <option value="moderator">Group Moderator</option>
          </select>
        </div>
        <div className="flex flex-col justify-end gap-3 pb-1">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={e => update("enabled", e.target.checked)}
              className="rounded"
            />
            Enabled
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={form.is_default}
              onChange={e => update("is_default", e.target.checked)}
              className="rounded"
            />
            Default model
          </label>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 block text-sm font-medium">Max tokens</label>
          <input
            type="number"
            min={0}
            max={200000}
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.max_tokens}
            onChange={e => update("max_tokens", e.target.valueAsNumber || 0)}
            placeholder="0"
          />
          <p className="mt-1 text-xs text-muted-foreground">
            Output token cap. 0 = auto (per-role default). Raise for reasoning/thinking models (e.g. the group moderator) that would otherwise return empty.
          </p>
        </div>
      </div>

      <div className="flex gap-2 pt-4">
        <Button type="submit" disabled={providers.length === 0}>{model ? "Save changes" : "Create model"}</Button>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </form>
  );
}
