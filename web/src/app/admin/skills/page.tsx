"use client";

import { useEffect, useState, useCallback } from "react";
import type { AdminSkill } from "@/lib/types";
import { getAdminSkills, deleteAdminSkill, createAdminSkill, updateAdminSkill } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { SkillForm } from "@/components/admin/SkillForm";
import { Plus, Pencil, Trash2, ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

export default function AdminSkillsPage() {
  const [skills, setSkills] = useState<AdminSkill[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingSkill, setEditingSkill] = useState<AdminSkill | null>(null);
  const [creatingSkill, setCreatingSkill] = useState(false);

  const fetchSkills = useCallback(async () => {
    try {
      const data = await getAdminSkills();
      setSkills(data.sort((a, b) => a.name.localeCompare(b.name)));
    } catch {
      toast.error("Error loading skills");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSkills();
  }, [fetchSkills]);

  const handleDelete = async (id: string) => {
    if (!confirm(`Delete skill "${id}"?`)) return;
    try {
      await deleteAdminSkill(id);
      toast.success("Skill deleted");
      fetchSkills();
    } catch {
      toast.error("Error deleting skill");
    }
  };

  const handleSave = async (skill: Partial<AdminSkill>) => {
    try {
      if (editingSkill) {
        await updateAdminSkill(editingSkill.id, skill);
        toast.success("Skill updated");
      } else {
        await createAdminSkill(skill);
        toast.success("Skill created");
      }
      setEditingSkill(null);
      setCreatingSkill(false);
      fetchSkills();
    } catch {
      toast.error("Error saving skill");
    }
  };

  if (creatingSkill || editingSkill) {
    return (
      <div>
        <button
          onClick={() => { setEditingSkill(null); setCreatingSkill(false); }}
          className="mb-4 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to skills
        </button>
        <h1 className="mb-6 text-2xl font-bold">
          {editingSkill ? `Edit: ${editingSkill.name}` : "New skill"}
        </h1>
        <SkillForm
          skill={editingSkill || undefined}
          onSave={handleSave}
          onCancel={() => { setEditingSkill(null); setCreatingSkill(false); }}
        />
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Skills</h1>
        <Button onClick={() => setCreatingSkill(true)} size="sm">
          <Plus className="mr-1 h-4 w-4" />
          New skill
        </Button>
      </div>

      {loading ? (
        <AdminTableSkeleton columns={4} rows={3} />
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="px-4 py-3 text-left font-medium">ID</th>
                <th className="px-4 py-3 text-left font-medium">Name</th>
                <th className="px-4 py-3 text-left font-medium">Description</th>
                <th className="px-4 py-3 text-left font-medium">Permissions</th>
                <th className="px-4 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {skills.map((skill) => (
                <tr key={skill.id} className="hover:bg-muted/30">
                  <td className="px-4 py-3 font-mono text-xs">{skill.id}</td>
                  <td className="px-4 py-3">{skill.name}</td>
                  <td className="max-w-xs truncate px-4 py-3 text-xs text-muted-foreground">{skill.description}</td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {skill.allowed_users?.includes("*") ? "All" : `${(skill.allowed_users?.length || 0)} users, ${(skill.allowed_groups?.length || 0)} groups`}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setEditingSkill(skill)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => handleDelete(skill.id)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {skills.length === 0 && (
            <div className="px-4 py-8 text-center text-muted-foreground">No skills configured</div>
          )}
        </div>
      )}
    </div>
  );
}
