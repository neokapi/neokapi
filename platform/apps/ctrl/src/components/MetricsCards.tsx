import { SubscriptionBadge } from "@neokapi/ui";
import type { BillingPlan } from "@neokapi/ui";
import type { PlatformMetrics } from "../types";

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

export function MetricsCards({ metrics, loading }: MetricsCardsProps) {
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
          <p className="text-2xl font-semibold">{formatPercent(metrics.credit_utilization_pct)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Churn Rate</p>
          <p className="text-2xl font-semibold">{formatPercent(metrics.churn_rate)}</p>
        </div>
      </div>

      {metrics.top_workspaces && metrics.top_workspaces.length > 0 && (
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
                    <SubscriptionBadge plan={ws.plan as BillingPlan} status="active" />
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
      )}
    </div>
  );
}
