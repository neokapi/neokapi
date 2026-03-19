import { useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Input } from "@neokapi/ui";
import { listWorkspaces } from "../api";
import { WorkspaceTable } from "../components/WorkspaceTable";

const PLAN_OPTIONS = ["", "free", "pro", "team", "enterprise"] as const;
const STATUS_OPTIONS = ["", "active", "past_due", "canceled", "trialing"] as const;

export function WorkspacesRoute() {
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [planFilter, setPlanFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");

  const { data: workspaces, isLoading } = useQuery({
    queryKey: ["admin", "workspaces", search, planFilter, statusFilter],
    queryFn: () =>
      listWorkspaces({
        q: search || undefined,
        plan: planFilter || undefined,
        status: statusFilter || undefined,
      }),
  });

  const handleRowClick = useCallback(
    (id: string) => {
      void navigate({ to: "/workspaces/$workspaceId", params: { workspaceId: id } });
    },
    [navigate],
  );

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Workspaces</h2>

      <div className="flex items-center gap-3">
        <Input
          placeholder="Search workspaces..."
          value={search}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)}
          className="max-w-sm"
        />
        <select
          value={planFilter}
          onChange={(e) => setPlanFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-sm"
        >
          {PLAN_OPTIONS.map((p) => (
            <option key={p} value={p}>
              {p || "All plans"}
            </option>
          ))}
        </select>
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-sm"
        >
          {STATUS_OPTIONS.map((s) => (
            <option key={s} value={s}>
              {s || "All statuses"}
            </option>
          ))}
        </select>
      </div>

      <WorkspaceTable
        workspaces={workspaces ?? []}
        loading={isLoading}
        onRowClick={handleRowClick}
      />
    </div>
  );
}
