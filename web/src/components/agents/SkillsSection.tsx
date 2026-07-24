"use client";

import { useEffect, useState } from "react";
import type { UserSkill, SkillDetail } from "@/lib/types";
import { getSkills, getSkillDetail } from "@/lib/api";
import { getEntityColor } from "@/lib/agent-colors";
import { MarkdownMessage } from "../chat/MarkdownMessage";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { BookOpen } from "lucide-react";
import { toast } from "sonner";

export function SkillsSection() {
  const [skills, setSkills] = useState<UserSkill[]>([]);
  const [selected, setSelected] = useState<SkillDetail | null>(null);
  const [loadingId, setLoadingId] = useState<string | null>(null);

  useEffect(() => {
    getSkills()
      .then(setSkills)
      .catch(() => {
        /* skills are optional; stay silent if unavailable */
      });
  }, []);

  const openSkill = async (id: string) => {
    setLoadingId(id);
    try {
      const detail = await getSkillDetail(id);
      setSelected(detail);
    } catch {
      toast.error("Error loading skill");
    } finally {
      setLoadingId(null);
    }
  };

  if (skills.length === 0) return null;

  return (
    <>
      <div className="mx-2.5 my-2 border-t" />
      <div className="flex items-center px-2.5 py-1">
        <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Skills
        </span>
      </div>

      {skills.map((skill) => (
        <button
          key={skill.id}
          onClick={() => openSkill(skill.id)}
          disabled={loadingId === skill.id}
          className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm text-foreground transition-all active:scale-[0.98] hover:bg-accent/50"
        >
          <div className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-md ${getEntityColor(skill.id).iconBg}`}>
            <BookOpen className={`h-3.5 w-3.5 ${getEntityColor(skill.id).iconFg}`} />
          </div>
          <div className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{skill.name}</span>
            {skill.description && (
              <span className="block truncate text-xs text-muted-foreground">
                {skill.description}
              </span>
            )}
          </div>
        </button>
      ))}

      <Dialog open={!!selected} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="max-h-[80vh] max-w-2xl overflow-hidden">
          <DialogHeader>
            <DialogTitle>{selected?.name}</DialogTitle>
            {selected?.description && (
              <DialogDescription>{selected.description}</DialogDescription>
            )}
          </DialogHeader>
          <div className="overflow-y-auto pr-1 text-sm">
            {selected && <MarkdownMessage content={selected.content} />}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
