"use client";

import React from "react";
import { useT } from "@/lib/i18n";
import { getEntityColor } from "@/lib/agent-colors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Bot,
  Brain,
  Check,
  Circle,
  Copy,
  FileDown,
  Loader2,
  Server,
  Share2,
  Users,
  Wrench,
} from "lucide-react";
import type { Agent, MCPServer, MultiAgentGroup } from "@/lib/types";
import type { LLMModel } from "@/contexts/ConfigContext";

interface ChatHeaderProps {
  // Mode flags
  isMCP: boolean;
  isMCPMulti: boolean;
  isMultiAgent: boolean;
  // Agent mode
  currentAgent: Agent | null;
  currentSessionName?: string;
  // Multi-agent mode
  multiAgentIds: string[];
  activeGroupId: string | null;
  multiAgentGroups: MultiAgentGroup[];
  getAgentName: (agentId: string) => string;
  // MCP mode
  currentMCPServer: MCPServer | null;
  multiServers: (MCPServer | undefined)[];
  selectedModelId: string;
  onModelChange: (modelId: string) => void;
  availableModels: LLMModel[];
  onShowToolsPanel: () => void;
  // Thinking toggle
  showThinking: boolean;
  onToggleThinking: () => void;
  // Actions
  hasMessages: boolean;
  onExportPDF: () => void;
  exporting: boolean;
  onCopyConversation: () => void;
  copied: boolean;
  onShare?: () => void;
  sharing?: boolean;
}

export const ChatHeader = React.memo(function ChatHeader({
  isMCP,
  isMCPMulti,
  isMultiAgent,
  currentAgent,
  currentSessionName,
  multiAgentIds,
  activeGroupId,
  multiAgentGroups,
  getAgentName,
  currentMCPServer,
  multiServers,
  selectedModelId,
  onModelChange,
  availableModels,
  onShowToolsPanel,
  showThinking,
  onToggleThinking,
  hasMessages,
  onExportPDF,
  exporting,
  onCopyConversation,
  copied,
  onShare,
  sharing,
}: ChatHeaderProps) {
  const t = useT();

  const getAgentColor = (agentId: string) => getEntityColor(agentId);

  const renderExportButtons = () => {
    if (!hasMessages) return null;
    return (
      <div className="ml-auto flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              onClick={onToggleThinking}
              className={`h-8 w-8 ${showThinking ? "" : "text-muted-foreground/50"}`}
              aria-label={t(showThinking ? "chat.hideThinking" : "chat.showThinking")}
            >
              <Brain className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="bottom" className="text-xs">
            {t(showThinking ? "chat.hideThinking" : "chat.showThinking")}
          </TooltipContent>
        </Tooltip>
        {onShare && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={onShare}
                disabled={sharing}
                className="h-8 w-8"
                aria-label={t("chat.share")}
              >
                {sharing ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Share2 className="h-3.5 w-3.5" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="text-xs">
              {t("chat.share")}
            </TooltipContent>
          </Tooltip>
        )}
        <Button
          variant="ghost"
          size="icon"
          onClick={onExportPDF}
          disabled={exporting}
          className="h-8 w-8"
          aria-label={t("chat.exportPDF")}
        >
          {exporting ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <FileDown className="h-3.5 w-3.5" />
          )}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={onCopyConversation}
          className="h-8 w-8"
          aria-label={t("chat.copyConversation")}
        >
          {copied ? (
            <Check className="h-3.5 w-3.5 text-emerald-500" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
        </Button>
      </div>
    );
  };

  if (isMultiAgent) {
    return (
      <div className="flex items-center gap-3 border-b px-6 py-3">
        <Users className="h-4 w-4 text-muted-foreground" />
        <h2 className="text-sm font-medium">
          {multiAgentGroups.find(g => g.id === activeGroupId)?.name || currentSessionName || t("multiAgent.title")}
        </h2>
        <div className="flex items-center gap-1">
          {multiAgentIds.map((id) => {
            const color = getAgentColor(id);
            return (
              <Tooltip key={id}>
                <TooltipTrigger asChild>
                  <div
                    className="flex h-5 w-5 items-center justify-center rounded-full text-white cursor-default"
                    style={{ background: `linear-gradient(135deg, ${color.avatarFrom}, ${color.avatarTo})` }}
                  >
                    <Bot className="h-2.5 w-2.5" />
                  </div>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="text-xs">
                  {getAgentName(id)}
                </TooltipContent>
              </Tooltip>
            );
          })}
        </div>
        {renderExportButtons()}
      </div>
    );
  }

  if (isMCP) {
    return (
      <div className="flex items-center gap-3 border-b px-6 py-3">
        {isMCPMulti ? (
          <>
            <Server className="h-4 w-4 text-muted-foreground" />
            <h2 className="text-sm font-medium">Multi-MCP</h2>
            {multiServers.map((s) => (
              <Badge key={s!.id} variant="outline" className="gap-1 text-xs font-normal">
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    s!.status === "connected" ? "bg-green-500" : "bg-red-500"
                  }`}
                />
                {s!.name}
              </Badge>
            ))}
          </>
        ) : (
          <>
            <Server className="h-4 w-4 text-muted-foreground" />
            <span
              className={`h-2 w-2 rounded-full ${
                currentMCPServer!.status === "connected"
                  ? "bg-green-500"
                  : currentMCPServer!.status === "error"
                    ? "bg-red-500"
                    : "bg-gray-400"
              }`}
            />
            <h2 className="text-sm font-medium">{currentMCPServer!.name}</h2>
            <Badge variant="outline" className="text-xs font-normal">
              {currentMCPServer!.transport.toUpperCase()}
            </Badge>
            <button
              onClick={onShowToolsPanel}
              aria-label={t("mcp.viewTools")}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              <Wrench className="h-3.5 w-3.5" />
              <Badge variant="secondary" className="h-4 px-1.5 text-[10px]">
                {currentMCPServer!.tool_count}
              </Badge>
            </button>
          </>
        )}
        {availableModels.length > 0 && (
          <Select value={selectedModelId} onValueChange={onModelChange}>
            <SelectTrigger size="sm" className={`h-7 w-auto gap-1 text-xs ${hasMessages ? "" : "ml-auto"}`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {availableModels.map((m) => (
                <SelectItem key={m.id} value={m.id}>
                  {m.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {renderExportButtons()}
      </div>
    );
  }

  // Agent mode header
  return (
    <div className="flex items-center gap-3 border-b px-6 py-3">
      <div className="flex items-center gap-2">
        <Bot className="h-4 w-4 text-muted-foreground" />
        <h2 className="text-sm font-medium">
          {currentSessionName || currentAgent!.name}
        </h2>
      </div>
      <div className="flex items-center gap-1.5">
        <Badge variant="outline" className="text-xs font-normal">
          {currentAgent!.protocol.toUpperCase()}
        </Badge>
        <Badge variant="outline" className="gap-1 text-xs font-normal">
          <Circle
            className={`h-1.5 w-1.5 fill-current ${
              currentAgent!.status === "healthy"
                ? "text-emerald-500"
                : "text-muted-foreground"
            }`}
          />
          {currentAgent!.status === "healthy" ? t("common.online") : currentAgent!.status}
        </Badge>
      </div>
      {renderExportButtons()}
    </div>
  );
});
