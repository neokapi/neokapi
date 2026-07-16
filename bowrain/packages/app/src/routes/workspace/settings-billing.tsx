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
  type BillingPlan,
  type BillingPlansResponse,
  type BillingUsageBreakdown,
  type CreditLedgerEntry,
  type ModelUsage,
  type RunnerUsage,
  ModelUsageTable,
} from "@neokapi/ui";

function formatTokens(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)}K`;
  return String(value);
}

function formatCredits(value: number): string {
  return value < 0 ? "Unlimited" : formatTokens(value);
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "long",
    day: "numeric",
    year: "numeric",
  });
}

function UsageBreakdownRow({ label, value }: { label: string; value: number }) {
  const formatted = formatTokens(value);
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
  const [plans, setPlans] = useState<BillingPlansResponse | null>(null);
  const [usage, setUsage] = useState<BillingUsageBreakdown | null>(null);
  const [modelUsage, setModelUsage] = useState<ModelUsage[]>([]);
  const [runnerUsage, setRunnerUsage] = useState<RunnerUsage[]>([]);
  const [ledger, setLedger] = useState<CreditLedgerEntry[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [checkoutError, setCheckoutError] = useState<string | null>(null);

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
      .catch(() => {})
      .finally(() => setLoaded(true));
    void api
      .billingGetPlans(ws)
      .then(setPlans)
      .catch(() => {});
    void api
      .billingGetUsage(ws)
      .then(setUsage)
      .catch(() => {});
    void api
      .billingGetLedger(ws)
      .then(setLedger)
      .catch(() => {});
    void api
      .billingGetModelUsage(ws)
      .then((r) => {
        setModelUsage(r.model_usage ?? []);
        setRunnerUsage(r.runner_usage ?? []);
      })
      .catch(() => {});
  }, [api, ws]);

  const handleManageSubscription = useCallback(async () => {
    if (!ws) return;
    setCheckoutError(null);
    try {
      const { url } = await api.billingCreatePortal(ws, window.location.href);
      window.location.href = url;
    } catch (err) {
      setCheckoutError(err instanceof Error ? err.message : "Could not open the billing portal.");
    }
  }, [api, ws]);

  // The client names a plan; the server resolves the price. A failure has to be
  // shown — an upgrade button that silently does nothing is worse than one that
  // errors, because the customer concludes the product is broken and leaves.
  const handleUpgrade = useCallback(
    async (plan: BillingPlan) => {
      if (!ws) return;
      setCheckoutError(null);
      try {
        const { url } = await api.billingCreateCheckout(
          ws,
          plan,
          `${window.location.origin}/${ws}/settings/billing?success=true`,
          `${window.location.origin}/${ws}/settings/billing`,
        );
        window.location.href = url;
      } catch (err) {
        setCheckoutError(err instanceof Error ? err.message : "Could not start checkout.");
      }
    },
    [api, ws],
  );

  const handleBuyCredits = useCallback(async () => {
    if (!ws) return;
    setCheckoutError(null);
    try {
      const { url } = await api.billingBuyCredits(
        ws,
        `${window.location.origin}/${ws}/settings/billing?credits=purchased`,
        `${window.location.origin}/${ws}/settings/billing`,
      );
      window.location.href = url;
    } catch (err) {
      setCheckoutError(err instanceof Error ? err.message : "Could not start the purchase.");
    }
  }, [api, ws]);

  if (!activeWorkspace) return null;
  if (!loaded) return <SettingsSkeleton />;

  if (!overview) {
    return (
      <div className="mx-auto w-full max-w-3xl py-4">
        <Card>
          <CardHeader>
            <CardTitle>Billing</CardTitle>
            <CardDescription>Billing is not available for this workspace.</CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  const subscription = overview.subscription;
  const credits = overview.credits;
  const weekEnd = credits?.weekEnd ? new Date(credits.weekEnd) : undefined;
  const isOwner = activeWorkspace.role === "owner";

  // Only plans this deployment can actually sell, and only ones above the current
  // plan — the server decides purchasability, so no button here can 503.
  const upgradeable = (plans?.plans ?? []).filter((p) => p.purchasable && !p.current);
  const creditPack = plans?.credit_pack;

  return (
    <div className="mx-auto w-full max-w-3xl py-4 space-y-4">
      {checkoutError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {checkoutError}
        </div>
      )}

      {/* A failed payment keeps full access until Stripe's dunning finally cancels
          the subscription (AD-018). The customer needs to know that is happening —
          this banner is the only warning before the plan drops to Free. */}
      {subscription.status === "past_due" && (
        <div className="rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-sm text-warning">
          Your last payment failed. Stripe will retry it; if every retry fails, the subscription is
          canceled and the workspace drops to the Free plan.{" "}
          {isOwner && overview.stripeCustomerId
            ? "Update your payment method under Manage Subscription."
            : "Ask a workspace owner to update the payment method."}
        </div>
      )}

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
          {subscription.status === "trialing" && subscription.trialEndsAt && (
            <p className="text-sm text-muted-foreground">
              Your Pro trial ends on {formatDate(subscription.trialEndsAt)}. No card is needed —
              after that the workspace moves to the Free plan unless you subscribe.
            </p>
          )}
          {subscription.cancelAt && (
            <p className="text-sm text-red-600 dark:text-red-400">
              Cancels on {formatDate(subscription.cancelAt)}
            </p>
          )}
          {isOwner && (
            <div className="flex flex-wrap gap-2">
              {overview.stripeCustomerId && (
                <Button variant="outline" size="sm" onClick={() => void handleManageSubscription()}>
                  Manage Subscription
                </Button>
              )}
              {upgradeable.map((plan) => (
                <Button key={plan.id} size="sm" onClick={() => void handleUpgrade(plan.id)}>
                  {plan.current ? plan.name : `Switch to ${plan.name}`}
                  {plan.per_seat ? " (per seat)" : ""}
                </Button>
              ))}
            </div>
          )}
          {isOwner && upgradeable.length === 0 && plans && (
            <p className="text-sm text-muted-foreground">
              No paid plans are available on this deployment.
            </p>
          )}
        </CardContent>
      </Card>

      {/* Credit Usage */}
      {credits && weekEnd && (
        <Card>
          <CardHeader>
            <CardTitle>Weekly Credit Usage</CardTitle>
            <CardDescription>AI credits reset every Monday at 00:00 UTC</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <UsageBar
              creditsUsed={credits.creditsUsed}
              creditsTotal={credits.creditsTotal}
              weekEnd={weekEnd}
            />
            {/* The credit pack has existed server-side since AD-018 with no way to
                buy it: there was no button anywhere in the product. */}
            {isOwner && creditPack?.purchasable && (
              <div className="flex items-center justify-between gap-4 rounded-md border border-border px-3 py-2">
                <div className="text-sm">
                  <p className="text-foreground">
                    Out of credits? Add {formatCredits(creditPack.credits)} more.
                  </p>
                  <p className="text-muted-foreground">
                    Top-up credits don&apos;t expire, and are used only after your weekly allowance
                    runs out.
                  </p>
                </div>
                <Button variant="outline" size="sm" onClick={() => void handleBuyCredits()}>
                  Buy credits
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      )}

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

      {/* Token Usage by Model */}
      {(modelUsage.length > 0 || runnerUsage.length > 0) && (
        <Card>
          <CardHeader>
            <CardTitle>Usage by Model</CardTitle>
            <CardDescription>Token consumption per AI model this week</CardDescription>
          </CardHeader>
          <CardContent>
            <ModelUsageTable entries={modelUsage} runnerEntries={runnerUsage} />
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
