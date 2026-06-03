import { Skeleton } from "@/components/ui/skeleton";

interface ChatMessageSkeletonProps {
  /** Number of skeleton messages to render */
  count?: number;
}

function SingleMessageSkeleton({ align }: { align: "left" | "right" }) {
  if (align === "right") {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%]">
          <Skeleton className="h-10 w-48 rounded-2xl rounded-tr-sm" />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-2">
        {/* Avatar */}
        <Skeleton className="h-6 w-6 shrink-0 rounded-full" />
        {/* Agent name */}
        <Skeleton className="h-3.5 w-20" />
      </div>
      {/* Text lines of varying width */}
      <div className="ml-8 space-y-2">
        <Skeleton className="h-3 w-full max-w-[280px]" />
        <Skeleton className="h-3 w-full max-w-[220px]" />
        <Skeleton className="h-3 w-full max-w-[160px]" />
      </div>
    </div>
  );
}

export function ChatMessageSkeleton({ count = 3 }: ChatMessageSkeletonProps) {
  // Alternate between user (right) and assistant (left) messages
  const pattern: Array<"left" | "right"> = ["right", "left", "right", "left", "right"];

  return (
    <div className="space-y-4">
      {Array.from({ length: count }, (_, i) => (
        <SingleMessageSkeleton key={i} align={pattern[i % pattern.length]} />
      ))}
    </div>
  );
}
