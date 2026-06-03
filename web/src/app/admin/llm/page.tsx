"use client";

import { useEffect, useState, useCallback } from "react";
import type { AdminLLMModel } from "@/lib/types";
import { getAdminLLMs, deleteAdminLLM, createAdminLLM, updateAdminLLM } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { LLMForm } from "@/components/admin/LLMForm";
import { Plus, Pencil, Trash2, ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

const roleLabels: Record<string, string> = {
  chat: "Chat",
  summarizer: "Summarizer",
  file_processor: "File Processor",
  chart_extractor: "Chart Extractor",
  session_namer: "Session Namer",
};

export default function AdminLLMPage() {
  const [models, setModels] = useState<AdminLLMModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingModel, setEditingModel] = useState<AdminLLMModel | null>(null);
  const [creatingModel, setCreatingModel] = useState(false);

  const fetchModels = useCallback(async () => {
    try {
      const data = await getAdminLLMs();
      setModels(data.sort((a, b) => a.name.localeCompare(b.name)));
    } catch {
      toast.error("Error loading LLM models");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchModels();
  }, [fetchModels]);

  const handleDelete = async (id: string) => {
    if (!confirm(`Delete LLM model "${id}"?`)) return;
    try {
      await deleteAdminLLM(id);
      toast.success("LLM model deleted");
      fetchModels();
    } catch {
      toast.error("Error deleting LLM model");
    }
  };

  const handleSave = async (model: Partial<AdminLLMModel>) => {
    try {
      if (editingModel) {
        await updateAdminLLM(editingModel.id, model);
        toast.success("LLM model updated");
      } else {
        await createAdminLLM(model);
        toast.success("LLM model created");
      }
      setEditingModel(null);
      setCreatingModel(false);
      fetchModels();
    } catch {
      toast.error("Error saving LLM model");
    }
  };

  if (creatingModel || editingModel) {
    return (
      <div>
        <button
          onClick={() => { setEditingModel(null); setCreatingModel(false); }}
          className="mb-4 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to LLM models
        </button>
        <h1 className="mb-6 text-2xl font-bold">
          {editingModel ? `Edit: ${editingModel.name}` : "New LLM model"}
        </h1>
        <LLMForm
          model={editingModel || undefined}
          onSave={handleSave}
          onCancel={() => { setEditingModel(null); setCreatingModel(false); }}
        />
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">LLM Models</h1>
        <Button onClick={() => setCreatingModel(true)} size="sm">
          <Plus className="mr-1 h-4 w-4" />
          New model
        </Button>
      </div>

      {loading ? (
        <AdminTableSkeleton columns={5} rows={3} />
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="px-4 py-3 text-left font-medium">ID</th>
                <th className="px-4 py-3 text-left font-medium">Name</th>
                <th className="px-4 py-3 text-left font-medium">Provider</th>
                <th className="px-4 py-3 text-left font-medium">Model</th>
                <th className="px-4 py-3 text-left font-medium">Role</th>
                <th className="px-4 py-3 text-left font-medium">Status</th>
                <th className="px-4 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {models.map((model) => (
                <tr key={model.id} className="hover:bg-muted/30">
                  <td className="px-4 py-3 font-mono text-xs">{model.id}</td>
                  <td className="px-4 py-3">
                    {model.name}
                    {model.is_default && (
                      <span className="ml-2 rounded bg-primary/10 px-1.5 py-0.5 text-xs text-primary">default</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <span className="rounded bg-muted px-2 py-0.5 text-xs">{model.provider}</span>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">{model.model}</td>
                  <td className="px-4 py-3">
                    <span className="rounded bg-muted px-2 py-0.5 text-xs">{roleLabels[model.role] || model.role}</span>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`rounded px-2 py-0.5 text-xs ${model.enabled ? "bg-green-500/10 text-green-600" : "bg-red-500/10 text-red-600"}`}>
                      {model.enabled ? "Active" : "Inactive"}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setEditingModel(model)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => handleDelete(model.id)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {models.length === 0 && (
            <div className="px-4 py-8 text-center text-muted-foreground">No LLM models configured</div>
          )}
        </div>
      )}
    </div>
  );
}
