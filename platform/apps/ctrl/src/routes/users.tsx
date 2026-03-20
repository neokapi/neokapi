import { useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { Input, Badge } from "@neokapi/ui";
import { listUsers, getUser } from "../api";
import { UserTable } from "../components/UserTable";
import type { AdminUserDetail } from "../types";

export function UsersRoute() {
  const [search, setSearch] = useState("");
  const [selectedUserId, setSelectedUserId] = useState<string | null>(null);

  const { data: users, isLoading } = useQuery({
    queryKey: ["admin", "users", search],
    queryFn: () => listUsers({ q: search || undefined }),
  });

  const { data: userDetail } = useQuery({
    queryKey: ["admin", "user", selectedUserId],
    queryFn: () => getUser(selectedUserId!),
    enabled: !!selectedUserId,
  });

  const handleRowClick = useCallback((id: string) => {
    setSelectedUserId((prev) => (prev === id ? null : id));
  }, []);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Users</h2>

      <Input
        placeholder="Search by email or name..."
        value={search}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      <UserTable
        users={users ?? []}
        loading={isLoading}
        selectedUserId={selectedUserId}
        onRowClick={handleRowClick}
      />

      {selectedUserId && userDetail && <UserWorkspacesInline user={userDetail} />}
    </div>
  );
}

function UserWorkspacesInline({ user }: { user: AdminUserDetail }) {
  return (
    <div className="rounded-lg border p-4 space-y-3">
      <h3 className="text-sm font-medium">Workspace memberships for {user.email}</h3>
      <div className="divide-y rounded-md border">
        {user.workspaces.map((ws) => (
          <div key={ws.workspace_id} className="flex items-center justify-between px-4 py-2">
            <div>
              <p className="text-sm font-medium">{ws.workspace_name}</p>
              <p className="text-xs text-muted-foreground">{ws.workspace_slug}</p>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant="outline">{ws.role}</Badge>
              <Badge variant="secondary">{ws.plan}</Badge>
            </div>
          </div>
        ))}
        {user.workspaces.length === 0 && (
          <p className="px-4 py-2 text-sm text-muted-foreground">No workspaces</p>
        )}
      </div>
    </div>
  );
}
