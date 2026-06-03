"use client";

import type { Session } from "@/lib/types";
import { useMCPContext } from "@/contexts/MCPContext";
import { useBackgroundStreamContext } from "@/contexts/BackgroundStreamContext";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Loader2, MoreHorizontal, Trash2 } from "lucide-react";
import { useT } from "@/lib/i18n";

interface MultiMCPItemProps {
  session: Session;
}

export function MultiMCPItem({ session }: MultiMCPItemProps) {
  const { currentMultiMCPSession, selectMultiMCPSession, deleteMultiMCPSession } =
    useMCPContext();
  const { isSessionRunning, stopStream } = useBackgroundStreamContext();
  const t = useT();

  const isSelected = currentMultiMCPSession?.session_id === session.session_id;

  return (
    <div
      className={`group flex items-center rounded-md px-2 py-1 ${
        isSelected ? "bg-accent" : "hover:bg-accent/50"
      }`}
    >
      <button
        onClick={() => selectMultiMCPSession(session.session_id)}
        className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
      >
        <span className="truncate text-xs text-foreground">
          {session.session_name}
        </span>
        {isSessionRunning(session.session_id) && (
          <Loader2 className="h-3 w-3 animate-spin text-primary shrink-0" />
        )}
      </button>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button className="ml-1 shrink-0 rounded p-0.5 opacity-0 hover:bg-accent group-hover:opacity-100">
            <MoreHorizontal className="h-3.5 w-3.5 text-muted-foreground" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-32">
          <DropdownMenuItem
            onClick={() => { stopStream(session.session_id); deleteMultiMCPSession(session.session_id); }}
            className="text-destructive focus:text-destructive"
          >
            <Trash2 className="mr-2 h-3.5 w-3.5" />
            {t("common.delete")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
