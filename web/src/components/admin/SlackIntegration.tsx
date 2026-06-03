"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import type { SlackIntegration as SlackIntegrationType } from "@/lib/types";
import {
  getAgentSlack,
  upsertAgentSlack,
  deleteAgentSlack,
  testAgentSlack,
} from "@/lib/api";
import { toast } from "sonner";
import { ChevronDown, ChevronRight, Plug, Trash2 } from "lucide-react";

interface SlackIntegrationProps {
  agentId?: string;
}

export function SlackIntegration({ agentId }: SlackIntegrationProps) {
  const [expanded, setExpanded] = useState(false);
  const [integration, setIntegration] = useState<SlackIntegrationType | null>(null);
  const [botToken, setBotToken] = useState("");
  const [appToken, setAppToken] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);

  const load = useCallback(async () => {
    if (!agentId) return;
    try {
      const data = await getAgentSlack(agentId);
      setIntegration(data);
      setEnabled(data.enabled);
    } catch {
      // No integration yet
    }
  }, [agentId]);

  useEffect(() => {
    if (agentId && expanded) {
      load();
    }
  }, [agentId, expanded, load]);

  if (!agentId) return null;

  const handleTest = async () => {
    const token = botToken || (integration?.has_bot_token ? "" : "");
    if (!token && !integration?.has_bot_token) {
      toast.error("Enter the Bot Token first");
      return;
    }
    setTesting(true);
    try {
      const result = await testAgentSlack(token, appToken, agentId);
      toast.success(`Connected to workspace: ${result.workspace_name}`);
    } catch (err) {
      toast.error(`Connection error: ${err instanceof Error ? err.message : "Unknown error"}`);
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async () => {
    setLoading(true);
    try {
      const data = await upsertAgentSlack(agentId, {
        bot_token: botToken || undefined,
        app_token: appToken || undefined,
        enabled,
      });
      setIntegration(data);
      setBotToken("");
      setAppToken("");
      toast.success(enabled ? "Slack integration enabled" : "Slack integration saved");
    } catch (err) {
      toast.error(`Error: ${err instanceof Error ? err.message : "Unknown error"}`);
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm("Delete the Slack integration? The bot will disconnect.")) return;
    setLoading(true);
    try {
      await deleteAgentSlack(agentId);
      setIntegration(null);
      setBotToken("");
      setAppToken("");
      setEnabled(false);
toast.success("Integration deleted");
    } catch (err) {
      toast.error(`Error: ${err instanceof Error ? err.message : "Unknown error"}`);
    } finally {
      setLoading(false);
    }
  };

  const statusColor = integration?.status === "connected"
    ? "bg-green-500"
    : integration?.status === "error"
    ? "bg-red-500"
    : "bg-yellow-500";

  const statusLabel = integration?.status === "connected"
    ? "Connected"
    : integration?.status === "error"
    ? "Error"
    : "Disconnected";

  return (
    <div className="rounded-md border border-border">
      <button
        type="button"
        className="flex w-full items-center justify-between px-4 py-3 text-sm font-medium"
        onClick={() => setExpanded(!expanded)}
      >
        <span className="flex items-center gap-2">
          <Plug className="h-4 w-4" />
          Slack integration
          {integration?.enabled && (
            <span className={`inline-block h-2 w-2 rounded-full ${statusColor}`} title={statusLabel} />
          )}
        </span>
        {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
      </button>

      {expanded && (
        <div className="space-y-4 border-t border-border px-4 py-4">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              className="rounded"
            />
            Enable Slack integration
          </label>

          {integration?.enabled && integration.workspace_name && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span className={`inline-block h-2 w-2 rounded-full ${statusColor}`} />
              {statusLabel}
              {integration.workspace_name && ` — ${integration.workspace_name}`}
              {integration.status_message && ` (${integration.status_message})`}
            </div>
          )}

          <div>
            <label className="mb-1 block text-sm font-medium">Bot Token</label>
            <input
              type="password"
              value={botToken}
              onChange={(e) => setBotToken(e.target.value)}
              placeholder={integration?.has_bot_token ? "••••••••••••• (configured)" : "xoxb-..."}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium">App Token (Socket Mode)</label>
            <input
              type="password"
              value={appToken}
              onChange={(e) => setAppToken(e.target.value)}
              placeholder={integration?.has_app_token ? "••••••••••••• (configured)" : "xapp-..."}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            />
          </div>

          <div className="flex items-center gap-2">
            <Button type="button" size="sm" onClick={handleTest} disabled={testing}>
              {testing ? "Testing..." : "Test connection"}
            </Button>
            <Button type="button" size="sm" onClick={handleSave} disabled={loading}>
              {loading ? "Saving..." : "Save"}
            </Button>
            {integration?.enabled && (
              <Button type="button" size="sm" variant="destructive" onClick={handleDelete} disabled={loading}>
                <Trash2 className="mr-1 h-3 w-3" />
                Delete
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
