"use client";

interface UserRow {
  user_email: string;
  requests: number;
  errors: number;
  last_access: string;
}

interface UsersTableProps {
  users: UserRow[];
  onUserClick?: (email: string) => void;
  selectedUser?: string | null;
}

export function UsersTable({ users, onUserClick, selectedUser }: UsersTableProps) {
  if (!users || users.length === 0) {
    return <p className="text-sm text-muted-foreground">No users in the period</p>;
  }

  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b text-left text-muted-foreground">
            <th className="px-4 py-2 font-medium">Email</th>
            <th className="px-4 py-2 font-medium text-right">Requests</th>
            <th className="px-4 py-2 font-medium text-right">Errors</th>
            <th className="px-4 py-2 font-medium">Last access</th>
          </tr>
        </thead>
        <tbody>
          {users.map((row) => (
            <tr
              key={row.user_email}
              className={`border-b last:border-0 ${onUserClick ? "cursor-pointer hover:bg-muted/50" : ""} ${selectedUser === row.user_email ? "bg-muted/70" : ""}`}
              onClick={() => onUserClick?.(row.user_email)}
            >
              <td className="px-4 py-2 font-medium">{row.user_email}</td>
              <td className="px-4 py-2 text-right font-mono">{row.requests.toLocaleString()}</td>
              <td className="px-4 py-2 text-right font-mono">{row.errors}</td>
              <td className="px-4 py-2 text-muted-foreground">
                {new Date(row.last_access).toLocaleString("en", { dateStyle: "short", timeStyle: "short" })}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
