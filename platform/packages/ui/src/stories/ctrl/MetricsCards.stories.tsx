import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BillingPlan } from "../../types/api";
import { SubscriptionBadge } from "../../components/billing/SubscriptionBadge";

interface TopWorkspaceUsage {
  workspace_id: string;
  workspace_name: string;
  plan: BillingPlan;
  credits_used: number;
  credits_total: number;
}

interface PlatformMetrics {
  mrr: number;
  active_workspaces: number;
  new_signups_7d: number;
  new_signups_30d: number;
  credit_utilization_percent: number;
  churn_rate_percent: number;
  top_workspaces: TopWorkspaceUsage[];
}

function formatCurrency(amount: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(amount);
}

function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`;
}

interface MetricsCardsProps {
  metrics: PlatformMetrics | null;
  loading: boolean;
}

function MetricsCards({ metrics, loading }: MetricsCardsProps) {
  if (loading || !metrics) {
    return (
      <div className="grid grid-cols-3 gap-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="rounded-lg border p-4 space-y-2 animate-pulse">
            <div className="h-3 w-20 bg-muted rounded" />
            <div className="h-6 w-16 bg-muted rounded" />
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-3 gap-4">
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">MRR</p>
          <p className="text-2xl font-semibold">{formatCurrency(metrics.mrr)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Active Workspaces</p>
          <p className="text-2xl font-semibold">{metrics.active_workspaces}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">New Signups</p>
          <p className="text-2xl font-semibold">{metrics.new_signups_7d}</p>
          <p className="text-xs text-muted-foreground">{metrics.new_signups_30d} in last 30d</p>
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Credit Utilization</p>
          <p className="text-2xl font-semibold">
            {formatPercent(metrics.credit_utilization_percent)}
          </p>
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Churn Rate</p>
          <p className="text-2xl font-semibold">{formatPercent(metrics.churn_rate_percent)}</p>
        </div>
      </div>

      <div className="rounded-lg border">
        <div className="p-4 border-b">
          <h3 className="text-sm font-medium">Top 5 Workspaces by Usage</h3>
        </div>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-2 text-left font-medium">Workspace</th>
              <th className="px-4 py-2 text-left font-medium">Plan</th>
              <th className="px-4 py-2 text-right font-medium">Credits Used</th>
              <th className="px-4 py-2 text-right font-medium">Credits Total</th>
              <th className="px-4 py-2 text-right font-medium">Usage</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {metrics.top_workspaces.map((ws) => (
              <tr key={ws.workspace_id} className="hover:bg-muted/30">
                <td className="px-4 py-2">{ws.workspace_name}</td>
                <td className="px-4 py-2">
                  <SubscriptionBadge plan={ws.plan} status="active" />
                </td>
                <td className="px-4 py-2 text-right tabular-nums">
                  {ws.credits_used.toLocaleString()}
                </td>
                <td className="px-4 py-2 text-right tabular-nums">
                  {ws.credits_total.toLocaleString()}
                </td>
                <td className="px-4 py-2 text-right tabular-nums">
                  {ws.credits_total > 0
                    ? formatPercent((ws.credits_used / ws.credits_total) * 100)
                    : "0%"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const meta: Meta<typeof MetricsCards> = {
  title: "Ctrl/MetricsCards",
  component: MetricsCards,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof MetricsCards>;

const sampleMetrics: PlatformMetrics = {
  mrr: 12500,
  active_workspaces: 150,
  new_signups_7d: 23,
  new_signups_30d: 87,
  credit_utilization_percent: 65.5,
  churn_rate_percent: 2.3,
  top_workspaces: [
    {
      workspace_id: "ws-1",
      workspace_name: "Acme Corp",
      plan: "team",
      credits_used: 1800000,
      credits_total: 2000000,
    },
    {
      workspace_id: "ws-2",
      workspace_name: "Widget Inc",
      plan: "pro",
      credits_used: 420000,
      credits_total: 500000,
    },
    {
      workspace_id: "ws-3",
      workspace_name: "Startup Labs",
      plan: "pro",
      credits_used: 350000,
      credits_total: 500000,
    },
    {
      workspace_id: "ws-4",
      workspace_name: "Growth Co",
      plan: "team",
      credits_used: 1200000,
      credits_total: 2000000,
    },
    {
      workspace_id: "ws-5",
      workspace_name: "Indie Dev",
      plan: "free",
      credits_used: 48000,
      credits_total: 50000,
    },
  ],
};

export const Default: Story = {
  args: { metrics: sampleMetrics, loading: false },
};

export const Loading: Story = {
  args: { metrics: null, loading: true },
};

export const HighChurn: Story = {
  args: {
    loading: false,
    metrics: {
      ...sampleMetrics,
      churn_rate_percent: 8.5,
      credit_utilization_percent: 92.1,
    },
  },
};

export const LowActivity: Story = {
  args: {
    loading: false,
    metrics: {
      mrr: 500,
      active_workspaces: 12,
      new_signups_7d: 1,
      new_signups_30d: 5,
      credit_utilization_percent: 15.2,
      churn_rate_percent: 0,
      top_workspaces: [
        {
          workspace_id: "ws-1",
          workspace_name: "Solo Dev",
          plan: "free",
          credits_used: 5000,
          credits_total: 50000,
        },
      ],
    },
  },
};
