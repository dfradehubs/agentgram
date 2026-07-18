"use client";

import { useState } from "react";
import type { AdminLLMProvider } from "@/lib/types";
import { Button } from "@/components/ui/button";

interface ProviderFormProps {
  provider?: AdminLLMProvider;
  onSave: (provider: Partial<AdminLLMProvider>) => void;
  onCancel: () => void;
}

export function ProviderForm({ provider, onSave, onCancel }: ProviderFormProps) {
  const [form, setForm] = useState({
    id: provider?.id || "",
    name: provider?.name || "",
    provider_type: provider?.provider_type || ("openai" as AdminLLMProvider["provider_type"]),
    api_key: "",
    endpoint: provider?.endpoint || "",
    enabled: provider?.enabled ?? true,
    clear_api_key: false,
  });
  const custom = form.provider_type === "custom";

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    onSave({ ...form, endpoint: custom ? form.endpoint : "" });
  };

  return (
    <form onSubmit={submit} className="max-w-2xl space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 block text-sm font-medium">ID</label>
          <input className="w-full rounded-md border bg-background px-3 py-2 text-sm" value={form.id}
            onChange={(e) => setForm({ ...form, id: e.target.value })} disabled={!!provider} required />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">Name</label>
          <input className="w-full rounded-md border bg-background px-3 py-2 text-sm" value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })} required placeholder="OpenAI Production" />
        </div>
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium">Provider type</label>
        <select className="w-full rounded-md border bg-background px-3 py-2 text-sm" value={form.provider_type}
          onChange={(e) => setForm({ ...form, provider_type: e.target.value as AdminLLMProvider["provider_type"], clear_api_key: false })}>
          <option value="anthropic">Anthropic</option>
          <option value="google">Google</option>
          <option value="openai">OpenAI</option>
          <option value="custom">Custom Endpoint (OpenAI-compatible)</option>
        </select>
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium">API Key {custom && "(optional)"}</label>
        <input type="password" className="w-full rounded-md border bg-background px-3 py-2 text-sm" value={form.api_key}
          onChange={(e) => setForm({ ...form, api_key: e.target.value })} required={!provider && !custom}
          placeholder={provider ? "Leave empty to keep the current key" : custom ? "Optional" : "Required"} />
        {provider && custom && provider.api_key && (
          <label className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
            <input type="checkbox" checked={form.clear_api_key}
              onChange={(e) => setForm({ ...form, clear_api_key: e.target.checked, api_key: e.target.checked ? "" : form.api_key })} />
            Remove the stored API key
          </label>
        )}
      </div>
      {custom && (
        <div>
          <label className="mb-1 block text-sm font-medium">Chat Completions endpoint</label>
          <input className="w-full rounded-md border bg-background px-3 py-2 text-sm" value={form.endpoint}
            onChange={(e) => setForm({ ...form, endpoint: e.target.value })} required
            placeholder="https://gateway.example.com/v1/chat/completions" />
          <p className="mt-1 text-xs text-muted-foreground">Must implement the OpenAI Chat Completions API.</p>
        </div>
      )}
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> Enabled
      </label>
      <div className="flex gap-2 pt-4">
        <Button type="submit">{provider ? "Save changes" : "Create provider"}</Button>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </form>
  );
}
