import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { FilterBar, useSetBreadcrumb } from "@neokapi/ui";
import type { FilterToken, FilterField } from "@neokapi/ui";
import { getUpsells } from "../api";
import { UpsellTable } from "../components/UpsellTable";
import type { UpsellOpportunity } from "../types";

const UPSELL_FIELDS: FilterField[] = [
  {
    key: "plan",
    label: "Current Plan",
    hint: "filter by current plan",
    values: [
      { value: "free", label: "Free" },
      { value: "pro", label: "Pro" },
      { value: "team", label: "Team" },
    ],
  },
  {
    key: "signal",
    label: "Signal",
    hint: "filter by upsell signal type",
    values: [
      { value: "credit_exhaustion", label: "Credit Exhaustion" },
      { value: "seat_pressure", label: "Seat Pressure" },
      { value: "feature_gate_hits", label: "Feature Gate Hits" },
      { value: "high_usage", label: "High Usage" },
      { value: "dormant_paid", label: "Dormant Paid" },
    ],
  },
];

function matchesFilters(
  upsell: UpsellOpportunity,
  filters: FilterToken[],
  search: string,
): boolean {
  for (const f of filters) {
    if (f.key === "plan" && upsell.current_plan !== f.value) return false;
    if (f.key === "signal" && upsell.signal !== f.value) return false;
  }
  if (search) {
    const q = search.toLowerCase();
    if (
      !upsell.workspace_name.toLowerCase().includes(q) &&
      !upsell.detail.toLowerCase().includes(q)
    )
      return false;
  }
  return true;
}

export function UpsellsRoute() {
  useSetBreadcrumb("Upsell Opportunities");
  const navigate = useNavigate();
  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState("");

  const { data: upsells, isLoading } = useQuery({
    queryKey: ["admin", "upsells"],
    queryFn: () => getUpsells(),
  });

  const filtered = (upsells ?? []).filter((u) => matchesFilters(u, filters, search));

  return (
    <div className="mx-auto w-full max-w-5xl space-y-4">
      <p className="text-sm text-muted-foreground">
        Workspaces that are likely candidates for an upgrade, ranked by priority.
      </p>

      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={UPSELL_FIELDS}
        placeholder="Search upsells..."
      />

      <UpsellTable
        upsells={filtered}
        loading={isLoading}
        onRowClick={(id) =>
          void navigate({ to: "/workspaces/$workspaceId", params: { workspaceId: id } })
        }
      />
    </div>
  );
}
