"use client";

import { useAgents } from "@/hooks/useAgents";
import { getEntityColor } from "@/lib/agent-colors";
import { useT } from "@/lib/i18n";
import { Bot, Check } from "lucide-react";

interface Props {
  agentIds: string[];
  selectedAgentIds: string[];
  onToggle: (agentId: string) => void;
}

export function AgentSelector({ agentIds, selectedAgentIds, onToggle }: Props) {
  const { agents } = useAgents();
  const t = useT();

  return (
    <div className="mb-2 flex items-center gap-1.5" role="group" aria-label={t("multiAgent.sendTo")}>
      <span className="text-xs text-muted-foreground" aria-hidden="true">{t("multiAgent.sendTo")}</span>
      <div className="flex flex-wrap gap-1">
        {agentIds.map((id) => {
          const agent = agents.find((a) => a.id === id);
          const color = getEntityColor(id);
          const isSelected = selectedAgentIds.includes(id);
          const name = agent?.name || id;
          return (
            <button
              key={id}
              onClick={() => onToggle(id)}
              role="checkbox"
              aria-checked={isSelected}
              aria-label={name}
              className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-colors ${
                isSelected
                  ? "ring-2 ring-ring ring-offset-1 ring-offset-background"
                  : "opacity-60 hover:opacity-100"
              }`}
              style={{
                background: `linear-gradient(135deg, ${color.avatarFrom}20, ${color.avatarTo}20)`,
                color: color.avatarFrom,
              }}
            >
              <Bot className="h-3 w-3" aria-hidden="true" />
              <span>{name}</span>
              {isSelected && <Check className="h-2.5 w-2.5" aria-hidden="true" />}
            </button>
          );
        })}
      </div>
    </div>
  );
}
