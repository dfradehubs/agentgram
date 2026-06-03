"use client";

import { useEffect, useState, useCallback } from "react";
import type { AdminAgent, AdminGroup } from "@/lib/types";
import {
  getAdminAgents, deleteAdminAgent, createAdminAgent, updateAdminAgent, updateAdminAgentPermissions,
  getAdminGroups, createAdminGroup, updateAdminGroup, deleteAdminGroup, updateAdminGroupPermissions,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { AgentForm } from "@/components/admin/AgentForm";
import { GroupForm } from "@/components/admin/GroupForm";
import { Plus, Pencil, Trash2, ArrowLeft, Eye } from "lucide-react";
import Link from "next/link";
import { toast } from "sonner";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

type Tab = "agents" | "groups";

export default function AdminAgentsPage() {
  const [tab, setTab] = useState<Tab>("agents");

  // Agents state
  const [agents, setAgents] = useState<AdminAgent[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(true);
  const [editingAgent, setEditingAgent] = useState<AdminAgent | null>(null);
  const [creatingAgent, setCreatingAgent] = useState(false);

  // Groups state
  const [groups, setGroups] = useState<AdminGroup[]>([]);
  const [loadingGroups, setLoadingGroups] = useState(true);
  const [editingGroup, setEditingGroup] = useState<AdminGroup | null>(null);
  const [creatingGroup, setCreatingGroup] = useState(false);

  const fetchAgents = useCallback(async () => {
    try {
      const data = await getAdminAgents();
      setAgents(data.sort((a, b) => a.name.localeCompare(b.name)));
    } catch {
      toast.error("Error loading agents");
    } finally {
      setLoadingAgents(false);
    }
  }, []);

  const fetchGroups = useCallback(async () => {
    try {
      const data = await getAdminGroups();
      setGroups(data.sort((a, b) => a.name.localeCompare(b.name)));
    } catch {
      toast.error("Error loading groups");
    } finally {
      setLoadingGroups(false);
    }
  }, []);

  useEffect(() => {
    fetchAgents();
    fetchGroups();
  }, [fetchAgents, fetchGroups]);

  // Agent handlers
  const handleDeleteAgent = async (id: string) => {
    if (!confirm(`Delete agent "${id}"?`)) return;
    try {
      await deleteAdminAgent(id);
      toast.success("Agent deleted");
      fetchAgents();
    } catch {
      toast.error("Error deleting agent");
    }
  };

  const handleSaveAgent = async (agent: Partial<AdminAgent>) => {
    try {
      if (editingAgent) {
        await updateAdminAgent(editingAgent.id, agent);
        await updateAdminAgentPermissions(
          editingAgent.id,
          agent.allowed_users || [],
          agent.allowed_groups || [],
        );
        toast.success("Agent updated");
      } else {
        await createAdminAgent(agent);
        toast.success("Agent created");
      }
      setEditingAgent(null);
      setCreatingAgent(false);
      fetchAgents();
    } catch {
      toast.error("Error saving agent");
    }
  };

  // Group handlers
  const handleDeleteGroup = async (id: string) => {
    if (!confirm(`Delete group "${id}"?`)) return;
    try {
      await deleteAdminGroup(id);
      toast.success("Group deleted");
      fetchGroups();
    } catch {
      toast.error("Error deleting group");
    }
  };

  const handleSaveGroup = async (group: Partial<AdminGroup>) => {
    try {
      if (editingGroup) {
        await updateAdminGroup(editingGroup.id, group);
        await updateAdminGroupPermissions(
          editingGroup.id,
          group.allowed_users || [],
          group.allowed_groups || [],
        );
        toast.success("Group updated");
      } else {
        await createAdminGroup(group);
        toast.success("Group created");
      }
      setEditingGroup(null);
      setCreatingGroup(false);
      fetchGroups();
    } catch {
      toast.error("Error saving group");
    }
  };

  // Agent form view
  if (creatingAgent || editingAgent) {
    return (
      <div>
        <button
          onClick={() => { setEditingAgent(null); setCreatingAgent(false); }}
          className="mb-4 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to agents
        </button>
        <h1 className="mb-6 text-2xl font-bold">
          {editingAgent ? `Edit: ${editingAgent.name}` : "New agent"}
        </h1>
        <AgentForm
          agent={editingAgent || undefined}
          onSave={handleSaveAgent}
          onCancel={() => { setEditingAgent(null); setCreatingAgent(false); }}
        />
      </div>
    );
  }

  // Group form view
  if (creatingGroup || editingGroup) {
    return (
      <div>
        <button
          onClick={() => { setEditingGroup(null); setCreatingGroup(false); }}
          className="mb-4 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to groups
        </button>
        <h1 className="mb-6 text-2xl font-bold">
          {editingGroup ? `Edit: ${editingGroup.name}` : "New group"}
        </h1>
        <GroupForm
          group={editingGroup || undefined}
          onSave={handleSaveGroup}
          onCancel={() => { setEditingGroup(null); setCreatingGroup(false); }}
        />
      </div>
    );
  }

  // Helper to get agent name by id
  const getAgentName = (id: string) => agents.find((a) => a.id === id)?.name || id;

  return (
    <div>
      {/* Tabs */}
      <div className="mb-6 flex items-center gap-4 border-b">
        <button
          onClick={() => setTab("agents")}
          className={`pb-2 text-sm font-medium transition-colors ${
            tab === "agents"
              ? "border-b-2 border-foreground text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          Agents
        </button>
        <button
          onClick={() => setTab("groups")}
          className={`pb-2 text-sm font-medium transition-colors ${
            tab === "groups"
              ? "border-b-2 border-foreground text-foreground"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          Groups
        </button>
      </div>

      {tab === "agents" && (
        <>
          <div className="mb-6 flex items-center justify-between">
            <h1 className="text-2xl font-bold">Agents</h1>
            <Button onClick={() => setCreatingAgent(true)} size="sm">
              <Plus className="mr-1 h-4 w-4" />
              New agent
            </Button>
          </div>

          {loadingAgents ? (
            <AdminTableSkeleton columns={6} rows={4} />
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead className="bg-muted/50">
                  <tr>
                    <th className="px-4 py-3 text-left font-medium">ID</th>
                    <th className="px-4 py-3 text-left font-medium">Name</th>
                    <th className="px-4 py-3 text-left font-medium">Protocol</th>
                    <th className="px-4 py-3 text-left font-medium">Status</th>
                    <th className="px-4 py-3 text-left font-medium">Permissions</th>
                    <th className="px-4 py-3 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {agents.map((agent) => (
                    <tr key={agent.id} className="hover:bg-muted/30">
                      <td className="px-4 py-3 font-mono text-xs">{agent.id}</td>
                      <td className="px-4 py-3">{agent.name}</td>
                      <td className="px-4 py-3">
                        <span className="rounded bg-muted px-2 py-0.5 text-xs">{agent.protocol}</span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs ${
                          agent.status === "healthy" ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400" :
                          agent.status === "unhealthy" ? "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400" :
                          "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
                        }`}>
                          {agent.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-xs text-muted-foreground">
                        {agent.allowed_users?.includes("*") ? "All" : `${(agent.allowed_users?.length || 0)} users, ${(agent.allowed_groups?.length || 0)} groups`}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Link href={`/admin/observability?r=agent:${agent.id}`}>
                            <Button variant="ghost" size="icon" className="h-7 w-7" title="View metrics">
                              <Eye className="h-3.5 w-3.5" />
                            </Button>
                          </Link>
                          <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setEditingAgent(agent)}>
                            <Pencil className="h-3.5 w-3.5" />
                          </Button>
                          <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => handleDeleteAgent(agent.id)}>
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {agents.length === 0 && (
                <div className="px-4 py-8 text-center text-muted-foreground">No agents configured</div>
              )}
            </div>
          )}
        </>
      )}

      {tab === "groups" && (
        <>
          <div className="mb-6 flex items-center justify-between">
            <h1 className="text-2xl font-bold">Groups</h1>
            <Button onClick={() => setCreatingGroup(true)} size="sm">
              <Plus className="mr-1 h-4 w-4" />
              New group
            </Button>
          </div>

          {loadingGroups ? (
            <AdminTableSkeleton columns={6} rows={4} />
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead className="bg-muted/50">
                  <tr>
                    <th className="px-4 py-3 text-left font-medium">Name</th>
                    <th className="px-4 py-3 text-left font-medium">Members</th>
                    <th className="px-4 py-3 text-left font-medium">Permissions</th>
                    <th className="px-4 py-3 text-left font-medium">Created by</th>
                    <th className="px-4 py-3 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {groups.map((group) => (
                    <tr key={group.id} className="hover:bg-muted/30">
                      <td className="px-4 py-3 font-medium">{group.name}</td>
                      <td className="px-4 py-3 text-xs text-muted-foreground">
                        {group.agent_ids?.map((id) => getAgentName(id)).join(", ")}
                      </td>
                      <td className="px-4 py-3 text-xs text-muted-foreground">
                        {group.allowed_users?.includes("*") ? "All" : (
                          <div className="space-y-0.5">
                            {group.allowed_users?.slice(0, 5).map((u) => (
                              <div key={u} className="truncate max-w-[200px]">{u}</div>
                            ))}
                            {(group.allowed_users?.length || 0) > 5 && (
                              <div className="text-muted-foreground/60">+{group.allowed_users!.length - 5} more</div>
                            )}
                            {group.allowed_groups?.map((g) => (
                              <div key={g} className="truncate max-w-[200px] text-violet-500">{g}</div>
                            ))}
                          </div>
                        )}
                      </td>
                      <td className="px-4 py-3 text-xs text-muted-foreground">{group.created_by}</td>
                      <td className="px-4 py-3 text-right">
                        <span className="text-xs text-muted-foreground">Read only</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {groups.length === 0 && (
                <div className="px-4 py-8 text-center text-muted-foreground">No groups configured</div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
