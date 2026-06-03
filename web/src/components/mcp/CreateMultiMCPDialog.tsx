"use client";

import { useState } from "react";
import { useMCPContext } from "@/contexts/MCPContext";
import { useConfig } from "@/contexts/ConfigContext";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Circle } from "lucide-react";
import { useT } from "@/lib/i18n";

interface CreateMultiMCPDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

export function CreateMultiMCPDialog({
  isOpen,
  onClose,
}: CreateMultiMCPDialogProps) {
  const { mcpServers, selectMultiMCP } = useMCPContext();
  const config = useConfig();
  const t = useT();
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [selectedModelId, setSelectedModelId] = useState(
    config.available_models.find((m) => m.default)?.id || config.available_models[0]?.id || ""
  );
  const [sessionName, setSessionName] = useState("");

  const toggleServer = (serverId: string) => {
    setSelectedIds((prev) =>
      prev.includes(serverId)
        ? prev.filter((id) => id !== serverId)
        : [...prev, serverId]
    );
  };

  const generateDefaultName = () => {
    return selectedIds
      .map((id) => mcpServers.find((s) => s.id === id)?.name || id)
      .join(" + ");
  };

  const handleCreate = () => {
    if (selectedIds.length < 2) return;
    selectMultiMCP(selectedIds);
    setSelectedIds([]);
    setSessionName("");
    onClose();
  };

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("multiMCP.title")}</DialogTitle>
          <DialogDescription>
            {t("multiMCP.description")}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <label className="mb-1 block text-sm font-medium">
              {t("multiMCP.nameLabel")}
            </label>
            <input
              type="text"
              value={sessionName}
              onChange={(e) => setSessionName(e.target.value)}
              placeholder={
                selectedIds.length >= 2
                  ? generateDefaultName()
                  : t("multiMCP.selectFirst")
              }
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {config.available_models.length > 1 && (
            <div>
              <label className="mb-1 block text-sm font-medium">{t("multiMCP.modelLabel")}</label>
              <select
                value={selectedModelId}
                onChange={(e) => setSelectedModelId(e.target.value)}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                {config.available_models.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="max-h-48 space-y-1 overflow-y-auto">
            {mcpServers.map((server) => {
              const isError = server.status !== "connected";
              return (
                <label
                  key={server.id}
                  className={`flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 transition-colors hover:bg-accent ${isError ? "opacity-70" : ""}`}
                >
                  <input
                    type="checkbox"
                    checked={selectedIds.includes(server.id)}
                    onChange={() => toggleServer(server.id)}
                    className="h-4 w-4 rounded border-input"
                  />
                  <div className="flex-1">
                    <div className="flex items-center gap-1.5 text-sm font-medium">
                      {server.name}
                      {isError && (
                        <span className="text-[10px] font-normal text-destructive">{t("multiMCP.disconnected")}</span>
                      )}
                    </div>
                    {server.description && (
                      <div className="text-xs text-muted-foreground">
                        {server.description}
                      </div>
                    )}
                  </div>
                  <Badge variant="secondary" className="gap-1 text-[10px]">
                    <Circle className={`h-1.5 w-1.5 fill-current ${isError ? "text-red-400" : "text-green-400"}`} />
                    {server.tool_count} tools
                  </Badge>
                </label>
              );
            })}

            {mcpServers.length < 2 && (
              <p className="py-2 text-center text-xs text-muted-foreground">
                {t("multiMCP.minServers")}
              </p>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={handleCreate}
            disabled={selectedIds.length < 2}
          >
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
