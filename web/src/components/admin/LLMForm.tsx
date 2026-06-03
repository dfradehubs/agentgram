"use client";

import { useState } from "react";
import type { AdminLLMModel } from "@/lib/types";
import { Button } from "@/components/ui/button";

interface LLMFormProps {
  model?: AdminLLMModel;
  onSave: (model: Partial<AdminLLMModel>) => void;
  onCancel: () => void;
}

export function LLMForm({ model, onSave, onCancel }: LLMFormProps) {
  const [form, setForm] = useState({
    id: model?.id || "",
    name: model?.name || "",
    provider: model?.provider || "anthropic",
    model_id: model?.model || "",
    api_key: model?.api_key || "",
    role: model?.role || "chat",
    enabled: model?.enabled ?? true,
    is_default: model?.is_default ?? false,
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      id: form.id,
      name: form.name,
      provider: form.provider,
      model: form.model_id,
      api_key: form.api_key,
      role: form.role,
      enabled: form.enabled,
      is_default: form.is_default,
    });
  };

  const update = (field: string, value: string | boolean) =>
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
            value={form.provider}
            onChange={e => update("provider", e.target.value)}
          >
            <option value="anthropic">Anthropic</option>
            <option value="openai">OpenAI</option>
            <option value="google">Google</option>
          </select>
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

      <div>
        <label className="mb-1 block text-sm font-medium">API Key</label>
        <input
          type="password"
          className="w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.api_key}
          onChange={e => update("api_key", e.target.value)}
          required={!model}
          placeholder={model ? "Leave empty to keep the current one" : "sk-..."}
        />
        {model && (
          <p className="mt-1 text-xs text-muted-foreground">
            Leave the field empty to keep the current API key
          </p>
        )}
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

      <div className="flex gap-2 pt-4">
        <Button type="submit">{model ? "Save changes" : "Create model"}</Button>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </form>
  );
}
