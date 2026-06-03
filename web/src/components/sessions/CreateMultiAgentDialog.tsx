"use client";

import { useState } from "react";
import { useAgents } from "@/hooks/useAgents";
import { useSessions } from "@/hooks/useSessions";
import { useAgentContext } from "@/contexts/AgentContext";
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

interface Props {
  isOpen: boolean;
  onClose: () => void;
}

export function CreateMultiAgentDialog({ isOpen, onClose }: Props) {
  const { agents } = useAgents();
  const { addMultiAgentGroup, selectGroup } = useSessions();
  const { selectAgent } = useAgentContext();
  const t = useT();
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [groupName, setGroupName] = useState("");
  const [allowedUsers, setAllowedUsers] = useState<string[]>([]);
  const [allowedGroups, setAllowedGroups] = useState<string[]>([]);

  const toggleAgent = (id: string) => {
    setSelectedIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
    );
  };

  const [creating, setCreating] = useState(false);

  const handleCreate = async () => {
    if (selectedIds.length < 2 || creating) return;
    setCreating(true);
    try {
      const group = await addMultiAgentGroup(
        groupName.trim(),
        selectedIds,
        allowedUsers.length > 0 ? allowedUsers : undefined,
        allowedGroups.length > 0 ? allowedGroups : undefined,
      );
      selectAgent(selectedIds[0]);
      selectGroup(group.id);
      setSelectedIds([]);
      setGroupName("");
      setAllowedUsers([]);
      setAllowedGroups([]);
      onClose();
    } catch {
      // Error handled silently - group creation failed
    } finally {
      setCreating(false);
    }
  };

  const handleClose = () => {
    setSelectedIds([]);
    setGroupName("");
    setAllowedUsers([]);
    setAllowedGroups([]);
    onClose();
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("multiAgent.title")}</DialogTitle>
          <DialogDescription>{t("multiAgent.description")}</DialogDescription>
        </DialogHeader>

        <input
          type="text"
          value={groupName}
          onChange={(e) => setGroupName(e.target.value)}
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
          <Button variant="outline" size="sm" onClick={handleClose}>
            {t("common.cancel")}
          </Button>
          <Button
            size="sm"
            onClick={handleCreate}
            disabled={selectedIds.length < 2 || creating}
          >
            {t("common.create")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
