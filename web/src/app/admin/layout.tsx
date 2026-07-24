"use client";

import { useUser } from "@/hooks/useUser";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { AdminNav } from "@/components/admin/AdminNav";
import { Skeleton } from "@/components/ui/skeleton";

// Sections an editor may access; everything else under /admin is admin-only.
const EDITOR_PATHS = ["/admin/agents", "/admin/mcp", "/admin/skills"];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const { user, isAdmin, role, isLoading } = useUser();
  const router = useRouter();
  const pathname = usePathname();
  const canAccessAdmin = isAdmin || role === "editor";
  // Admins reach everything; editors only the editor sections.
  const sectionAllowed = isAdmin || EDITOR_PATHS.some((p) => pathname === p || pathname.startsWith(p + "/"));

  useEffect(() => {
    if (isLoading || !user) return;
    if (!canAccessAdmin) {
      router.replace("/");
    } else if (!sectionAllowed) {
      router.replace("/admin/agents");
    }
  }, [user, canAccessAdmin, sectionAllowed, isLoading, router]);

  if (isLoading || !canAccessAdmin || !sectionAllowed) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="space-y-3" aria-busy="true">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-4 w-40" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen bg-background">
      <AdminNav />
      <main className="flex-1 overflow-auto p-6">{children}</main>
    </div>
  );
}
