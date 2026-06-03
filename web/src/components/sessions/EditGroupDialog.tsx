"use client";

import { useState, useEffect } from "react";
import type { MultiAgentGroup } from "@/lib/types";
import { useAgents } from "@/hooks/useAgents";
import { useSessions } from "@/hooks/useSessions";
import { useT } from "@/lib/i18n";
import { getEntityColor } from "@/lib/agent-colors";
import { TagInput } from "@/components/admin/TagInput";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Bot, Check } from "lucide-react";
import { toast } from "sonner";

interface Props {
  isOpen: boolean;
  onClose: () => void;
  group: MultiAgentGroup;
}

export function EditGroupDialog({ isOpen, onClose, group }: Props) {
  const { agents } = useAgents();
  const { updateMultiAgentGroup } = useSessions();
  const t = useT();
  const [name, setName] = useState(group.name);
  const [selectedIds, setSelectedIds] = useState<string[]>(group.agentIds);
  const [allowedUsers, setAllowedUsers] = useState<string[]>(group.allowedUsers || []);
  const [allowedGroups, setAllowedGroups] = useState<string[]>(group.allowedGroups || []);
  const [saving, setSaving] = useState(false);

  // Sync state when group changes
  useEffect(() => {
    setName(group.name);
    setSelectedIds(group.agentIds);
    setAllowedUsers(group.allowedUsers || []);
    setAllowedGroups(group.allowedGroups || []);
  }, [group]);

  const toggleAgent = (id: string) => {
    setSelectedIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    );
  };

  const handleSave = async () => {
    if (selectedIds.length < 2 || saving) return;
    setSaving(true);
    try {
      await updateMultiAgentGroup(group.id, {
        name: name.trim() || group.name,
        agentIds: selectedIds,
        allowed_users: allowedUsers,
        allowed_groups: allowedGroups,
      });
      toast.success(t("admin.groups.saved"));
      onClose();
    } catch {
      toast.error(t("admin.groups.error"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("multiAgent.editGroup")}</DialogTitle>
          <DialogDescription>{t("multiAgent.editGroupDescription")}</DialogDescription>
        </DialogHeader>

        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t("multiAgent.sessionName")}
          className="w-full rounded-md border bg-transparent px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
        />

        <div className="max-h-48 space-y-1 overflow-y-auto">
          {agents.map((agent) => {
            const selected = selectedIds.includes(agent.id);
            const color = getEntityColor(agent.id);
            return (
              <button
                key={agent.id}
                onClick={() => toggleAgent(agent.id)}
                className={`flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                  selected ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
                }`}
              >
                <div
                  className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-white"
                  style={{ background: `linear-gradient(135deg, ${color.avatarFrom}, ${color.avatarTo})` }}
                >
                  <Bot className="h-3 w-3" />
                </div>
                <span className="flex-1 truncate">{agent.name}</span>
                {selected && <Check className="h-4 w-4 text-emerald-500" />}
              </button>
            );
          })}
        </div>

        <TagInput
          label="Allowed users"
          values={allowedUsers}
          onChange={setAllowedUsers}
          placeholder="email@example.com"
        />

        <TagInput
          label="Allowed groups"
          values={allowedGroups}
          onChange={setAllowedGroups}
          placeholder="workspace-group"
        />

        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={selectedIds.length < 2 || saving}
          >
            {saving ? t("common.creating") : t("common.save")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
