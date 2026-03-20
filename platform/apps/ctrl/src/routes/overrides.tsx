import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge, FilterBar, useSetBreadcrumb } from "@neokapi/ui";
import type { FilterToken, FilterField } from "@neokapi/ui";
import { listAllOverrides } from "../api";
import type { FeatureOverride } from "../types";

const OVERRIDE_FIELDS: FilterField[] = [
  {
    key: "status",
    label: "Status",
    hint: "filter by enabled/disabled",
    values: [
      { value: "enabled", label: "Enabled" },
      { value: "disabled", label: "Disabled" },
    ],
  },
  {
    key: "feature",
    label: "Feature",
    hint: "filter by feature flag name",
  },
];

function matchesFilters(
  override: FeatureOverride,
  filters: FilterToken[],
  search: string,
): boolean {
  for (const f of filters) {
    if (f.key === "status") {
      const wantEnabled = f.value === "enabled";
      if (override.enabled !== wantEnabled) return false;
    }
    if (f.key === "feature") {
      if (!override.feature.toLowerCase().includes(f.value.toLowerCase())) return false;
    }
  }
  if (search) {
    const q = search.toLowerCase();
    const name = (override.workspace_name ?? override.workspace_id).toLowerCase();
    if (!name.includes(q) && !override.feature.toLowerCase().includes(q)) return false;
  }
  return true;
}

export function OverridesRoute() {
  useSetBreadcrumb("Feature Overrides");
  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState("");

  const { data: overrides, isLoading } = useQuery({
    queryKey: ["admin", "overrides"],
    queryFn: () => listAllOverrides(),
  });

  const filtered = (overrides ?? []).filter((o) => matchesFilters(o, filters, search));

  return (
    <div className="mx-auto w-full max-w-5xl space-y-4">
      <p className="text-sm text-muted-foreground">
        All per-workspace feature overrides across the platform.
      </p>

      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={OVERRIDE_FIELDS}
        placeholder="Search overrides..."
      />

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full border-collapse">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                  Workspace
                </th>
                <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                  Feature
                </th>
                <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                  Status
                </th>
                <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                  Reason
                </th>
                <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                  Created By
                </th>
                <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                  Expires
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((override) => (
                <tr
                  key={override.id}
                  className="border-b border-border/50 transition-colors hover:bg-accent/50"
                >
                  <td className="px-4 py-2.5 text-sm">
                    {override.workspace_name ?? override.workspace_id}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs">{override.feature}</td>
                  <td className="px-4 py-2.5">
                    <Badge variant={override.enabled ? "default" : "secondary"}>
                      {override.enabled ? "Enabled" : "Disabled"}
                    </Badge>
                  </td>
                  <td className="px-4 py-2.5 text-sm text-muted-foreground">
                    {override.reason ?? "--"}
                  </td>
                  <td className="px-4 py-2.5 text-sm text-muted-foreground">
                    {override.created_by}
                  </td>
                  <td className="px-4 py-2.5 text-sm text-muted-foreground">
                    {override.expires_at
                      ? new Date(override.expires_at).toLocaleDateString()
                      : "Never"}
                  </td>
                </tr>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-4 py-6 text-center text-sm text-muted-foreground">
                    No feature overrides found.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
