"use client";

import { useState } from "react";
import { Bot, Plug, Terminal } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useUser } from "@/hooks/useUser";
import { useAgents } from "@/hooks/useAgents";
import { createAdminAgent } from "@/lib/api";
import { slugify } from "@/lib/slugify";

export function FirstRunWizard() {
  const t = useT();
  const { isAdmin, role } = useUser();
  const { refreshAgents, selectAgent } = useAgents();
  const canRegister = isAdmin || role === "editor";
  const [name, setName] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [protocol, setProtocol] = useState("custom");
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !endpoint.trim()) return;
    setSaving(true);
    try {
      const created = await createAdminAgent({
        id: slugify(name),
        name: name.trim(),
        description: "",
        category: "getting-started",
        protocol,
        endpoint: endpoint.trim(),
        forward_authorization: false,
        require_github_token: false,
        headers: {},
        allowed_users: [],
        allowed_groups: ["*"],
      });
      toast.success(t("wizard.created"));
      await refreshAgents();
      selectAgent(created.id);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("wizard.createError"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex h-full flex-col items-center justify-center p-8 animate-in fade-in duration-500">
      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-violet-500/10 to-purple-600/10">
        <Bot className="h-7 w-7 text-violet-500" />
      </div>
      <h3 className="mb-1.5 text-base font-medium">{t("wizard.title")}</h3>
      <p className="mb-6 max-w-md text-center text-sm text-muted-foreground">
        {t("wizard.subtitle")}
      </p>

      {canRegister ? (
        <form onSubmit={handleSubmit} className="w-full max-w-md space-y-3">
          <label className="block text-xs font-medium text-muted-foreground">
            {t("wizard.name")}
            <input
              className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="logs-agent"
              required
            />
          </label>
          <label className="block text-xs font-medium text-muted-foreground">
            {t("wizard.endpoint")}
            <input
              className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              placeholder="http://localhost:9000/chat"
              required
            />
          </label>
          <label className="block text-xs font-medium text-muted-foreground">
            {t("wizard.protocol")}
            <select
              className="mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={protocol}
              onChange={(e) => setProtocol(e.target.value)}
            >
              <option value="custom">Custom REST / SSE</option>
              <option value="a2a">A2A (JSON-RPC)</option>
              <option value="adk">Google ADK</option>
            </select>
          </label>
          <Button type="submit" className="w-full" disabled={saving}>
            <Plug className="mr-2 h-4 w-4" />
            {saving ? t("common.creating") : t("wizard.submit")}
          </Button>
        </form>
      ) : (
        <p className="max-w-md text-center text-sm text-muted-foreground">
          {t("wizard.askAdmin")}
        </p>
      )}

      <div className="mt-8 w-full max-w-md rounded-md border border-dashed p-3 text-left">
        <div className="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <Terminal className="h-3.5 w-3.5" />
          {t("wizard.mcpHint")}
        </div>
        <pre className="overflow-x-auto text-[11px] leading-relaxed text-muted-foreground">
          {`claude mcp add --transport http agentgram http://localhost:8080/mcp`}
        </pre>
      </div>
    </div>
  );
}
