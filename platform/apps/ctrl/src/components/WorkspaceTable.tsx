import { SubscriptionBadge, UsageBar } from "@neokapi/ui";
import type { AdminWorkspace } from "../types";

interface WorkspaceTableProps {
  workspaces: AdminWorkspace[];
  loading: boolean;
  onRowClick: (id: string) => void;
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
    <div className="rounded-lg border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-muted/50">
            <th className="px-4 py-2 text-left font-medium">Name</th>
            <th className="px-4 py-2 text-left font-medium">Owner</th>
            <th className="px-4 py-2 text-left font-medium">Plan</th>
            <th className="px-4 py-2 text-left font-medium">Status</th>
            <th className="px-4 py-2 text-left font-medium">Credit Usage</th>
            <th className="px-4 py-2 text-right font-medium">Members</th>
            <th className="px-4 py-2 text-left font-medium">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {workspaces.map((ws) => (
            <tr
              key={ws.id}
              onClick={() => onRowClick(ws.id)}
              className="hover:bg-muted/30 cursor-pointer"
            >
              <td className="px-4 py-2 font-medium">{ws.name}</td>
              <td className="px-4 py-2 text-muted-foreground">{ws.owner_email}</td>
              <td className="px-4 py-2">
                <SubscriptionBadge plan={ws.plan} status={ws.status} />
              </td>
              <td className="px-4 py-2 text-muted-foreground">{ws.status}</td>
              <td className="px-4 py-2 w-36">
                <UsageBar used={ws.credits_used} total={ws.credits_total} compact />
              </td>
              <td className="px-4 py-2 text-right tabular-nums">{ws.member_count}</td>
              <td className="px-4 py-2 text-muted-foreground">
                {new Date(ws.created_at).toLocaleDateString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
