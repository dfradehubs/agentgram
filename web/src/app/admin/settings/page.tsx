"use client";

import { useEffect, useState, useCallback } from "react";
import type { AppSetting } from "@/lib/types";
import { getAdminSettings, updateAdminSettings } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import { AdminTableSkeleton } from "@/components/admin/AdminTableSkeleton";

export default function AdminSettingsPage() {
  const [settings, setSettings] = useState<AppSetting[]>([]);
  const [values, setValues] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getAdminSettings();
      setSettings(data);
      setValues(Object.fromEntries(data.map((s) => [s.key, s.value])));
    } catch {
      toast.error("Failed to load settings");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const dirty = settings.some((s) => values[s.key] !== s.value);

  // Group settings by section, preserving first-seen section order.
  const sections: { name: string; items: AppSetting[] }[] = [];
  for (const s of settings) {
    const name = s.section || "General";
    let sec = sections.find((x) => x.name === name);
    if (!sec) {
      sec = { name, items: [] };
      sections.push(sec);
    }
    sec.items.push(s);
  }

  const handleSave = async () => {
    setSaving(true);
    try {
      const changed = Object.fromEntries(
        settings.filter((s) => values[s.key] !== s.value).map((s) => [s.key, values[s.key]]),
      );
      const updated = await updateAdminSettings(changed);
      setSettings(updated);
      setValues(Object.fromEntries(updated.map((s) => [s.key, s.value])));
      toast.success("Settings saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to save settings");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">General Configuration</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Runtime operational settings — no redeploy. Applied at once on this
            instance; other instances converge within ~30s.
          </p>
        </div>
        <Button onClick={handleSave} disabled={!dirty || saving}>
          {saving ? "Saving…" : "Save changes"}
        </Button>
      </div>

      {loading ? (
        <AdminTableSkeleton columns={2} rows={4} />
      ) : (
        <div className="max-w-2xl space-y-8">
          {sections.map((sec) => (
            <section key={sec.name}>
              <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                {sec.name}
              </h2>
              <div className="divide-y rounded-lg border">
                {sec.items.map((s) => (
                  <div key={s.key} className="p-4">
                    <div className="flex items-center justify-between gap-4">
                      <label htmlFor={s.key} className="text-sm font-medium">
                        {s.label}
                      </label>
                      <input
                        id={s.key}
                        type={s.type === "int" ? "number" : "text"}
                        min={s.type === "int" ? (s.min ?? 1) : undefined}
                        max={s.type === "int" && s.max ? s.max : undefined}
                        className="w-40 rounded-md border bg-background px-3 py-2 text-right text-sm"
                        value={values[s.key] ?? ""}
                        onChange={(e) => setValues((prev) => ({ ...prev, [s.key]: e.target.value }))}
                      />
                    </div>
                    <p className="mt-2 text-xs text-muted-foreground">{s.description}</p>
                    <p className="mt-1 text-xs text-muted-foreground/60">
                      Default: <code>{s.default}</code>
                      {s.type === "duration" && " — a Go duration like \"10m\" or \"90s\""}
                    </p>
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
