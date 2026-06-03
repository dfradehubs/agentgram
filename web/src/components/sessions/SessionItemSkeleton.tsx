import { Skeleton } from "@/components/ui/skeleton";

export function SessionItemSkeleton() {
  return (
    <div className="flex items-center rounded-md px-2 py-1.5">
      {/* Status dot */}
      <Skeleton className="mr-2 h-1.5 w-1.5 shrink-0 rounded-full" />
      {/* Session name */}
      <Skeleton className="h-3 flex-1 max-w-[120px]" />
      {/* Relative date */}
      <Skeleton className="ml-auto h-2.5 w-6 shrink-0" />
    </div>
  );
}
