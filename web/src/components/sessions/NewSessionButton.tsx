"use client";

import { useSessions } from "@/hooks/useSessions";
import { useT } from "@/lib/i18n";
import { Plus } from "lucide-react";

interface NewSessionButtonProps {
  onNewSession?: () => void;
}

export function NewSessionButton({ onNewSession }: NewSessionButtonProps) {
  const { createNewSession } = useSessions();
  const t = useT();

  return (
    <button
      className="flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-xs text-muted-foreground transition-all active:scale-[0.98] hover:bg-accent/50 hover:text-foreground"
      onClick={onNewSession || createNewSession}
      aria-label={t("a11y.newSession")}
    >
      <Plus className="h-3.5 w-3.5" aria-hidden="true" />
      {t("a11y.newSession")}
    </button>
  );
}
