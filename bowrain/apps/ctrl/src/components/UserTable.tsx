import { cn } from "@neokapi/ui";
import type { AdminUser } from "../types";

interface UserTableProps {
  users: AdminUser[];
  loading: boolean;
  selectedUserId: string | null;
  onRowClick: (id: string) => void;
}

export function UserTable({ users, loading, selectedUserId, onRowClick }: UserTableProps) {
  if (loading) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        Loading users...
      </div>
    );
  }

  if (users.length === 0) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        No users found.
      </div>
    );
  }

  return (
    <div className="rounded-lg border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-muted/50">
            <th className="px-4 py-2 text-left font-medium">Email</th>
            <th className="px-4 py-2 text-left font-medium">Name</th>
            <th className="px-4 py-2 text-right font-medium">Workspaces</th>
            <th className="px-4 py-2 text-left font-medium">Last Login</th>
            <th className="px-4 py-2 text-left font-medium">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {users.map((user) => (
            <tr
              key={user.id}
              onClick={() => onRowClick(user.id)}
              className={cn(
                "hover:bg-muted/30 cursor-pointer",
                selectedUserId === user.id && "bg-muted/50",
              )}
            >
              <td className="px-4 py-2 font-medium">{user.email}</td>
              <td className="px-4 py-2">{user.name}</td>
              <td className="px-4 py-2 text-right tabular-nums">{user.workspace_count}</td>
              <td className="px-4 py-2 text-muted-foreground">
                {user.last_login ? new Date(user.last_login).toLocaleDateString() : "Never"}
              </td>
              <td className="px-4 py-2 text-muted-foreground">
                {new Date(user.created_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
