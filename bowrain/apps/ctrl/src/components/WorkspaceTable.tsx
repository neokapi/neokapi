import { SubscriptionBadge, cn } from "@neokapi/ui";
import type { BillingPlan, BillingStatus } from "@neokapi/ui";
import type { AdminWorkspace } from "../types";

interface WorkspaceTableProps {
  workspaces: AdminWorkspace[];
  loading: boolean;
  onRowClick: (id: string) => void;
}

function CreditBar({ used, total }: { used: number; total: number }) {
  const pct = total > 0 ? Math.min((used / total) * 100, 100) : 0;
  const barColor =
    pct > 80
      ? "bg-red-500 dark:bg-red-400"
      : pct > 60
        ? "bg-yellow-500 dark:bg-yellow-400"
        : "bg-green-500 dark:bg-green-400";

  return (
    <div className="space-y-1">
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div className={cn("h-full rounded-full", barColor)} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-[11px] text-muted-foreground">{Math.round(pct)}%</span>
    </div>
  );
}

export function WorkspaceTable({ workspaces, loading, onRowClick }: WorkspaceTableProps) {
  if (loading) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        Loading workspaces...
      </div>
    );
  }

  if (workspaces.length === 0) {
    return (
      <div className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
        No workspaces found.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full border-collapse">
        <thead>
          <tr className="border-b border-border">
            <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
              Name
            </th>
            <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
              Owner
            </th>
            <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
              Plan
            </th>
            <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
              Usage
            </th>
            <th className="px-4 py-2.5 text-right text-sm font-medium text-muted-foreground">
              Members
            </th>
            <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
              Created
            </th>
          </tr>
        </thead>
        <tbody>
          {workspaces.map((ws) => (
            <tr
              key={ws.id}
              onClick={() => onRowClick(ws.id)}
              className="border-b border-border/50 transition-colors hover:bg-accent/50 cursor-pointer"
            >
              <td className="px-4 py-2.5 text-sm font-medium">{ws.name}</td>
              <td className="px-4 py-2.5 text-sm text-muted-foreground">{ws.owner_email}</td>
              <td className="px-4 py-2.5">
                <SubscriptionBadge
                  plan={ws.plan as BillingPlan}
                  status={ws.status as BillingStatus}
                />
              </td>
              <td className="px-4 py-2.5 w-28">
                <CreditBar used={ws.credits_used} total={ws.credits_total} />
              </td>
              <td className="px-4 py-2.5 text-right text-sm tabular-nums">{ws.member_count}</td>
              <td className="px-4 py-2.5 text-sm text-muted-foreground">
                {new Date(ws.created_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
