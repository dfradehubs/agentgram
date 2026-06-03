"use client";

import { useEffect, useState, useCallback } from "react";
import type { AdminMCPServer, MCPOAuth2ScopeMapping } from "@/lib/types";
import { getAdminMCPServers, deleteAdminMCPServer, createAdminMCPServer, updateAdminMCPServer, getMCPScopeMappings, upsertMCPScopeMapping, deleteMCPScopeMapping } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { MCPForm } from "@/components/admin/MCPForm";
import { Plus, Pencil, Trash2, ArrowLeft, Eye } from "lucide-react";
import Link from "next/link";
import { toast } from "sonner";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

export default function AdminMCPPage() {
  const [servers, setServers] = useState<AdminMCPServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingServer, setEditingServer] = useState<AdminMCPServer | null>(null);
  const [creatingServer, setCreatingServer] = useState(false);

  const fetchServers = useCallback(async () => {
    try {
      const data = await getAdminMCPServers();
      setServers(data.sort((a, b) => a.name.localeCompare(b.name)));
    } catch {
      toast.error("Error loading MCP servers");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchServers();
  }, [fetchServers]);

  const handleDelete = async (id: string) => {
    if (!confirm(`Delete MCP server "${id}"?`)) return;
    try {
      await deleteAdminMCPServer(id);
      toast.success("MCP server deleted");
      fetchServers();
    } catch {
      toast.error("Error deleting MCP server");
    }
  };

  const handleSave = async (server: Partial<AdminMCPServer>) => {
    try {
      if (editingServer) {
        await updateAdminMCPServer(editingServer.id, server);
        toast.success("MCP server updated");
      } else {
        await createAdminMCPServer(server);
        toast.success("MCP server created");
      }
      setEditingServer(null);
      setCreatingServer(false);
      fetchServers();
    } catch {
      toast.error("Error saving MCP server");
    }
  };

  if (creatingServer || editingServer) {
    return (
      <div>
        <button
          onClick={() => { setEditingServer(null); setCreatingServer(false); }}
          className="mb-4 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to MCP servers
        </button>
        <h1 className="mb-6 text-2xl font-bold">
          {editingServer ? `Edit: ${editingServer.name}` : "New MCP server"}
        </h1>
        <MCPForm
          server={editingServer || undefined}
          onSave={handleSave}
          onCancel={() => { setEditingServer(null); setCreatingServer(false); }}
        />
        {editingServer && editingServer.auth_type === "oauth2" && (
          <ScopeMappingSection serverId={editingServer.id} />
        )}
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">MCP Servers</h1>
        <Button onClick={() => setCreatingServer(true)} size="sm">
          <Plus className="mr-1 h-4 w-4" />
          New server
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
                <th className="px-4 py-3 text-left font-medium">Transport</th>
                <th className="px-4 py-3 text-left font-medium">URL</th>
                <th className="px-4 py-3 text-left font-medium">Permissions</th>
                <th className="px-4 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {servers.map((server) => (
                <tr key={server.id} className="hover:bg-muted/30">
                  <td className="px-4 py-3 font-mono text-xs">{server.id}</td>
                  <td className="px-4 py-3">{server.name}</td>
                  <td className="px-4 py-3">
                    <span className="rounded bg-muted px-2 py-0.5 text-xs">{server.transport}</span>
                  </td>
                  <td className="max-w-xs truncate px-4 py-3 text-xs text-muted-foreground">{server.url}</td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {server.allowed_users?.includes("*") ? "All" : `${(server.allowed_users?.length || 0)} users, ${(server.allowed_groups?.length || 0)} groups`}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Link href={`/admin/observability?r=mcp:${server.id}`}>
                        <Button variant="ghost" size="icon" className="h-7 w-7" title="View metrics">
                          <Eye className="h-3.5 w-3.5" />
                        </Button>
                      </Link>
                      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setEditingServer(server)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => handleDelete(server.id)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {servers.length === 0 && (
            <div className="px-4 py-8 text-center text-muted-foreground">No MCP servers configured</div>
          )}
        </div>
      )}
    </div>
  );
}

function ScopeMappingSection({ serverId }: { serverId: string }) {
  const [mappings, setMappings] = useState<MCPOAuth2ScopeMapping[]>([]);
  const [newGroup, setNewGroup] = useState("");
  const [newScopes, setNewScopes] = useState("");
  const [loading, setLoading] = useState(true);

  const fetchMappings = useCallback(async () => {
    try {
      const data = await getMCPScopeMappings(serverId);
      setMappings(data);
    } catch {
      toast.error("Error loading scope mappings");
    } finally {
      setLoading(false);
    }
  }, [serverId]);

  useEffect(() => { fetchMappings(); }, [fetchMappings]);

  const handleAdd = async () => {
    if (!newGroup.trim() || !newScopes.trim()) return;
    try {
      await upsertMCPScopeMapping(serverId, newGroup.trim(), newScopes.trim());
      setNewGroup("");
      setNewScopes("");
      fetchMappings();
      toast.success("Scope mapping saved");
    } catch {
      toast.error("Error saving scope mapping");
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteMCPScopeMapping(serverId, id);
      fetchMappings();
    } catch {
      toast.error("Error deleting scope mapping");
    }
  };

  return (
    <div className="mt-8 max-w-2xl">
      <h2 className="mb-3 text-lg font-semibold">Scope Mappings by Group</h2>
      <p className="mb-4 text-sm text-muted-foreground">
        Assign additional OAuth2 scopes based on the user&apos;s group. The base scopes (configured above) always apply.
      </p>

      {loading ? (
        <div className="text-sm text-muted-foreground">Loading...</div>
      ) : (
        <>
          {mappings.length > 0 && (
            <div className="mb-4 overflow-x-auto rounded-lg border">
              <table className="w-full text-sm">
                <thead className="bg-muted/50">
                  <tr>
                    <th className="px-4 py-2 text-left font-medium">Group</th>
                    <th className="px-4 py-2 text-left font-medium">Additional scopes</th>
                    <th className="px-4 py-2 text-right font-medium"></th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {mappings.map((m) => (
                    <tr key={m.id}>
                      <td className="px-4 py-2 font-mono text-xs">{m.group_name}</td>
                      <td className="px-4 py-2 text-xs">{m.scopes}</td>
                      <td className="px-4 py-2 text-right">
                        <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={() => handleDelete(m.id)}>
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="flex items-end gap-2">
            <div className="flex-1">
              <label className="mb-1 block text-xs font-medium">Group</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-1.5 text-sm"
                value={newGroup}
                onChange={e => setNewGroup(e.target.value)}
                placeholder="/google-workspace/sre@example.com"
              />
            </div>
            <div className="flex-1">
              <label className="mb-1 block text-xs font-medium">Scopes</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-1.5 text-sm"
                value={newScopes}
                onChange={e => setNewScopes(e.target.value)}
                placeholder="admin:write deploy:execute"
              />
            </div>
            <Button size="sm" onClick={handleAdd} disabled={!newGroup.trim() || !newScopes.trim()}>
              <Plus className="mr-1 h-3 w-3" />
              Add
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
