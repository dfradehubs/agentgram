"use client";

import { useState, useEffect } from "react";
import { getAdminAgents, getAdminMCPServers } from "@/lib/api";
import { getEntityColor } from "@/lib/agent-colors";
import { ChevronDown, X } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";

export interface ResourceSelection {
  type: "agent" | "mcp";
  id: string;
  name: string;
}

interface ResourceSelectorProps {
  onFilterChange: (selected: ResourceSelection[]) => void;
  initialSelection?: ResourceSelection[];
}

interface ResourceOption {
  id: string;
  name: string;
  type: "agent" | "mcp";
}

const TYPE_LABELS: Record<string, string> = {
  agent: "Agents",
  mcp: "MCP Servers",
};

export function ResourceSelector({ onFilterChange, initialSelection }: ResourceSelectorProps) {
  const [resources, setResources] = useState<ResourceOption[]>([]);
  const [selected, setSelected] = useState<ResourceSelection[]>(initialSelection ?? []);

  useEffect(() => {
    async function load() {
      try {
        const [agents, mcpServers] = await Promise.all([
          getAdminAgents(),
          getAdminMCPServers(),
        ]);
        const all: ResourceOption[] = [
          ...agents.map((a) => ({ id: a.id, name: a.name, type: "agent" as const })),
          ...mcpServers.map((s) => ({ id: s.id, name: s.name, type: "mcp" as const })),
        ];
        setResources(all);

        // Backfill names for selections loaded from URL (which only have type:id)
        if (selected.some((s) => !s.name)) {
          const lookup = new Map(all.map((r) => [`${r.type}:${r.id}`, r.name]));
          const patched = selected.map((s) => ({
            ...s,
            name: s.name || lookup.get(`${s.type}:${s.id}`) || s.id,
          }));
          setSelected(patched);
          onFilterChange(patched);
        }
      } catch (err) {
        if (process.env.NODE_ENV !== "production") {
          console.error("[ResourceSelector] failed to load resources", err);
        }
      }
    }
    load();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  function isSelected(r: ResourceOption): boolean {
    return selected.some((s) => s.type === r.type && s.id === r.id);
  }

  function toggle(r: ResourceOption) {
    const next = isSelected(r)
      ? selected.filter((s) => !(s.type === r.type && s.id === r.id))
      : [...selected, { type: r.type, id: r.id, name: r.name }];
    setSelected(next);
    onFilterChange(next);
  }

  function clearAll() {
    setSelected([]);
    onFilterChange([]);
  }

  function removeOne(r: ResourceSelection) {
    const next = selected.filter((s) => !(s.type === r.type && s.id === r.id));
    setSelected(next);
    onFilterChange(next);
  }

  // Group resources by type
  const grouped = (["agent", "mcp"] as const)
    .map((type) => ({
      type,
      label: TYPE_LABELS[type],
      items: resources.filter((r) => r.type === type),
    }))
    .filter((g) => g.items.length > 0);

  const triggerLabel =
    selected.length === 0
      ? "All resources"
      : selected.length === 1
      ? selected[0].name
      : `${selected.length} selected`;

  return (
    <div className="flex items-center gap-2">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button className="flex items-center gap-2 rounded-md border bg-background px-3 py-1.5 text-sm hover:bg-muted transition-colors">
            {selected.length > 0 && (
              <div className="flex -space-x-1">
                {selected.slice(0, 3).map((s) => (
                  <span
                    key={`${s.type}-${s.id}`}
                    className="inline-block h-3 w-3 rounded-full border border-background"
                    style={{ backgroundColor: getEntityColor(s.id).avatarFrom }}
                  />
                ))}
                {selected.length > 3 && (
                  <span className="inline-flex h-3 min-w-3 items-center justify-center rounded-full border border-background bg-muted px-0.5 text-[8px]">
                    +{selected.length - 3}
                  </span>
                )}
              </div>
            )}
            {triggerLabel}
            <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-64">
          {selected.length > 0 && (
            <>
              <DropdownMenuItem
                onClick={clearAll}
                onSelect={(e) => e.preventDefault()}
                className="text-xs text-muted-foreground justify-center cursor-pointer"
              >
                Clear selection
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          {grouped.map((group, gi) => (
            <div key={group.type}>
              {gi > 0 && <DropdownMenuSeparator />}
              <DropdownMenuLabel className="text-xs text-muted-foreground">
                {group.label}
              </DropdownMenuLabel>
              {group.items.map((r) => (
                <DropdownMenuCheckboxItem
                  key={`${r.type}-${r.id}`}
                  checked={isSelected(r)}
                  onCheckedChange={() => toggle(r)}
                  onSelect={(e) => e.preventDefault()}
                >
                  <span
                    className="mr-2 inline-block h-2.5 w-2.5 rounded-full shrink-0"
                    style={{ backgroundColor: getEntityColor(r.id).avatarFrom }}
                  />
                  {r.name}
                </DropdownMenuCheckboxItem>
              ))}
            </div>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      {/* Chips for selected resources */}
      {selected.length > 0 && selected.length <= 5 && (
        <div className="flex flex-wrap gap-1">
          {selected.map((s) => (
            <button
              key={`chip-${s.type}-${s.id}`}
              onClick={() => removeOne(s)}
              className="flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs hover:bg-muted transition-colors"
            >
              <span
                className="inline-block h-2 w-2 rounded-full"
                style={{ backgroundColor: getEntityColor(s.id).avatarFrom }}
              />
              {s.name}
              <X className="h-3 w-3" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
