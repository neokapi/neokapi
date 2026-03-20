import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FilterBar, useSetBreadcrumb } from "@neokapi/ui";
import type { FilterToken, FilterField, FilterPreset } from "@neokapi/ui";
import { listEvents } from "../api";
import { EventFeed } from "../components/EventFeed";
import type { BillingEventType } from "../types";

const EVENT_FIELDS: FilterField[] = [
  {
    key: "type",
    label: "Event Type",
    hint: "filter by billing event type",
    values: [
      { value: "subscription_created", label: "Subscription Created" },
      { value: "subscription_upgraded", label: "Subscription Upgraded" },
      { value: "subscription_downgraded", label: "Subscription Downgraded" },
      { value: "subscription_canceled", label: "Subscription Canceled" },
      { value: "payment_succeeded", label: "Payment Succeeded" },
      { value: "payment_failed", label: "Payment Failed" },
      { value: "credits_purchased", label: "Credits Purchased" },
      { value: "credits_granted", label: "Credits Granted" },
    ],
  },
];

const EVENT_PRESETS: FilterPreset[] = [
  { label: "Failed payments", filters: [{ key: "type", value: "payment_failed" }] },
  { label: "Cancellations", filters: [{ key: "type", value: "subscription_canceled" }] },
];

export function EventsRoute() {
  useSetBreadcrumb("Billing Events");
  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState("");

  const typeFilter = filters.find((f) => f.key === "type")?.value;

  const { data: events, isLoading } = useQuery({
    queryKey: ["admin", "events", typeFilter],
    queryFn: () =>
      listEvents({
        type: (typeFilter as BillingEventType) || undefined,
      }),
  });

  return (
    <div className="mx-auto w-full max-w-5xl space-y-4">
      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={EVENT_FIELDS}
        presets={EVENT_PRESETS}
        placeholder="Search events..."
      />

      <EventFeed events={events ?? []} loading={isLoading} />
    </div>
  );
}
