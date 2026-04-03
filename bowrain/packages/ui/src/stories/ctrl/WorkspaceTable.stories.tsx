import type { Meta, StoryObj } from "@storybook/react-vite";
import type { BillingPlan, BillingStatus } from "../../types/api";
import { SubscriptionBadge } from "../../components/billing/SubscriptionBadge";

interface AdminWorkspace {
  id: string;
  name: string;
  owner_email: string;
  plan: BillingPlan;
  status: BillingStatus;
  credits_used: number;
  credits_total: number;
  member_count: number;
  created_at: string;
}

interface WorkspaceTableProps {
  workspaces: AdminWorkspace[];
  loading: boolean;
  onRowClick: (id: string) => void;
}

function WorkspaceTable({ workspaces, loading, onRowClick }: WorkspaceTableProps) {
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

const meta: Meta<typeof WorkspaceTable> = {
  title: "Ctrl/WorkspaceTable",
  component: WorkspaceTable,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof WorkspaceTable>;

const sampleWorkspaces: AdminWorkspace[] = [
  {
    id: "ws-1",
    name: "Acme Corp",
    owner_email: "admin@acme.com",
    plan: "team",
    status: "active",
    credits_used: 1500000,
    credits_total: 2000000,
    member_count: 12,
    created_at: "2025-06-15T00:00:00Z",
  },
  {
    id: "ws-2",
    name: "Widget Inc",
    owner_email: "ceo@widget.io",
    plan: "pro",
    status: "active",
    credits_used: 300000,
    credits_total: 500000,
    member_count: 3,
    created_at: "2025-09-01T00:00:00Z",
  },
  {
    id: "ws-3",
    name: "Startup Labs",
    owner_email: "dev@startup.co",
    plan: "free",
    status: "active",
    credits_used: 48000,
    credits_total: 50000,
    member_count: 1,
    created_at: "2026-01-10T00:00:00Z",
  },
  {
    id: "ws-4",
    name: "Old Corp",
    owner_email: "legacy@old.com",
    plan: "pro",
    status: "canceled",
    credits_used: 0,
    credits_total: 500000,
    member_count: 2,
    created_at: "2024-11-20T00:00:00Z",
  },
  {
    id: "ws-5",
    name: "Enterprise Co",
    owner_email: "ops@enterprise.com",
    plan: "enterprise",
    status: "active",
    credits_used: 5000000,
    credits_total: 10000000,
    member_count: 50,
    created_at: "2025-01-01T00:00:00Z",
  },
];

export const Default: Story = {
  args: { workspaces: sampleWorkspaces, loading: false, onRowClick: () => {} },
};

export const Loading: Story = {
  args: { workspaces: [], loading: true, onRowClick: () => {} },
};

export const Empty: Story = {
  args: { workspaces: [], loading: false, onRowClick: () => {} },
};

export const SingleWorkspace: Story = {
  args: {
    workspaces: [sampleWorkspaces[0]],
    loading: false,
    onRowClick: () => {},
  },
};
