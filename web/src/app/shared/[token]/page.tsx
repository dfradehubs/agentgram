"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { getSharedSession, cloneSharedSession } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { Loader2, AlertCircle } from "lucide-react";

export default function SharedSessionPage() {
  const { token } = useParams<{ token: string }>();
  const t = useT();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token) return;

    let cancelled = false;

    (async () => {
      try {
        // Validate access first
        const info = await getSharedSession(token);
        if (cancelled) return;

        // Auto-clone immediately
        const { session } = await cloneSharedSession(token);
        if (cancelled) return;

        sessionStorage.setItem(
          "agentgram-pending-select",
          JSON.stringify({ agentId: info.agent_id, sessionId: session.session_id })
        );
        window.location.href = "/";
      } catch (err: unknown) {
        if (cancelled) return;
        const status = (err as { status?: number }).status;
        if (status === 403) {
          setError(t("shared.noAccess"));
        } else {
          setError(t("shared.expired"));
        }
      }
    })();

    return () => { cancelled = true; };
  }, [token, t]);

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="mx-auto max-w-md rounded-xl border bg-card p-8 text-center shadow-sm">
          <AlertCircle className="mx-auto mb-4 h-12 w-12 text-destructive" />
          <p className="text-lg font-medium text-foreground">{error}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}
