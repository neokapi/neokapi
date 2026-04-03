import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Tabs, TabsList, TabsTrigger, TabsContent, Badge, Button } from "@neokapi/ui";
import {
  SubscriptionBadge,
  CreditLedger,
  ModelUsageTable,
  cn,
  useSetBreadcrumb,
} from "@neokapi/ui";
import type { BillingPlan, BillingStatus, CreditLedgerEntry } from "@neokapi/ui";
import { ExternalLink, UserPlus } from "lucide-react";
import {
  getWorkspace,
  getFeatureOverrides,
  getNotes,
  getLedger,
  getModelUsage,
  impersonateWorkspace,
} from "../api";
import { ChangePlanDialog } from "../components/ChangePlanDialog";
import { GrantCreditsDialog } from "../components/GrantCreditsDialog";
import { FeatureOverrideDialog } from "../components/FeatureOverrideDialog";
import { AddNoteDialog } from "../components/AddNoteDialog";
import { AddMemberDialog } from "../components/AddMemberDialog";

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

function toLedgerEntries(
  entries: {
    id: string;
    amount: number;
    balance_after: number;
    operation: string;
    reference_id: string | null;
    created_at: string;
  }[],
): CreditLedgerEntry[] {
  return entries.map((e) => ({
    id: e.id,
    amount: e.amount,
    balanceAfter: e.balance_after,
    operation: e.operation,
    referenceId: e.reference_id ?? undefined,
    createdAt: e.created_at,
  }));
}

export function WorkspaceDetailRoute() {
  const { workspaceId } = useParams({ strict: false });
  const [showChangePlan, setShowChangePlan] = useState(false);
  const [showGrantCredits, setShowGrantCredits] = useState(false);
  const [showFeatureOverride, setShowFeatureOverride] = useState(false);
  const [showAddNote, setShowAddNote] = useState(false);
  const [showAddMember, setShowAddMember] = useState(false);

  const impersonate = useMutation({
    mutationFn: () => impersonateWorkspace(workspaceId!),
    onSuccess: (data) => {
      void navigator.clipboard.writeText(data.token);
      window.open(data.url, "_blank");
    },
  });

  const { data: workspace, isLoading } = useQuery({
    queryKey: ["admin", "workspace", workspaceId],
    queryFn: () => getWorkspace(workspaceId!),
    enabled: !!workspaceId,
  });

  const { data: overrides } = useQuery({
    queryKey: ["admin", "workspace", workspaceId, "overrides"],
    queryFn: () => getFeatureOverrides(workspaceId!),
    enabled: !!workspaceId,
  });

  const { data: notes } = useQuery({
    queryKey: ["admin", "workspace", workspaceId, "notes"],
    queryFn: () => getNotes(workspaceId!),
    enabled: !!workspaceId,
  });

  const { data: ledger } = useQuery({
    queryKey: ["admin", "workspace", workspaceId, "ledger"],
    queryFn: () => getLedger(workspaceId!),
    enabled: !!workspaceId,
  });

  const { data: modelUsageData } = useQuery({
    queryKey: ["admin", "workspace", workspaceId, "model-usage"],
    queryFn: () => getModelUsage(workspaceId!),
    enabled: !!workspaceId,
  });

  useSetBreadcrumb(workspace?.name ?? "Workspace");

  if (isLoading || !workspace) {
    return (
      <div className="flex items-center justify-center h-64 text-muted-foreground text-sm">
        Loading workspace...
      </div>
    );
  }

  const stripeUrl = workspace.stripe_customer_id
    ? `https://dashboard.stripe.com/customers/${workspace.stripe_customer_id}`
    : null;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <h2 className="text-xl font-semibold">{workspace.name}</h2>
          <p className="text-sm text-muted-foreground">{workspace.owner_email}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => setShowChangePlan(true)}>
            Change Plan
          </Button>
          {stripeUrl && (
            <Button variant="outline" size="sm" asChild>
              <a href={stripeUrl} target="_blank" rel="noopener noreferrer">
                Open in Stripe <ExternalLink className="ml-1 h-3 w-3" />
              </a>
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => impersonate.mutate()}
            disabled={impersonate.isPending}
          >
            {impersonate.isPending ? "Creating token..." : "View as customer"}
            {!impersonate.isPending && <ExternalLink className="ml-1 h-3 w-3" />}
          </Button>
        </div>
      </div>
      {impersonate.isSuccess && (
        <div className="rounded-md border border-blue-200 bg-blue-50 p-3 text-sm dark:border-blue-800 dark:bg-blue-950">
          Token copied to clipboard. Expires{" "}
          {new Date(impersonate.data.expires_at).toLocaleTimeString()}. Use as{" "}
          <code className="text-xs bg-muted px-1 rounded">
            Authorization: Bearer {impersonate.data.token.slice(0, 12)}...
          </code>
        </div>
      )}

      <div className="grid grid-cols-4 gap-4">
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Plan</p>
          <SubscriptionBadge
            plan={workspace.plan as BillingPlan}
            status={workspace.status as BillingStatus}
          />
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Credit Usage</p>
          <CreditBar used={workspace.credits_used} total={workspace.credits_total} />
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Members</p>
          <p className="text-lg font-semibold">
            {workspace.member_count} /{" "}
            {workspace.seat_count === -1 ? "unlimited" : workspace.seat_count}
          </p>
        </div>
        <div className="rounded-lg border p-4 space-y-1">
          <p className="text-sm text-muted-foreground">Created</p>
          <p className="text-sm">{new Date(workspace.created_at).toLocaleDateString()}</p>
        </div>
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="credits">Credits</TabsTrigger>
          <TabsTrigger value="usage">Model Usage</TabsTrigger>
          <TabsTrigger value="overrides">Overrides</TabsTrigger>
          <TabsTrigger value="notes">Notes</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4 space-y-4">
          <div className="rounded-lg border">
            <div className="flex items-center justify-between p-4 border-b">
              <h3 className="text-sm font-medium">Members</h3>
              <Button variant="outline" size="sm" onClick={() => setShowAddMember(true)}>
                <UserPlus className="mr-1 h-3 w-3" /> Add Member
              </Button>
            </div>
            <div className="divide-y">
              {(workspace.members ?? []).map((member) => (
                <div key={member.user_id} className="flex items-center justify-between px-4 py-3">
                  <div>
                    <p className="text-sm font-medium">{member.name}</p>
                    <p className="text-xs text-muted-foreground">{member.email}</p>
                  </div>
                  <Badge variant="outline">{member.role}</Badge>
                </div>
              ))}
              {(workspace.members ?? []).length === 0 && (
                <p className="px-4 py-3 text-sm text-muted-foreground">No members</p>
              )}
            </div>
          </div>

          {workspace.current_period_start && workspace.current_period_end && (
            <div className="rounded-lg border p-4 space-y-2">
              <h3 className="text-sm font-medium">Billing Period</h3>
              <p className="text-sm text-muted-foreground">
                {new Date(workspace.current_period_start).toLocaleDateString()} &mdash;{" "}
                {new Date(workspace.current_period_end).toLocaleDateString()}
              </p>
              {workspace.cancel_at && (
                <p className="text-sm text-destructive">
                  Cancels on {new Date(workspace.cancel_at).toLocaleDateString()}
                </p>
              )}
            </div>
          )}
        </TabsContent>

        <TabsContent value="credits" className="mt-4 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">Credit Ledger</h3>
            <Button variant="outline" size="sm" onClick={() => setShowGrantCredits(true)}>
              Grant Credits
            </Button>
          </div>
          <CreditLedger entries={toLedgerEntries(ledger ?? [])} />
        </TabsContent>

        <TabsContent value="usage" className="mt-4 space-y-4">
          <h3 className="text-sm font-medium">Usage by Model &amp; Runner</h3>
          <ModelUsageTable
            entries={modelUsageData?.model_usage ?? []}
            runnerEntries={modelUsageData?.runner_usage ?? []}
          />
        </TabsContent>

        <TabsContent value="overrides" className="mt-4 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">Feature Overrides</h3>
            <Button variant="outline" size="sm" onClick={() => setShowFeatureOverride(true)}>
              Add Override
            </Button>
          </div>
          <div className="rounded-lg border">
            <div className="divide-y">
              {(overrides ?? []).map((override) => (
                <div key={override.id} className="flex items-center justify-between px-4 py-3">
                  <div>
                    <p className="text-sm font-medium">{override.feature}</p>
                    <p className="text-xs text-muted-foreground">
                      {override.reason ?? "No reason"} &middot; by {override.created_by}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant={override.enabled ? "default" : "secondary"}>
                      {override.enabled ? "Enabled" : "Disabled"}
                    </Badge>
                    {override.expires_at && (
                      <span className="text-xs text-muted-foreground">
                        expires {new Date(override.expires_at).toLocaleDateString()}
                      </span>
                    )}
                  </div>
                </div>
              ))}
              {(!overrides || overrides.length === 0) && (
                <p className="px-4 py-3 text-sm text-muted-foreground">No feature overrides</p>
              )}
            </div>
          </div>
        </TabsContent>

        <TabsContent value="notes" className="mt-4 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">Internal Notes</h3>
            <Button variant="outline" size="sm" onClick={() => setShowAddNote(true)}>
              Add Note
            </Button>
          </div>
          <div className="space-y-3">
            {(notes ?? []).map((note) => (
              <div key={note.id} className="rounded-lg border p-4 space-y-1">
                <div className="flex items-center justify-between">
                  <p className="text-xs font-medium">{note.author_email}</p>
                  <p className="text-xs text-muted-foreground">
                    {new Date(note.created_at).toLocaleString()}
                  </p>
                </div>
                <p className="text-sm">{note.content}</p>
              </div>
            ))}
            {(!notes || notes.length === 0) && (
              <p className="text-sm text-muted-foreground">No notes yet</p>
            )}
          </div>
        </TabsContent>
      </Tabs>

      <ChangePlanDialog
        open={showChangePlan}
        onOpenChange={setShowChangePlan}
        workspaceId={workspace.id}
        currentPlan={workspace.plan}
      />
      <GrantCreditsDialog
        open={showGrantCredits}
        onOpenChange={setShowGrantCredits}
        workspaceId={workspace.id}
      />
      <FeatureOverrideDialog
        open={showFeatureOverride}
        onOpenChange={setShowFeatureOverride}
        workspaceId={workspace.id}
      />
      <AddNoteDialog open={showAddNote} onOpenChange={setShowAddNote} workspaceId={workspace.id} />
      <AddMemberDialog
        open={showAddMember}
        onOpenChange={setShowAddMember}
        workspaceId={workspace.id}
      />
    </div>
  );
}
