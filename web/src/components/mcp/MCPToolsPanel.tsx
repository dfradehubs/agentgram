"use client";

import { useState, useEffect } from "react";
import type { MCPTool } from "@/lib/types";
import { getMCPTools, reconnectMCPServer, getMCPOAuth2LoginURL, ApiError } from "@/lib/api";
import { useMCPContext } from "@/contexts/MCPContext";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { KeyRound, Loader2, RefreshCw, Wrench } from "lucide-react";
import { useT } from "@/lib/i18n";

interface MCPToolsPanelProps {
  isOpen: boolean;
  onClose: () => void;
}

export function MCPToolsPanel({ isOpen, onClose }: MCPToolsPanelProps) {
  const { currentMCPServer, refreshServers } = useMCPContext();
  const t = useT();
  const [tools, setTools] = useState<MCPTool[]>([]);
  const [isLoadingTools, setIsLoadingTools] = useState(false);
  const [isReconnecting, setIsReconnecting] = useState(false);
  const [needsOAuth2, setNeedsOAuth2] = useState(false);

  useEffect(() => {
    if (isOpen && currentMCPServer) {
      loadTools();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- loadTools is stable, currentMCPServer only needed for id
  }, [isOpen, currentMCPServer?.id]);

  useEffect(() => {
    const handler = (e: MessageEvent) => {
      if (e.data?.type === "mcp-oauth-complete" && e.data.server_id === currentMCPServer?.id) {
        setNeedsOAuth2(false);
        loadTools();
      }
    };
    window.addEventListener("message", handler);
    return () => window.removeEventListener("message", handler);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentMCPServer?.id]);

  const loadTools = async () => {
    if (!currentMCPServer) return;
    setIsLoadingTools(true);
    setNeedsOAuth2(false);
    try {
      const result = await getMCPTools(currentMCPServer.id);
      setTools(result);
    } catch (err: unknown) {
      const apiErr = err as { status?: number; message?: string };
      if (apiErr.status === 403) {
        setNeedsOAuth2(true);
      }
      setTools([]);
    } finally {
      setIsLoadingTools(false);
    }
  };

  const handleReconnect = async () => {
    if (!currentMCPServer) return;
    setIsReconnecting(true);
    setNeedsOAuth2(false);
    try {
      await reconnectMCPServer(currentMCPServer.id);
      await refreshServers();
      await loadTools();
    } catch (err: unknown) {
      const apiErr = err as { status?: number; message?: string };
      if (apiErr.status === 403) {
        setNeedsOAuth2(true);
      }
    } finally {
      setIsReconnecting(false);
    }
  };

  const handleOAuth2Connect = () => {
    if (!currentMCPServer) return;
    window.open(
      getMCPOAuth2LoginURL(currentMCPServer.id, window.location.href),
      "_blank",
      "width=600,height=700"
    );
  };

  if (!currentMCPServer) return null;

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Wrench className="h-4 w-4" />
            {t("mcp.toolsOf", { name: currentMCPServer.name })}
          </DialogTitle>
        </DialogHeader>

        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span
              className={`h-2 w-2 rounded-full ${
                currentMCPServer.status === "connected"
                  ? "bg-green-500"
                  : currentMCPServer.status === "error"
                    ? "bg-red-500"
                    : "bg-gray-400"
              }`}
            />
            <span className="text-sm capitalize">
              {currentMCPServer.status}
            </span>
            {currentMCPServer.status_error && (
              <span className="text-xs text-destructive">
                {currentMCPServer.status_error}
              </span>
            )}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={handleReconnect}
            disabled={isReconnecting}
          >
            {isReconnecting ? (
              <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
            ) : (
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            )}
            {t("mcp.reconnect")}
          </Button>
        </div>

        {needsOAuth2 && (
          <div className="flex items-center justify-between rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3">
            <p className="text-sm text-amber-700 dark:text-amber-400">
              This server requires OAuth2 authorization
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={handleOAuth2Connect}
              className="shrink-0 gap-1.5"
            >
              <KeyRound className="h-3 w-3" />
              Conectar
            </Button>
          </div>
        )}

        <div className="max-h-80 space-y-2 overflow-y-auto">
          {isLoadingTools ? (
            <div className="flex items-center justify-center py-8" aria-live="polite">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" aria-label={t("mcp.loadingTools")} />
            </div>
          ) : tools.length === 0 && !needsOAuth2 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              {t("mcp.noTools")}
            </p>
          ) : (
            tools.map((tool) => (
              <div
                key={tool.name}
                className="rounded-lg border bg-muted/30 px-3 py-2"
              >
                <div className="flex items-center gap-2">
                  <Wrench className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="text-sm font-medium">{tool.name}</span>
                </div>
                {tool.description && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {tool.description}
                  </p>
                )}
                {tool.inputSchema &&
                  typeof tool.inputSchema.properties === "object" &&
                  tool.inputSchema.properties != null && (
                  <div className="mt-1.5 flex flex-wrap gap-1">
                    {Object.keys(
                      tool.inputSchema.properties as Record<string, unknown>
                    ).map((param) => (
                      <Badge
                        key={param}
                        variant="secondary"
                        className="text-[10px]"
                      >
                        {param}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
