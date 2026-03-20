import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "../../components/ui/badge";

type BillingEventType =
  | "subscription_created"
  | "subscription_upgraded"
  | "subscription_downgraded"
  | "subscription_canceled"
  | "payment_succeeded"
  | "payment_failed"
  | "credits_purchased"
  | "credits_granted";

interface BillingEvent {
  id: string;
  type: BillingEventType;
  workspace_name: string;
  detail: string;
  created_at: string;
}

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

function EventFeed({ events, loading }: EventFeedProps) {
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

const meta: Meta<typeof EventFeed> = {
  title: "Ctrl/EventFeed",
  component: EventFeed,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof EventFeed>;

const sampleEvents: BillingEvent[] = [
  {
    id: "1",
    type: "subscription_created",
    workspace_name: "Acme Corp",
    detail: "Pro plan activated",
    created_at: "2026-03-18T14:30:00Z",
  },
  {
    id: "2",
    type: "payment_failed",
    workspace_name: "Widget Inc",
    detail: "Invoice inv_123, amount=$25 USD",
    created_at: "2026-03-17T09:15:00Z",
  },
  {
    id: "3",
    type: "credits_granted",
    workspace_name: "Startup Labs",
    detail: "Granted 100K credits: support compensation",
    created_at: "2026-03-16T11:00:00Z",
  },
  {
    id: "4",
    type: "subscription_canceled",
    workspace_name: "Old Corp",
    detail: "Downgraded to free",
    created_at: "2026-03-15T16:45:00Z",
  },
  {
    id: "5",
    type: "subscription_upgraded",
    workspace_name: "Growth Inc",
    detail: "Upgraded from Pro to Team",
    created_at: "2026-03-14T08:20:00Z",
  },
];

export const Default: Story = {
  args: { events: sampleEvents, loading: false },
};

export const Loading: Story = {
  args: { events: [], loading: true },
};

export const Empty: Story = {
  args: { events: [], loading: false },
};

export const AllEventTypes: Story = {
  args: {
    loading: false,
    events: [
      {
        id: "1",
        type: "subscription_created",
        workspace_name: "WS-1",
        detail: "Created",
        created_at: "2026-03-20T00:00:00Z",
      },
      {
        id: "2",
        type: "subscription_upgraded",
        workspace_name: "WS-2",
        detail: "Upgraded",
        created_at: "2026-03-20T00:00:00Z",
      },
      {
        id: "3",
        type: "subscription_downgraded",
        workspace_name: "WS-3",
        detail: "Downgraded",
        created_at: "2026-03-20T00:00:00Z",
      },
      {
        id: "4",
        type: "subscription_canceled",
        workspace_name: "WS-4",
        detail: "Canceled",
        created_at: "2026-03-20T00:00:00Z",
      },
      {
        id: "5",
        type: "payment_succeeded",
        workspace_name: "WS-5",
        detail: "Payment OK",
        created_at: "2026-03-20T00:00:00Z",
      },
      {
        id: "6",
        type: "payment_failed",
        workspace_name: "WS-6",
        detail: "Payment Failed",
        created_at: "2026-03-20T00:00:00Z",
      },
      {
        id: "7",
        type: "credits_purchased",
        workspace_name: "WS-7",
        detail: "Purchased",
        created_at: "2026-03-20T00:00:00Z",
      },
      {
        id: "8",
        type: "credits_granted",
        workspace_name: "WS-8",
        detail: "Granted",
        created_at: "2026-03-20T00:00:00Z",
      },
    ],
  },
};
