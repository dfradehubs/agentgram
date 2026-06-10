"use client";

import { useState } from "react";
import type { AdminMCPServer, MCPApiKeyRule } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { TagInput } from "./TagInput";
import { ApiKeyRulesEditor } from "./ApiKeyRulesEditor";

interface MCPFormProps {
  server?: AdminMCPServer;
  onSave: (server: Partial<AdminMCPServer>) => void;
  onCancel: () => void;
}

export function MCPForm({ server, onSave, onCancel }: MCPFormProps) {
  const [form, setForm] = useState({
    id: server?.id || "",
    name: server?.name || "",
    description: server?.description || "",
    transport: server?.transport || "http",
    url: server?.url || "",
    forward_auth: server?.forward_auth || false,
    allowed_users: server?.allowed_users || [],
    allowed_groups: server?.allowed_groups || [],
    auth_type: server?.auth_type || "none",
    oauth2_auth_server_url: server?.oauth2_auth_server_url || "",
    oauth2_client_id: server?.oauth2_client_id || "",
    oauth2_client_secret: server?.oauth2_client_secret || "",
    oauth2_scopes: server?.oauth2_scopes || "",
    bearer_token: server?.bearer_token || "",
    auth_header_name: server?.auth_header_name || "Authorization",
    api_key_rules: (server?.api_key_rules || []) as MCPApiKeyRule[],
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      ...form,
      headers: server?.headers || {},
      forward_auth: form.auth_type === "forward",
      allowed_users: form.allowed_users,
      allowed_groups: form.allowed_groups,
      auth_header_name: form.auth_type === "bearer" ? form.auth_header_name : "",
      api_key_rules: form.auth_type === "bearer" ? form.api_key_rules : [],
    });
  };

  const update = (field: string, value: string) =>
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
            disabled={!!server}
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
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 block text-sm font-medium">Transport</label>
          <select
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.transport}
            onChange={e => update("transport", e.target.value)}
          >
            <option value="http">HTTP</option>
            <option value="sse">SSE</option>
          </select>
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">URL</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.url}
            onChange={e => update("url", e.target.value)}
            required
          />
        </div>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">Authentication type</label>
        <select
          className="w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.auth_type}
          onChange={e => setForm(prev => ({
            ...prev,
            auth_type: e.target.value,
            forward_auth: e.target.value === "forward",
          }))}
        >
          <option value="none">No authentication</option>
          <option value="forward">Forward user JWT</option>
          <option value="oauth2">OAuth 2.1 (per user)</option>
          <option value="bearer">Bearer Token (service)</option>
        </select>
        <p className="mt-1 text-xs text-muted-foreground">
          {form.auth_type === "none" && "The MCP server does not require authentication."}
          {form.auth_type === "forward" && "The user's Keycloak JWT is forwarded to the MCP server."}
          {form.auth_type === "oauth2" && "Each user authorizes their access to the MCP server via OAuth2. Scopes are assigned per group."}
          {form.auth_type === "bearer" && "Static service token shared by all users. Sent as an Authorization: Bearer <token> header."}
        </p>
      </div>

      {form.auth_type === "bearer" && (
        <div className="space-y-4 rounded-lg border border-dashed p-4">
          <h3 className="text-sm font-semibold">Bearer / API Key</h3>
          <p className="text-xs text-muted-foreground">
            Agentgram authenticates to the MCP server as a service with an API key. Optionally send a different key per user or group; otherwise the default key (fallback) is used.
          </p>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="mb-1 block text-sm font-medium">Default API key (fallback)</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-2 text-sm font-mono"
                type="password"
                value={form.bearer_token}
                onChange={e => update("bearer_token", e.target.value)}
                placeholder="Paste the API key here"
                autoComplete="off"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                Used when no rule matches. Stored in the API database; any administrator with panel access can retrieve it.
              </p>
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium">Auth header</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-2 text-sm"
                value={form.auth_header_name}
                onChange={e => update("auth_header_name", e.target.value)}
                placeholder="Authorization"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                &quot;Authorization&quot; sends the key with a &quot;Bearer &quot; prefix. Any other header (e.g. X-API-Key) sends the key verbatim.
              </p>
            </div>
          </div>

          <ApiKeyRulesEditor
            rules={form.api_key_rules}
            onChange={rules => setForm(prev => ({ ...prev, api_key_rules: rules }))}
          />
        </div>
      )}

      {form.auth_type === "oauth2" && (
        <div className="space-y-4 rounded-lg border border-dashed p-4">
          <h3 className="text-sm font-semibold">OAuth 2.1 Configuration</h3>
          <p className="text-xs text-muted-foreground">
            Only the MCP URL is required. On save, the authorization server, scopes, and client_id are discovered automatically (via RFC 9728 + RFC 8414 + RFC 7591). You can override any field manually.
          </p>

          <div>
            <label className="mb-1 block text-sm font-medium">Authorization Server URL <span className="font-normal text-muted-foreground">(optional)</span></label>
            <input
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
              value={form.oauth2_auth_server_url}
              onChange={e => update("oauth2_auth_server_url", e.target.value)}
              placeholder="Discovered automatically from the MCP URL"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="mb-1 block text-sm font-medium">Client ID</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-2 text-sm"
                value={form.oauth2_client_id}
                onChange={e => update("oauth2_client_id", e.target.value)}
                placeholder="Leave empty for automatic registration (RFC 7591)"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                If the auth server supports Dynamic Client Registration, it will register automatically on save.
              </p>
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium">Client Secret</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-2 text-sm"
                type="password"
                value={form.oauth2_client_secret}
                onChange={e => update("oauth2_client_secret", e.target.value)}
                placeholder="Leave empty for public clients (PKCE)"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                Only for confidential clients. Leave empty to use PKCE.
              </p>
            </div>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium">Base scopes <span className="font-normal text-muted-foreground">(optional)</span></label>
            <input
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
              value={form.oauth2_scopes}
              onChange={e => update("oauth2_scopes", e.target.value)}
              placeholder="Discovered automatically from the auth server"
            />
            <p className="mt-1 text-xs text-muted-foreground">
              Base scopes for all users. Auto-discovered if not specified. Additional per-group scopes are configured in Scope Mappings.
            </p>
          </div>
        </div>
      )}

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
        <Button type="submit">{server ? "Save changes" : "Create server"}</Button>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </form>
  );
}
