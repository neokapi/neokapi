import { useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge, FilterBar, useSetBreadcrumb } from "@neokapi/ui";
import { listUsers, getUser } from "../api";
import { UserTable } from "../components/UserTable";
import { useUrlFilters } from "../hooks/useUrlFilters";
import type { AdminUserDetail } from "../types";

export function UsersRoute() {
  useSetBreadcrumb("Users");
  const { filters, search, setFilters, setSearch } = useUrlFilters([], "/users");
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
    <div className="mx-auto w-full max-w-5xl space-y-4">
      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={[]}
        placeholder="Search by email..."
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
