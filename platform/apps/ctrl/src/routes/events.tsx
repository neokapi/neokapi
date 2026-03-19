import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Input } from "@neokapi/ui";
import { listEvents } from "../api";
import { EventFeed } from "../components/EventFeed";
import type { BillingEventType } from "../types";

const EVENT_TYPES: { value: string; label: string }[] = [
  { value: "", label: "All events" },
  { value: "subscription_created", label: "Subscription Created" },
  { value: "subscription_upgraded", label: "Subscription Upgraded" },
  { value: "subscription_downgraded", label: "Subscription Downgraded" },
  { value: "subscription_canceled", label: "Subscription Canceled" },
  { value: "payment_succeeded", label: "Payment Succeeded" },
  { value: "payment_failed", label: "Payment Failed" },
  { value: "credits_purchased", label: "Credits Purchased" },
  { value: "credits_granted", label: "Credits Granted" },
];

export function EventsRoute() {
  const [typeFilter, setTypeFilter] = useState("");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");

  const { data: events, isLoading } = useQuery({
    queryKey: ["admin", "events", typeFilter, fromDate, toDate],
    queryFn: () =>
      listEvents({
        type: (typeFilter as BillingEventType) || undefined,
        from: fromDate || undefined,
        to: toDate || undefined,
      }),
  });

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Billing Events</h2>

      <div className="flex items-center gap-3">
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-sm"
        >
          {EVENT_TYPES.map((et) => (
            <option key={et.value} value={et.value}>
              {et.label}
            </option>
          ))}
        </select>
        <Input
          type="date"
          value={fromDate}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setFromDate(e.target.value)}
          className="w-40"
          placeholder="From"
        />
        <Input
          type="date"
          value={toDate}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setToDate(e.target.value)}
          className="w-40"
          placeholder="To"
        />
      </div>

      <EventFeed events={events ?? []} loading={isLoading} />
    </div>
  );
}
