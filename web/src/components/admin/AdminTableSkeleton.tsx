import { Skeleton } from "@/components/ui/skeleton";

interface AdminTableSkeletonProps {
  /** Number of columns in the table */
  columns?: number;
  /** Number of skeleton rows to display */
  rows?: number;
}

export function AdminTableSkeleton({ columns = 4, rows = 5 }: AdminTableSkeletonProps) {
  return (
    <div className="overflow-x-auto rounded-lg border" aria-busy="true">
      <table className="w-full text-sm">
        <thead className="bg-muted/50">
          <tr>
            {Array.from({ length: columns }, (_, i) => (
              <th key={i} className="px-4 py-3 text-left">
                <Skeleton className="h-3.5 w-20" />
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y">
          {Array.from({ length: rows }, (_, row) => (
            <tr key={row}>
              {Array.from({ length: columns }, (_, col) => (
                <td key={col} className="px-4 py-3">
                  <Skeleton
                    className="h-3.5"
                    style={{ width: `${60 + ((col * 17 + row * 7) % 40)}%` }}
                  />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
