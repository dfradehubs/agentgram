"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Bot, Server, Users, UsersRound, ArrowLeft, Brain, BarChart3, Settings, KeyRound, BookOpen, ScrollText } from "lucide-react";
import { cn } from "@/lib/utils";

const navItems = [
  { href: "/admin/agents", label: "Agents", icon: Bot },
  { href: "/admin/groups", label: "Groups", icon: UsersRound },
  { href: "/admin/mcp", label: "MCP Servers", icon: Server },
  { href: "/admin/skills", label: "Skills", icon: BookOpen },
  { href: "/admin/providers", label: "Providers", icon: KeyRound },
  { href: "/admin/llm", label: "LLM Models", icon: Brain },
  { href: "/admin/users", label: "Users", icon: Users },
  { href: "/admin/observability", label: "Observability", icon: BarChart3 },
  { href: "/admin/audit", label: "Audit", icon: ScrollText },
  { href: "/admin/settings", label: "General Configuration", icon: Settings },
];

export function AdminNav() {
  const pathname = usePathname();

  return (
    <nav className="flex w-56 flex-col border-r bg-muted/30 p-4">
      <Link
        href="/"
        className="mb-6 flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to chat
      </Link>

      <h2 className="mb-4 text-lg font-semibold">Admin</h2>

      <div className="flex flex-col gap-1">
        {navItems.map((item) => {
          const Icon = item.icon;
          const active = pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors",
                active
                  ? "bg-primary/10 font-medium text-primary"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              <Icon className="h-4 w-4" />
              {item.label}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
