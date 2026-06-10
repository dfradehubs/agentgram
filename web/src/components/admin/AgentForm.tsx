"use client";

import { useState } from "react";
import type { AdminAgent, AgentApiKeyRule } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { TagInput } from "./TagInput";
import { ApiKeyRulesEditor } from "./ApiKeyRulesEditor";
import { SlackIntegration } from "./SlackIntegration";
import { Info, Lock } from "lucide-react";
import { useT } from "@/lib/i18n";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface AgentFormProps {
  agent?: AdminAgent;
  onSave: (agent: Partial<AdminAgent>) => void;
  onCancel: () => void;
}

export function AgentForm({ agent, onSave, onCancel }: AgentFormProps) {
  const t = useT();
  const [form, setForm] = useState({
    id: agent?.id || "",
    name: agent?.name || "",
    description: agent?.description || "",
    category: agent?.category || "",
    protocol: agent?.protocol || "custom",
    endpoint: agent?.endpoint || "",
    agent_card_path: agent?.agent_card_path || "",
    auth_type: agent?.auth_type || (agent?.forward_authorization ? "forward" : "none"),
    bearer_token: agent?.bearer_token || "",
    auth_header_name: agent?.auth_header_name || "Authorization",
    api_key_rules: (agent?.api_key_rules || []) as AgentApiKeyRule[],
    require_github_token: agent?.require_github_token || false,
    pipeline_final_agent: agent?.pipeline_final_agent || "",
    adk_app_name: agent?.adk_app_name || "",
    adk_user_id: agent?.adk_user_id || "",
    health_check_url: agent?.health_check?.url || "",
    custom_request_template: agent?.custom_format?.request_template || "",
    custom_response_path: agent?.custom_format?.response_content_path || "",
    custom_request_method: agent?.custom_format?.request_method || "POST",
    custom_content_type: agent?.custom_format?.request_content_type || "application/json",
    max_context_tokens: agent?.max_context_tokens ?? 200000,
    summarize_threshold: agent?.summarize_threshold ?? 0.80,
    allowed_users: agent?.allowed_users || [],
    allowed_groups: agent?.allowed_groups || [],
  });

  const inherited = agent?.inherited_permissions;

  // Compute default health check URL from endpoint
  const defaultHealthUrl = (() => {
    try {
      const u = new URL(form.endpoint);
      return `${u.origin}/health`;
    } catch {
      return "";
    }
  })();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const healthUrl = form.health_check_url || defaultHealthUrl;

    // Build custom_format only if any field has a non-default value
    const hasCustomFormat = form.protocol === "custom" && (
      form.custom_request_template ||
      form.custom_response_path ||
      (form.custom_request_method && form.custom_request_method !== "POST") ||
      (form.custom_content_type && form.custom_content_type !== "application/json")
    );

    onSave({
      id: form.id,
      name: form.name,
      description: form.description,
      category: form.category,
      protocol: form.protocol,
      endpoint: form.endpoint,
      agent_card_path: form.agent_card_path,
      auth_type: form.auth_type,
      forward_authorization: form.auth_type === "forward",
      bearer_token: form.auth_type === "bearer" ? form.bearer_token : "",
      auth_header_name: form.auth_type === "bearer" ? form.auth_header_name : "",
      api_key_rules: form.auth_type === "bearer" ? form.api_key_rules : [],
      require_github_token: form.require_github_token,
      pipeline_final_agent: form.pipeline_final_agent,
      adk_app_name: form.adk_app_name,
      adk_user_id: form.adk_user_id,
      max_context_tokens: form.max_context_tokens,
      summarize_threshold: form.summarize_threshold,
      headers: agent?.headers || {},
      health_check: healthUrl
        ? { enabled: true, url: healthUrl, endpoint: "", interval_seconds: 30, timeout_seconds: 5 }
        : undefined,
      custom_format: hasCustomFormat ? {
        request_template: form.custom_request_template || undefined,
        response_content_path: form.custom_response_path || undefined,
        request_method: form.custom_request_method !== "POST" ? form.custom_request_method : undefined,
        request_content_type: form.custom_content_type !== "application/json" ? form.custom_content_type : undefined,
      } : undefined,
      allowed_users: form.allowed_users,
      allowed_groups: form.allowed_groups,
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
            disabled={!!agent}
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
          <label className="mb-1 block text-sm font-medium">Category</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.category}
            onChange={e => update("category", e.target.value)}
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium">Protocol</label>
          <select
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.protocol}
            onChange={e => update("protocol", e.target.value)}
          >
            <option value="custom">Custom</option>
            <option value="a2a">A2A</option>
            <option value="adk">ADK</option>
          </select>
        </div>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">Endpoint</label>
        <input
          className="w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.endpoint}
          onChange={e => update("endpoint", e.target.value)}
          required
        />
      </div>

      {form.protocol === "a2a" && (
        <div>
          <label className="mb-1 block text-sm font-medium">Agent Card Path</label>
          <input
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.agent_card_path}
            onChange={e => update("agent_card_path", e.target.value)}
            placeholder="/.well-known/agent.json"
          />
        </div>
      )}

      {form.protocol === "adk" && (
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1 block text-sm font-medium">ADK App Name</label>
            <input
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
              value={form.adk_app_name}
              onChange={e => update("adk_app_name", e.target.value)}
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium">ADK User ID</label>
            <input
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
              value={form.adk_user_id}
              onChange={e => update("adk_user_id", e.target.value)}
            />
          </div>
        </div>
      )}

      {form.protocol === "custom" && (
        <div className="rounded-md border border-dashed p-4 space-y-3">
          <p className="text-sm font-medium text-muted-foreground">Custom format (optional)</p>

          <div>
            <label className="mb-1 block text-sm font-medium">Request template</label>
            <textarea
              className="w-full rounded-md border bg-background px-3 py-2 font-mono text-sm"
              value={form.custom_request_template}
              onChange={e => update("custom_request_template", e.target.value)}
              rows={4}
              placeholder={'{"query": "{{.Query}}", "conversation_id": "{{.SessionID}}"}'}
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium">JSON response content path</label>
            <input
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
              value={form.custom_response_path}
              onChange={e => update("custom_response_path", e.target.value)}
              placeholder="choices[0].message.content"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="mb-1 block text-sm font-medium">HTTP method</label>
              <select
                className="w-full rounded-md border bg-background px-3 py-2 text-sm"
                value={form.custom_request_method}
                onChange={e => update("custom_request_method", e.target.value)}
              >
                <option value="POST">POST</option>
                <option value="PUT">PUT</option>
                <option value="PATCH">PATCH</option>
              </select>
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium">Content-Type</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-2 text-sm"
                value={form.custom_content_type}
                onChange={e => update("custom_content_type", e.target.value)}
                placeholder="application/json"
              />
            </div>
          </div>

          <p className="text-xs text-muted-foreground">
            Leave empty to use the default format. Available variables:{" "}
            <code className="rounded bg-muted px-1">{"{{.Query}}"}</code>{" "}
            <code className="rounded bg-muted px-1">{"{{.LastMessage}}"}</code>{" "}
            <code className="rounded bg-muted px-1">{"{{.SessionID}}"}</code>{" "}
            <code className="rounded bg-muted px-1">{"{{.Messages}}"}</code>{" "}
            <code className="rounded bg-muted px-1">{"{{.UserEmail}}"}</code>
          </p>
        </div>
      )}

      <div>
        <label className="mb-1 block text-sm font-medium">Pipeline Final Agent</label>
        <input
          className="w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.pipeline_final_agent}
          onChange={e => update("pipeline_final_agent", e.target.value)}
          placeholder="Only for A2A with pipeline"
        />
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">{t("admin.agents.authType")}</label>
        <select
          className="w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.auth_type}
          onChange={e => update("auth_type", e.target.value)}
        >
          <option value="none">{t("admin.agents.authTypeNone")}</option>
          <option value="forward">{t("admin.agents.authTypeForward")}</option>
          <option value="bearer">{t("admin.agents.authTypeBearer")}</option>
          <option value="oauth2" disabled>{t("admin.agents.authTypeOAuth2Soon")}</option>
        </select>
        <p className="mt-1 text-xs text-muted-foreground">
          {form.auth_type === "none" && t("admin.agents.authTypeNoneHelp")}
          {form.auth_type === "forward" && t("admin.agents.authTypeForwardHelp")}
          {form.auth_type === "bearer" && t("admin.agents.authTypeBearerHelp")}
        </p>
      </div>

      {form.auth_type === "bearer" && (
        <div className="space-y-4 rounded-lg border border-dashed p-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="mb-1 block text-sm font-medium">{t("admin.agents.bearerToken")}</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-2 font-mono text-sm"
                type="password"
                value={form.bearer_token}
                onChange={e => update("bearer_token", e.target.value)}
                autoComplete="off"
              />
              <p className="mt-1 text-xs text-muted-foreground">{t("admin.agents.bearerTokenHelp")}</p>
            </div>
            <div>
              <label className="mb-1 block text-sm font-medium">{t("admin.agents.authHeaderName")}</label>
              <input
                className="w-full rounded-md border bg-background px-3 py-2 text-sm"
                value={form.auth_header_name}
                onChange={e => update("auth_header_name", e.target.value)}
                placeholder="Authorization"
              />
              <p className="mt-1 text-xs text-muted-foreground">{t("admin.agents.authHeaderNameHelp")}</p>
            </div>
          </div>

          <ApiKeyRulesEditor
            rules={form.api_key_rules}
            onChange={rules => setForm(prev => ({ ...prev, api_key_rules: rules }))}
          />
        </div>
      )}

      <div className="flex gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={form.require_github_token}
            onChange={e => update("require_github_token", e.target.checked)}
          />
          Require GitHub Token
        </label>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="mb-1 flex items-center gap-1.5 text-sm font-medium">
            {t("admin.agents.maxContextTokens")}
            <Tooltip>
              <TooltipTrigger asChild>
                <Info className="h-3.5 w-3.5 text-muted-foreground cursor-help" />
              </TooltipTrigger>
              <TooltipContent side="top" className="max-w-xs text-xs">
                {t("admin.agents.maxContextTokensHelp")}
              </TooltipContent>
            </Tooltip>
          </label>
          <input
            type="number"
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.max_context_tokens}
            onChange={e => update("max_context_tokens", parseInt(e.target.value) || 0)}
            min={0}
          />
        </div>
        <div>
          <label className="mb-1 flex items-center gap-1.5 text-sm font-medium">
            {t("admin.agents.summarizeThreshold")}
            <Tooltip>
              <TooltipTrigger asChild>
                <Info className="h-3.5 w-3.5 text-muted-foreground cursor-help" />
              </TooltipTrigger>
              <TooltipContent side="top" className="max-w-xs text-xs">
                {t("admin.agents.summarizeThresholdHelp")}
              </TooltipContent>
            </Tooltip>
          </label>
          <input
            type="number"
            className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            value={form.summarize_threshold}
            onChange={e => update("summarize_threshold", parseFloat(e.target.value) || 0)}
            min={0}
            max={1}
            step={0.01}
          />
        </div>
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium">Health Check URL</label>
        <input
          className="w-full rounded-md border bg-background px-3 py-2 text-sm"
          value={form.health_check_url}
          onChange={e => update("health_check_url", e.target.value)}
          placeholder={defaultHealthUrl || "https://host:port/health"}
        />
        <p className="mt-1 text-xs text-muted-foreground">
          {defaultHealthUrl ? `Default: ${defaultHealthUrl}` : "Calculated automatically from the endpoint"}
        </p>
      </div>

      <TagInput
        label="Allowed users"
        values={form.allowed_users}
        onChange={v => setForm(prev => ({ ...prev, allowed_users: v }))}
        placeholder="* for everyone, or emails"
      />
      {inherited && inherited.users.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {inherited.users.map((p, i) => (
            <Tooltip key={`iu-${i}`}>
              <TooltipTrigger asChild>
                <span className="flex items-center gap-1 rounded bg-muted/60 px-2 py-0.5 text-xs text-muted-foreground">
                  <Lock className="h-3 w-3" />
                  {p.value}
                </span>
              </TooltipTrigger>
              <TooltipContent side="top" className="text-xs">
                Inherited from group: {p.from_group}
              </TooltipContent>
            </Tooltip>
          ))}
        </div>
      )}

      <TagInput
        label="Allowed groups"
        values={form.allowed_groups}
        onChange={v => setForm(prev => ({ ...prev, allowed_groups: v }))}
        placeholder="/google-workspace/group@example.com"
      />
      {inherited && inherited.groups.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {inherited.groups.map((p, i) => (
            <Tooltip key={`ig-${i}`}>
              <TooltipTrigger asChild>
                <span className="flex items-center gap-1 rounded bg-muted/60 px-2 py-0.5 text-xs text-muted-foreground">
                  <Lock className="h-3 w-3" />
                  {p.value}
                </span>
              </TooltipTrigger>
              <TooltipContent side="top" className="text-xs">
                Inherited from group: {p.from_group}
              </TooltipContent>
            </Tooltip>
          ))}
        </div>
      )}

      {agent && <SlackIntegration agentId={agent.id} />}

      <div className="flex gap-2 pt-4">
        <Button type="submit">{agent ? "Save changes" : "Create agent"}</Button>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </form>
  );
}
