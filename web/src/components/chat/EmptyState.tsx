"use client";

import { Bot, Keyboard } from "lucide-react";
import { useT } from "@/lib/i18n";

export function EmptyState() {
  const t = useT();

  return (
    <div className="flex h-full flex-col items-center justify-center p-8 animate-in fade-in duration-500">
      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-violet-500/10 to-purple-600/10">
        <Bot className="h-7 w-7 text-violet-500" />
      </div>
      <h3 className="mb-1.5 text-base font-medium">
        {t("empty.selectAgent")}
      </h3>
      <p className="max-w-xs text-center text-sm text-muted-foreground">
        {t("empty.selectAgentDescription")}
      </p>
      <div className="mt-4 flex items-center gap-1.5 text-muted-foreground/70">
        <Keyboard className="h-3.5 w-3.5" />
        <span className="text-xs">{t("empty.typeToStart")}</span>
      </div>
    </div>
  );
}
