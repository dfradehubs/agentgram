"use client";

import { useState } from "react";
import { Brain, ChevronDown, ChevronRight, Loader2 } from "lucide-react";
import type { Message } from "@/lib/types";

interface ThinkingBubblesProps {
  steps: Message[];
  isStreaming?: boolean;
  expanded?: boolean;
}

export function ThinkingBubbles({ steps, isStreaming = false, expanded = true }: ThinkingBubblesProps) {
  if (steps.length === 0) return null;

  const latest = steps[steps.length - 1];

  if (expanded) {
    return (
      <div className="mb-2 rounded-lg border border-border bg-muted/50 px-3 py-2">
        <div className="mb-1 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          {isStreaming ? (
            <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
          ) : (
            <Brain className="h-3 w-3 shrink-0" />
          )}
          <span>Reasoning</span>
        </div>
        <div className="max-h-64 overflow-y-auto text-xs italic text-muted-foreground whitespace-pre-wrap">
          {latest.content}
        </div>
      </div>
    );
  }

  return <CollapsedThinking content={latest.content} isStreaming={isStreaming} />;
}

function CollapsedThinking({ content, isStreaming }: { content: string; isStreaming: boolean }) {
  const [open, setOpen] = useState(false);

  // Last sentence/line as summary
  const lines = content.trim().split("\n").filter(Boolean);
  const summary = lines[lines.length - 1]?.slice(0, 120) || "Agent reasoning";

  return (
    <div className="mb-2">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
      >
        {isStreaming ? (
          <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
        ) : open ? (
          <ChevronDown className="h-3 w-3 shrink-0" />
        ) : (
          <ChevronRight className="h-3 w-3 shrink-0" />
        )}
        <Brain className="h-3 w-3 shrink-0" />
        <span className="truncate italic">{summary}</span>
      </button>
      {open && (
        <div className="mt-1 rounded-lg border border-border bg-muted/50 px-3 py-2 max-h-64 overflow-y-auto text-xs italic text-muted-foreground whitespace-pre-wrap">
          {content}
        </div>
      )}
    </div>
  );
}
