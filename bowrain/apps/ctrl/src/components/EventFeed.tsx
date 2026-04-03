import { Badge } from "@neokapi/ui";
import type { BillingEvent, BillingEventType } from "../types";

const eventBadgeVariant: Record<
  BillingEventType,
  "default" | "secondary" | "destructive" | "outline"
> = {
  subscription_created: "default",
  subscription_upgraded: "default",
  subscription_downgraded: "secondary",
  subscription_canceled: "destructive",
  payment_succeeded: "default",
  payment_failed: "destructive",
  credits_purchased: "outline",
  credits_granted: "outline",
};

const eventLabel: Record<BillingEventType, string> = {
  subscription_created: "Created",
  subscription_upgraded: "Upgraded",
  subscription_downgraded: "Downgraded",
  subscription_canceled: "Canceled",
  payment_succeeded: "Payment OK",
  payment_failed: "Payment Failed",
  credits_purchased: "Credits Purchased",
  credits_granted: "Credits Granted",
};

interface EventFeedProps {
  events: BillingEvent[];
  loading: boolean;
}

export function EventFeed({ events, loading }: EventFeedProps) {
  if (loading) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        Loading events...
      </div>
    );
  }

  if (events.length === 0) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        No billing events found.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {events.map((event) => (
        <div
          key={event.id}
          className="flex items-center justify-between rounded-lg border px-4 py-3"
        >
          <div className="flex items-center gap-3">
            <Badge variant={eventBadgeVariant[event.type]}>{eventLabel[event.type]}</Badge>
            <div>
              <p className="text-sm font-medium">{event.workspace_name}</p>
              <p className="text-xs text-muted-foreground">{event.detail}</p>
            </div>
          </div>
          <span className="text-xs text-muted-foreground whitespace-nowrap">
            {new Date(event.created_at).toLocaleString()}
          </span>
        </div>
      ))}
    </div>
  );
}
