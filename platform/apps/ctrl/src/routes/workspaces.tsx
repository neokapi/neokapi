import { useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { FilterBar, useSetBreadcrumb } from "@neokapi/ui";
import type { FilterToken, FilterField, FilterPreset } from "@neokapi/ui";
import { listWorkspaces } from "../api";
import { WorkspaceTable } from "../components/WorkspaceTable";

const WORKSPACE_FIELDS: FilterField[] = [
  {
    key: "plan",
    label: "Plan",
    hint: "filter by subscription plan",
    values: [
      { value: "free", label: "Free" },
      { value: "pro", label: "Pro" },
      { value: "team", label: "Team" },
      { value: "enterprise", label: "Enterprise" },
    ],
  },
  {
    key: "status",
    label: "Status",
    hint: "filter by billing status",
    values: [
      { value: "active", label: "Active" },
      { value: "past_due", label: "Past Due" },
      { value: "canceled", label: "Canceled" },
      { value: "trialing", label: "Trialing" },
    ],
  },
];

const WORKSPACE_PRESETS: FilterPreset[] = [
  { label: "Past due", filters: [{ key: "status", value: "past_due" }] },
  { label: "Trialing", filters: [{ key: "status", value: "trialing" }] },
];

export function WorkspacesRoute() {
  useSetBreadcrumb("Workspaces");
  const navigate = useNavigate();
  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState("");

  const planFilter = filters.find((f) => f.key === "plan")?.value;
  const statusFilter = filters.find((f) => f.key === "status")?.value;

  const { data: workspaces, isLoading } = useQuery({
    queryKey: ["admin", "workspaces", search, planFilter, statusFilter],
    queryFn: () =>
      listWorkspaces({
        q: search || undefined,
        plan: planFilter,
        status: statusFilter,
      }),
  });

  const handleRowClick = useCallback(
    (id: string) => {
      void navigate({ to: "/workspaces/$workspaceId", params: { workspaceId: id } });
    },
    [navigate],
  );

  return (
    <div className="mx-auto w-full max-w-5xl space-y-4">
      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={WORKSPACE_FIELDS}
        presets={WORKSPACE_PRESETS}
        placeholder="Search workspaces..."
      />

      <WorkspaceTable
        workspaces={workspaces ?? []}
        loading={isLoading}
        onRowClick={handleRowClick}
      />
    </div>
  );
}
