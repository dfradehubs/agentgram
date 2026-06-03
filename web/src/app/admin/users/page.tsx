"use client";

import { useEffect, useState, useCallback } from "react";
import type { AdminUser, BasicAuthUser, SlackUserLink } from "@/lib/types";
import {
  getAdminUsers,
  updateAdminUserRole,
  getBasicAuthUsers,
  createBasicAuthUser,
  deleteBasicAuthUser,
  getSlackUserLinks,
  revokeSlackUserLink,
  revokeSlackUserGitHub,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import { Plus, Trash2, KeyRound, MessageSquare } from "lucide-react";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

export default function AdminUsersPage() {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [basicUsers, setBasicUsers] = useState<BasicAuthUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [newUser, setNewUser] = useState({ username: "", email: "", password: "" });
  const [creating, setCreating] = useState(false);
  const [slackLinks, setSlackLinks] = useState<SlackUserLink[]>([]);

  const fetchSlackLinks = useCallback(async () => {
    try {
      const data = await getSlackUserLinks();
      setSlackLinks(data || []);
    } catch {
      // Slack links may not be configured — ignore
    }
  }, []);

  const fetchUsers = useCallback(async () => {
    try {
      const data = await getAdminUsers();
      setUsers(data.sort((a, b) => a.email.localeCompare(b.email)));
    } catch {
      toast.error("Error loading users");
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchBasicUsers = useCallback(async () => {
    try {
      const data = await getBasicAuthUsers();
      setBasicUsers(data);
    } catch {
      // Basic auth may not be enabled — ignore
    }
  }, []);

  useEffect(() => {
    fetchUsers();
    fetchBasicUsers();
    fetchSlackLinks();
  }, [fetchUsers, fetchBasicUsers, fetchSlackLinks]);

  const handleToggleRole = async (user: AdminUser) => {
    const newRole = user.role === "admin" ? "user" : "admin";
    if (!confirm(`Change role of ${user.email} to "${newRole}"?`)) return;
    try {
      await updateAdminUserRole(user.email, newRole);
      toast.success(`Role of ${user.email} changed to ${newRole}`);
      fetchUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Error changing role");
    }
  };

  const handleCreateBasicUser = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    try {
      await createBasicAuthUser(newUser);
      toast.success(`User "${newUser.username}" created`);
      setNewUser({ username: "", email: "", password: "" });
      setShowCreateForm(false);
      fetchBasicUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Error creating user");
    } finally {
      setCreating(false);
    }
  };

  const handleRevokeSlackLink = async (link: SlackUserLink) => {
    if (!confirm(`Revoke Slack link for ${link.email}? They will have to link their account again.`)) return;
    try {
      await revokeSlackUserLink(link.slack_user_id);
      toast.success(`Link for ${link.email} revoked`);
      fetchSlackLinks();
    } catch {
      toast.error("Error revoking link");
    }
  };

  const handleRevokeSlackGitHub = async (link: SlackUserLink) => {
    if (!confirm(`Revoke GitHub for ${link.email}?`)) return;
    try {
      await revokeSlackUserGitHub(link.slack_user_id);
      toast.success(`GitHub for ${link.email} revoked`);
      fetchSlackLinks();
    } catch {
      toast.error("Error revoking GitHub");
    }
  };

  const handleDeleteBasicUser = async (user: BasicAuthUser) => {
    if (!confirm(`Delete basic auth user "${user.username}"?`)) return;
    try {
      await deleteBasicAuthUser(user.id);
      toast.success(`User "${user.username}" deleted`);
      fetchBasicUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Error deleting user");
    }
  };

  return (
    <div className="space-y-8">
      {/* SSO Users */}
      <div>
        <div className="mb-6">
          <h1 className="text-2xl font-bold">Users</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Users are created automatically when they sign in. Here you can manage their roles.
          </p>
        </div>

        {loading ? (
          <AdminTableSkeleton columns={4} rows={5} />
        ) : (
          <div className="overflow-x-auto rounded-lg border">
            <table className="w-full text-sm">
              <thead className="bg-muted/50">
                <tr>
                  <th className="px-4 py-3 text-left font-medium">Email</th>
                  <th className="px-4 py-3 text-left font-medium">Role</th>
                  <th className="px-4 py-3 text-left font-medium">Created</th>
                  <th className="px-4 py-3 text-left font-medium">Last access</th>
                  <th className="px-4 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {users.map((user) => (
                  <tr key={user.id} className="hover:bg-muted/30">
                    <td className="px-4 py-3">{user.email}</td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex rounded-full px-2 py-0.5 text-xs ${
                        user.role === "admin"
                          ? "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400"
                          : "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
                      }`}>
                        {user.role}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {new Date(user.created_at).toLocaleDateString("en-US")}
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {user.last_access_at ? new Date(user.last_access_at).toLocaleString("en-US", { dateStyle: "short", timeStyle: "short" }) : "—"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {user.protected ? (
                        <span className="text-xs text-muted-foreground" title="Admin by system configuration">
                          Protected
                        </span>
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleToggleRole(user)}
                        >
                          {user.role === "admin" ? "Remove admin" : "Make admin"}
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {users.length === 0 && (
              <div className="px-4 py-8 text-center text-muted-foreground">No registered users</div>
            )}
          </div>
        )}
      </div>

      {/* Basic Auth Users */}
      <div>
        <div className="mb-4 flex items-center justify-between">
          <div>
            <h2 className="flex items-center gap-2 text-lg font-semibold">
              <KeyRound className="h-5 w-5" />
              Basic Auth Users
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Users with username and password authentication. The first admin user is created from the configuration (<code className="rounded bg-muted px-1 text-xs">seed_users</code>).
            </p>
          </div>
          <Button
            size="sm"
            onClick={() => setShowCreateForm(!showCreateForm)}
          >
            <Plus className="mr-1 h-4 w-4" />
            New user
          </Button>
        </div>

        {/* Create form */}
        {showCreateForm && (
          <form onSubmit={handleCreateBasicUser} className="mb-4 rounded-lg border bg-muted/30 p-4">
            <div className="grid grid-cols-3 gap-3">
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">Username</label>
                <input
                  type="text"
                  value={newUser.username}
                  onChange={(e) => setNewUser({ ...newUser, username: e.target.value })}
                  className="w-full rounded-md border bg-transparent px-3 py-1.5 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700"
                  required
                  placeholder="username"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">Email</label>
                <input
                  type="email"
                  value={newUser.email}
                  onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
                  className="w-full rounded-md border bg-transparent px-3 py-1.5 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700"
                  required
                  placeholder="user@example.com"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">Password</label>
                <input
                  type="password"
                  value={newUser.password}
                  onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
                  className="w-full rounded-md border bg-transparent px-3 py-1.5 text-sm outline-none focus:ring-2 focus:ring-zinc-400 dark:border-zinc-700"
                  required
                  placeholder="••••••••"
                  minLength={8}
                />
              </div>
            </div>
            <div className="mt-3 flex justify-end gap-2">
              <Button type="button" variant="ghost" size="sm" onClick={() => setShowCreateForm(false)}>
                Cancel
              </Button>
              <Button type="submit" size="sm" disabled={creating}>
                {creating ? "Creating..." : "Create user"}
              </Button>
            </div>
          </form>
        )}

        {/* Basic auth users table */}
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="px-4 py-3 text-left font-medium">Username</th>
                <th className="px-4 py-3 text-left font-medium">Email</th>
                <th className="px-4 py-3 text-left font-medium">Created</th>
                <th className="px-4 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {basicUsers.map((user) => (
                <tr key={user.id} className="hover:bg-muted/30">
                  <td className="px-4 py-3 font-mono text-sm">{user.username}</td>
                  <td className="px-4 py-3">{user.email}</td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {new Date(user.created_at).toLocaleDateString("en-US")}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleDeleteBasicUser(user)}
                      className="text-red-500 hover:text-red-600"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {basicUsers.length === 0 && (
            <div className="px-4 py-8 text-center text-muted-foreground">
              No basic auth users. Use <code className="rounded bg-muted px-1 text-xs">seed_users</code> in the config or create one here.
            </div>
          )}
        </div>
      </div>

      {/* Slack Linked Accounts */}
      <div>
        <div className="mb-4">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <MessageSquare className="h-5 w-5" />
            Linked Slack accounts
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Users who have linked their Slack account with Keycloak. Each one generates its own JWT when linking.
            Revoking removes the token and the user will have to link again.
          </p>
        </div>

        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="px-4 py-3 text-left font-medium">Email</th>
                <th className="px-4 py-3 text-left font-medium">Slack User ID</th>
                <th className="px-4 py-3 text-left font-medium">GitHub</th>
                <th className="px-4 py-3 text-left font-medium">Linked</th>
                <th className="px-4 py-3 text-left font-medium">Last used</th>
                <th className="px-4 py-3 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {slackLinks.map((link) => (
                <tr key={link.slack_user_id} className="hover:bg-muted/30">
                  <td className="px-4 py-3">{link.email}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{link.slack_user_id}</td>
                  <td className="px-4 py-3">
                    {link.has_github ? (
                      <span className="inline-flex items-center gap-1 rounded-full bg-green-500/10 px-2 py-0.5 text-xs text-green-500">Connected</span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {new Date(link.created_at).toLocaleDateString("en-US")}
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {new Date(link.updated_at).toLocaleString("en-US", { dateStyle: "short", timeStyle: "short" })}
                  </td>
                  <td className="px-4 py-3 text-right space-x-1">
                    {link.has_github && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRevokeSlackGitHub(link)}
                        className="text-orange-500 hover:text-orange-600"
                      >
                        Revoke GitHub
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRevokeSlackLink(link)}
                      className="text-red-500 hover:text-red-600"
                    >
                      <Trash2 className="mr-1 h-3 w-3" />
                      Revoke
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {slackLinks.length === 0 && (
            <div className="px-4 py-8 text-center text-muted-foreground">
              No linked accounts. Users link their account the first time they message a Slack bot.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
