import { Skeleton } from "@/components/ui/skeleton";

export function AgentItemSkeleton() {
  return (
    <div className="flex items-center gap-2.5 rounded-lg px-2.5 py-2">
      {/* Avatar */}
      <Skeleton className="h-7 w-7 shrink-0 rounded-md" />
      {/* Name + description */}
      <div className="min-w-0 flex-1 space-y-1.5">
        <Skeleton className="h-3.5 w-24" />
        <Skeleton className="h-3 w-36" />
      </div>
      {/* Chevron */}
      <Skeleton className="h-3.5 w-3.5 shrink-0 rounded-sm" />
    </div>
  );
}
