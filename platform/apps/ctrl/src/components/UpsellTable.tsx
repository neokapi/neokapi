import { Badge } from "@neokapi/ui";
import { SubscriptionBadge } from "@neokapi/ui";
import type { UpsellOpportunity, UpsellSignal } from "../types";

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

export function UpsellTable({ upsells, loading, onRowClick }: UpsellTableProps) {
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
