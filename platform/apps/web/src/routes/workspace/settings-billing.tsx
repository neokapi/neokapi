import { useState, useEffect, useCallback } from "react";
import {
  useWorkspace,
  useApi,
  SettingsSkeleton,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  SubscriptionBadge,
  UsageBar,
  CreditLedger,
  type BillingOverview,
  type BillingUsageBreakdown,
  type CreditLedgerEntry,
} from "@neokapi/ui";

function UsageBreakdownRow({ label, value }: { label: string; value: number }) {
  const formatted =
    value >= 1_000_000
      ? `${(value / 1_000_000).toFixed(1)}M`
      : value >= 1_000
        ? `${(value / 1_000).toFixed(0)}K`
        : String(value);
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-b-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="font-mono text-sm text-foreground">{formatted}</span>
    </div>
  );
}

export function SettingsBillingRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const ws = activeWorkspace?.slug ?? "";

  const [overview, setOverview] = useState<BillingOverview | null>(null);
  const [usage, setUsage] = useState<BillingUsageBreakdown | null>(null);
  const [ledger, setLedger] = useState<CreditLedgerEntry[]>([]);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Billing — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  useEffect(() => {
    if (!ws) return;
    void api
      .billingGetOverview(ws)
      .then(setOverview)
      .catch(() => {});
    void api
      .billingGetUsage(ws)
      .then(setUsage)
      .catch(() => {});
    void api
      .billingGetLedger(ws)
      .then(setLedger)
      .catch(() => {});
  }, [api, ws]);

  const handleManageSubscription = useCallback(async () => {
    if (!ws) return;
    try {
      const { url } = await api.billingCreatePortal(ws, window.location.href);
      window.location.href = url;
    } catch {
      // Error handling would go here
    }
  }, [api, ws]);

  const handleUpgrade = useCallback(
    async (priceId: string) => {
      if (!ws) return;
      try {
        const { url } = await api.billingCreateCheckout(
          ws,
          priceId,
          `${window.location.origin}/${ws}/settings/billing?success=true`,
          `${window.location.origin}/${ws}/settings/billing`,
        );
        window.location.href = url;
      } catch {
        // Error handling would go here
      }
    },
    [api, ws],
  );

  if (!activeWorkspace) return null;
  if (!overview) return <SettingsSkeleton />;

  const { subscription, credits } = overview;
  const weekEnd = new Date(credits.weekEnd);
  const isOwner = activeWorkspace.role === "owner";

  return (
    <div className="mx-auto w-full max-w-3xl py-4 space-y-4">
      {/* Subscription */}
      <Card>
        <CardHeader>
          <CardTitle>Subscription</CardTitle>
          <CardDescription>Your current plan and subscription status</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <SubscriptionBadge plan={subscription.plan} status={subscription.status} />
            {subscription.seatCount > 1 && (
              <span className="text-sm text-muted-foreground">{subscription.seatCount} seats</span>
            )}
          </div>
          {subscription.cancelAt && (
            <p className="text-sm text-red-600 dark:text-red-400">
              Cancels on{" "}
              {new Date(subscription.cancelAt).toLocaleDateString("en-US", {
                month: "long",
                day: "numeric",
                year: "numeric",
              })}
            </p>
          )}
          {isOwner && (
            <div className="flex gap-2">
              {subscription.plan !== "enterprise" && overview.stripeCustomerId && (
                <Button variant="outline" size="sm" onClick={() => void handleManageSubscription()}>
                  Manage Subscription
                </Button>
              )}
              {(subscription.plan === "free" || subscription.plan === "pro") && (
                <Button size="sm" onClick={() => void handleUpgrade("stripe_team_price_id")}>
                  Upgrade Plan
                </Button>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Credit Usage */}
      <Card>
        <CardHeader>
          <CardTitle>Weekly Credit Usage</CardTitle>
          <CardDescription>AI credits reset every Monday at 00:00 UTC</CardDescription>
        </CardHeader>
        <CardContent>
          <UsageBar
            creditsUsed={credits.creditsUsed}
            creditsTotal={credits.creditsTotal}
            weekEnd={weekEnd}
          />
        </CardContent>
      </Card>

      {/* Usage Breakdown */}
      {usage && (
        <Card>
          <CardHeader>
            <CardTitle>Usage Breakdown</CardTitle>
            <CardDescription>Credits consumed by operation type this week</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid">
              <UsageBreakdownRow label="AI Translation" value={usage.aiTranslation} />
              <UsageBreakdownRow label="AI Quality Check" value={usage.aiQualityCheck} />
              <UsageBreakdownRow label="@bravo Messages" value={usage.bravoMessages} />
              <UsageBreakdownRow label="@bravo Container" value={usage.bravoContainer} />
              <div className="flex items-center justify-between pt-2 mt-1 border-t border-border font-medium">
                <span className="text-sm text-foreground">Total</span>
                <span className="font-mono text-sm text-foreground">
                  {usage.total >= 1_000
                    ? `${(usage.total / 1_000).toFixed(0)}K`
                    : String(usage.total)}
                </span>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Credit Ledger */}
      <Card>
        <CardHeader>
          <CardTitle>Credit Transactions</CardTitle>
          <CardDescription>Recent credit activity for this workspace</CardDescription>
        </CardHeader>
        <CardContent>
          <CreditLedger entries={ledger} />
        </CardContent>
      </Card>
    </div>
  );
}
