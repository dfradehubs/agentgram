"use client";

import { useCallback, useEffect, useState } from "react";
import { ArrowLeft, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import type { AdminLLMProvider } from "@/lib/types";
import { createAdminLLMProvider, deleteAdminLLMProvider, getAdminLLMProviders, updateAdminLLMProvider } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ProviderForm } from "@/components/admin/ProviderForm";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

const typeLabels: Record<string, string> = { anthropic: "Anthropic", google: "Google", openai: "OpenAI", custom: "Custom Endpoint" };

export default function AdminProvidersPage() {
  const [providers, setProviders] = useState<AdminLLMProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<AdminLLMProvider | null>(null);
  const [creating, setCreating] = useState(false);
  const load = useCallback(async () => {
    try { setProviders(await getAdminLLMProviders()); }
    catch { toast.error("Error loading providers"); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { load(); }, [load]);

  const save = async (value: Partial<AdminLLMProvider>) => {
    try {
      if (editing) await updateAdminLLMProvider(editing.id, value);
      else await createAdminLLMProvider(value);
      toast.success(editing ? "Provider updated" : "Provider created");
      setEditing(null); setCreating(false); await load();
    } catch (error) { toast.error(error instanceof Error ? error.message : "Error saving provider"); }
  };
  const remove = async (provider: AdminLLMProvider) => {
    if (!confirm(`Delete provider "${provider.name}"?`)) return;
    try { await deleteAdminLLMProvider(provider.id); toast.success("Provider deleted"); await load(); }
    catch (error) { toast.error(error instanceof Error ? error.message : "Provider is still in use"); }
  };

  if (creating || editing) return <div>
    <button onClick={() => { setCreating(false); setEditing(null); }} className="mb-4 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
      <ArrowLeft className="h-4 w-4" /> Back to providers
    </button>
    <h1 className="mb-6 text-2xl font-bold">{editing ? `Edit: ${editing.name}` : "New provider"}</h1>
    <ProviderForm provider={editing || undefined} onSave={save} onCancel={() => { setCreating(false); setEditing(null); }} />
  </div>;

  return <div>
    <div className="mb-6 flex items-center justify-between">
      <div><h1 className="text-2xl font-bold">Providers</h1><p className="mt-1 text-sm text-muted-foreground">Share credentials across multiple LLM models.</p></div>
      <Button onClick={() => setCreating(true)} size="sm"><Plus className="mr-1 h-4 w-4" />New provider</Button>
    </div>
    {loading ? <AdminTableSkeleton columns={5} rows={3} /> : <div className="overflow-x-auto rounded-lg border">
      <table className="w-full text-sm"><thead className="bg-muted/50"><tr>
        <th className="px-4 py-3 text-left font-medium">Name</th><th className="px-4 py-3 text-left font-medium">Type</th>
        <th className="px-4 py-3 text-left font-medium">API key</th><th className="px-4 py-3 text-left font-medium">Status</th><th className="px-4 py-3 text-right font-medium">Actions</th>
      </tr></thead><tbody className="divide-y">{providers.map((provider) => <tr key={provider.id}>
        <td className="px-4 py-3"><div>{provider.name}</div><div className="font-mono text-xs text-muted-foreground">{provider.id}</div></td>
        <td className="px-4 py-3">{typeLabels[provider.provider_type] || provider.provider_type}</td>
        <td className="px-4 py-3 font-mono text-xs">{provider.api_key || "Not set"}</td>
        <td className="px-4 py-3">{provider.enabled ? "Active" : "Inactive"}</td>
        <td className="px-4 py-3 text-right"><Button variant="ghost" size="icon" onClick={() => setEditing(provider)}><Pencil className="h-4 w-4" /></Button>
          <Button variant="ghost" size="icon" className="text-destructive" onClick={() => remove(provider)}><Trash2 className="h-4 w-4" /></Button></td>
      </tr>)}</tbody></table>
      {providers.length === 0 && <div className="px-4 py-8 text-center text-muted-foreground">No providers configured</div>}
    </div>}
  </div>;
}
