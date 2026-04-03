import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BillingPlan } from "../../types/api";
import { Badge } from "@neokapi/ui-primitives/components/ui/badge";
import { SubscriptionBadge } from "../../components/billing/SubscriptionBadge";

type UpsellSignal =
  | "credit_exhaustion"
  | "seat_pressure"
  | "feature_gate_hits"
  | "high_usage"
  | "trial_expiring"
  | "dormant_paid";

interface UpsellOpportunity {
  workspace_id: string;
  workspace_name: string;
  current_plan: BillingPlan;
  signal: UpsellSignal;
  score: number;
  detail: string;
  suggested_plan: string;
  detected_at: string;
}

const signalBadge: Record<UpsellSignal, { label: string; className: string }> = {
  credit_exhaustion: {
    label: "Credit Exhaustion",
    className: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
  },
  seat_pressure: {
    label: "Seat Pressure",
    className: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400",
  },
  feature_gate_hits: {
    label: "Feature Gate Hits",
    className: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400",
  },
  high_usage: {
    label: "High Usage",
    className: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400",
  },
  trial_expiring: {
    label: "Trial Expiring",
    className: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400",
  },
  dormant_paid: {
    label: "Dormant Paid",
    className: "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400",
  },
};

interface UpsellTableProps {
  upsells: UpsellOpportunity[];
  loading: boolean;
  onRowClick: (workspaceId: string) => void;
}

function UpsellTable({ upsells, loading, onRowClick }: UpsellTableProps) {
  if (loading) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        Loading upsell opportunities...
      </div>
    );
  }

  if (upsells.length === 0) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        No upsell opportunities detected.
      </div>
    );
  }

  return (
    <div className="rounded-lg border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-muted/50">
            <th className="px-4 py-2 text-left font-medium">Workspace</th>
            <th className="px-4 py-2 text-left font-medium">Signal</th>
            <th className="px-4 py-2 text-left font-medium">Current Plan</th>
            <th className="px-4 py-2 text-left font-medium">Suggested</th>
            <th className="px-4 py-2 text-right font-medium">Score</th>
            <th className="px-4 py-2 text-left font-medium">Detail</th>
            <th className="px-4 py-2 text-left font-medium">Detected</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {upsells.map((upsell) => {
            const signal = signalBadge[upsell.signal];
            return (
              <tr
                key={`${upsell.workspace_id}-${upsell.signal}`}
                onClick={() => onRowClick(upsell.workspace_id)}
                className="hover:bg-muted/30 cursor-pointer"
              >
                <td className="px-4 py-2 font-medium">{upsell.workspace_name}</td>
                <td className="px-4 py-2">
                  <span
                    className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${signal.className}`}
                  >
                    {signal.label}
                  </span>
                </td>
                <td className="px-4 py-2">
                  <SubscriptionBadge plan={upsell.current_plan} status="active" />
                </td>
                <td className="px-4 py-2">
                  <Badge variant="outline">{upsell.suggested_plan}</Badge>
                </td>
                <td className="px-4 py-2 text-right tabular-nums font-medium">{upsell.score}</td>
                <td className="px-4 py-2 text-muted-foreground max-w-xs truncate">
                  {upsell.detail}
                </td>
                <td className="px-4 py-2 text-muted-foreground">
                  {new Date(upsell.detected_at).toLocaleDateString()}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

const meta: Meta<typeof UpsellTable> = {
  title: "Ctrl/UpsellTable",
  component: UpsellTable,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof UpsellTable>;

const sampleUpsells: UpsellOpportunity[] = [
  {
    workspace_id: "ws-1",
    workspace_name: "Acme Corp",
    current_plan: "free",
    signal: "credit_exhaustion",
    score: 90,
    detail: "Exhausted credits 3 weeks in a row",
    suggested_plan: "pro",
    detected_at: "2026-03-18T00:00:00Z",
  },
  {
    workspace_id: "ws-2",
    workspace_name: "Widget Inc",
    current_plan: "pro",
    signal: "high_usage",
    score: 75,
    detail: "Average credit usage 92% over 3 weeks",
    suggested_plan: "team",
    detected_at: "2026-03-17T00:00:00Z",
  },
  {
    workspace_id: "ws-3",
    workspace_name: "Growth Co",
    current_plan: "pro",
    signal: "seat_pressure",
    score: 70,
    detail: "3/3 seats used, invited 2 more",
    suggested_plan: "team",
    detected_at: "2026-03-16T00:00:00Z",
  },
  {
    workspace_id: "ws-4",
    workspace_name: "Startup Labs",
    current_plan: "free",
    signal: "feature_gate_hits",
    score: 80,
    detail: "Hit feature gates 12 times this week",
    suggested_plan: "pro",
    detected_at: "2026-03-15T00:00:00Z",
  },
  {
    workspace_id: "ws-5",
    workspace_name: "Old Corp",
    current_plan: "team",
    signal: "dormant_paid",
    score: 50,
    detail: "Average credit usage 3% over 4 weeks",
    suggested_plan: "team",
    detected_at: "2026-03-14T00:00:00Z",
  },
];

export const Default: Story = {
  args: { upsells: sampleUpsells, loading: false, onRowClick: () => {} },
};

export const Loading: Story = {
  args: { upsells: [], loading: true, onRowClick: () => {} },
};

export const Empty: Story = {
  args: { upsells: [], loading: false, onRowClick: () => {} },
};

export const AllSignalTypes: Story = {
  args: {
    loading: false,
    onRowClick: () => {},
    upsells: [
      {
        workspace_id: "ws-1",
        workspace_name: "WS-1",
        current_plan: "free",
        signal: "credit_exhaustion",
        score: 90,
        detail: "Credit exhaustion detail",
        suggested_plan: "pro",
        detected_at: "2026-03-20T00:00:00Z",
      },
      {
        workspace_id: "ws-2",
        workspace_name: "WS-2",
        current_plan: "pro",
        signal: "seat_pressure",
        score: 75,
        detail: "Seat pressure detail",
        suggested_plan: "team",
        detected_at: "2026-03-20T00:00:00Z",
      },
      {
        workspace_id: "ws-3",
        workspace_name: "WS-3",
        current_plan: "free",
        signal: "feature_gate_hits",
        score: 80,
        detail: "Feature gate detail",
        suggested_plan: "pro",
        detected_at: "2026-03-20T00:00:00Z",
      },
      {
        workspace_id: "ws-4",
        workspace_name: "WS-4",
        current_plan: "pro",
        signal: "high_usage",
        score: 70,
        detail: "High usage detail",
        suggested_plan: "team",
        detected_at: "2026-03-20T00:00:00Z",
      },
      {
        workspace_id: "ws-5",
        workspace_name: "WS-5",
        current_plan: "pro",
        signal: "trial_expiring",
        score: 85,
        detail: "Trial expiring detail",
        suggested_plan: "pro",
        detected_at: "2026-03-20T00:00:00Z",
      },
      {
        workspace_id: "ws-6",
        workspace_name: "WS-6",
        current_plan: "team",
        signal: "dormant_paid",
        score: 50,
        detail: "Dormant paid detail",
        suggested_plan: "team",
        detected_at: "2026-03-20T00:00:00Z",
      },
    ],
  },
};
