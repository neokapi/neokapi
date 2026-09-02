import { useState, useEffect, useCallback, type ReactNode } from "react";
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
  ALL_OPERATIONS,
  operationLabel,
  type BillingOverview,
  type BillingPlan,
  type BillingPlansResponse,
  type CreditLedgerPage,
  type ModelUsage,
  type RunnerUsage,
  ModelUsageTable,
} from "@neokapi/ui";
import { useSearch } from "@tanstack/react-router";
import { usePlatform } from "../../platform";
import { searchPlan, searchSeats, type IntendedPlan } from "../intended-plan";

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

/** Ledger rows per page; the server caps the window at its own maximum. */
const LEDGER_PAGE_SIZE = 25;

function UsageBreakdownRow({ label, value }: { label: string; value: number }) {
  const formatted = formatTokens(value);
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-b-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="font-mono text-sm text-foreground">{formatted}</span>
    </div>
  );
}

/**
 * Prominent card shown when the user arrived from a landing "Start free trial"
 * CTA (the index route carries `?plan` through, surviving the OIDC round-trip).
 * It names the pre-selected plan and offers one-click checkout — but never forces
 * it. Purchasability is decided by the server (the plans response), so this
 * degrades to a friendly "not enabled yet" state instead of a button that 503s
 * when the deployment has no Stripe price configured (prod placeholder keys).
 */
function IntendedPlanBanner({
  plan,
  seats,
  plans,
  isOwner,
  onUpgrade,
}: {
  plan: IntendedPlan;
  seats?: number;
  plans: BillingPlansResponse | null;
  isOwner: boolean;
  onUpgrade: (plan: BillingPlan, seats?: number) => void;
}) {
  const info = plans?.plans.find((p) => p.id === plan);
  const name = info?.name ?? plan.charAt(0).toUpperCase() + plan.slice(1);

  let title = `Complete your ${name} upgrade`;
  let body: string;
  let action: ReactNode = null;

  if (info?.current) {
    title = `You're on the ${name} plan`;
    body = `You're already subscribed to ${name}, so there is nothing more to do here.`;
  } else if (info?.purchasable) {
    if (isOwner) {
      body = `You selected ${name} on bowrain.cloud. One step left: continue to checkout to activate it for this workspace.`;
      action = (
        <Button size="sm" onClick={() => onUpgrade(plan, seats)}>
          Complete your {name} upgrade
        </Button>
      );
    } else {
      body = `You selected ${name}, but only a workspace owner can complete the purchase. Ask an owner to finish the upgrade.`;
    }
  } else {
    // Billing not provisioned on this deployment (e.g. prod placeholder Stripe
    // keys): name the plan and degrade to a friendly state — never a 503 error.
    body = `You selected ${name}. Paid plans aren't enabled on this deployment yet, so there's nothing to purchase right now.`;
    title = `${name} plan`;
  }

  return (
    <Card className="border-primary/40 bg-primary/5">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{body}</CardDescription>
      </CardHeader>
      {action && <CardContent>{action}</CardContent>}
    </Card>
  );
}

export function SettingsBillingRoute() {
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const platform = usePlatform();
  const ws = activeWorkspace?.slug ?? "";

  // Plan pre-selected on the landing page and carried here by the index route's
  // plan passthrough. The route already validated it; re-coerce defensively.
  const rawSearch = useSearch({ strict: false }) as { plan?: unknown; seats?: unknown };
  const intendedPlan = searchPlan(rawSearch.plan);
  const intendedSeats = searchSeats(rawSearch.seats);

  // Stripe portal/checkout URLs are external redirects. In the browser we
  // navigate the tab; in the desktop webview that would trap the user on
  // Stripe (and Stripe cannot redirect back to the app), so open the OS browser.
  const openBilling = (url: string) => {
    if (platform.kind === "desktop") platform.openExternal(url);
    else window.location.href = url;
  };

  const [overview, setOverview] = useState<BillingOverview | null>(null);
  const [plans, setPlans] = useState<BillingPlansResponse | null>(null);
  const [modelUsage, setModelUsage] = useState<ModelUsage[]>([]);
  const [runnerUsage, setRunnerUsage] = useState<RunnerUsage[]>([]);
  // One /usage read answers both billing panels: `usage_by_operation` is summed
  // in SQL over the whole window (the operation filter does not narrow it), and
  // `entries` is the page that filter does narrow.
  const [ledgerPage, setLedgerPage] = useState<CreditLedgerPage | null>(null);
  const [ledgerLoading, setLedgerLoading] = useState(false);
  const [ledgerOperation, setLedgerOperation] = useState<string>(ALL_OPERATIONS);
  const [ledgerOffset, setLedgerOffset] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [checkoutError, setCheckoutError] = useState<string | null>(null);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Billing · ${activeWorkspace.name} · Bowrain`;
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
      .billingGetModelUsage(ws)
      .then((r) => {
        setModelUsage(r.model_usage ?? []);
        setRunnerUsage(r.runner_usage ?? []);
      })
      .catch(() => {});
  }, [api, ws]);

  useEffect(() => {
    if (!ws) return;
    setLedgerLoading(true);
    void api
      .billingGetLedgerPage(ws, {
        limit: LEDGER_PAGE_SIZE,
        offset: ledgerOffset,
        operation: ledgerOperation === ALL_OPERATIONS ? undefined : ledgerOperation,
      })
      .then(setLedgerPage)
      .catch(() => {})
      .finally(() => setLedgerLoading(false));
  }, [api, ws, ledgerOperation, ledgerOffset]);

  const handleManageSubscription = useCallback(async () => {
    if (!ws) return;
    setCheckoutError(null);
    try {
      const { url } = await api.billingCreatePortal(ws, window.location.href);
      openBilling(url);
    } catch (err) {
      setCheckoutError(err instanceof Error ? err.message : "Could not open the billing portal.");
    }
  }, [api, ws]);

  // The client names a plan; the server resolves the price. A failure has to be
  // shown — an upgrade button that silently does nothing is worse than one that
  // errors, because the customer concludes the product is broken and leaves.
  const handleUpgrade = useCallback(
    async (plan: BillingPlan, seats?: number) => {
      if (!ws) return;
      setCheckoutError(null);
      try {
        const { url } = await api.billingCreateCheckout(
          ws,
          plan,
          `${window.location.origin}/${ws}/settings/billing?success=true`,
          `${window.location.origin}/${ws}/settings/billing`,
          seats,
        );
        openBilling(url);
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
      openBilling(url);
    } catch (err) {
      setCheckoutError(err instanceof Error ? err.message : "Could not start the purchase.");
    }
  }, [api, ws]);

  if (!activeWorkspace) return null;
  if (!loaded) return <SettingsSkeleton />;

  if (!overview) {
    return (
      <div className="mx-auto w-full max-w-3xl py-4 space-y-4">
        {intendedPlan && (
          <IntendedPlanBanner
            plan={intendedPlan}
            seats={intendedSeats}
            plans={plans}
            isOwner={activeWorkspace.role === "owner"}
            onUpgrade={handleUpgrade}
          />
        )}
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
  const resetsAt = credits?.resetsAt ? new Date(credits.resetsAt) : undefined;
  // Paid plans have a monthly allowance; a Free (or still-trialing) workspace
  // draws on its one-time trial credits, surfaced via spendableCredits.
  const hasPlanAllowance = (credits?.creditsTotal ?? 0) > 0;
  const isOwner = activeWorkspace.role === "owner";

  // Only plans this deployment can actually sell, and only ones above the current
  // plan — the server decides purchasability, so no button here can 503.
  const upgradeable = (plans?.plans ?? []).filter((p) => p.purchasable && !p.current);
  const creditPack = plans?.credit_pack;

  // Per-operation credit SPEND for the window, summed by the server. Rows are
  // whatever the deployment actually charged, so a newly-added operation shows
  // up here instead of vanishing from a fixed list. Debits only, by
  // definition: a purchase is not consumption.
  const usageRows = Object.entries(ledgerPage?.usage_by_operation ?? {})
    .filter(([, value]) => value !== 0)
    .sort((a, b) => b[1] - a[1]);
  const usageTotal = usageRows.reduce((sum, [, value]) => sum + value, 0);

  // The ledger's chips filter the TABLE, so they read the window's whole
  // operation vocabulary — spend plus the purchases, grants, expiries and
  // resets. Derived from the spend breakdown they could not offer the very
  // operations the rows underneath were showing. Largest movement first, by
  // magnitude, so a big grant and a big spend rank alike.
  const ledgerOperations = Object.entries(ledgerPage?.net_by_operation ?? {})
    .sort((a, b) => Math.abs(b[1]) - Math.abs(a[1]))
    .map(([op]) => op);

  return (
    <div className="mx-auto w-full max-w-3xl py-4 space-y-4">
      {checkoutError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {checkoutError}
        </div>
      )}

      {intendedPlan && (
        <IntendedPlanBanner
          plan={intendedPlan}
          seats={intendedSeats}
          plans={plans}
          isOwner={isOwner}
          onUpgrade={handleUpgrade}
        />
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
              Your Pro trial ends on {formatDate(subscription.trialEndsAt)}. No card is needed.
              After that the workspace moves to the Free plan unless you subscribe.
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
      {credits && (
        <Card>
          <CardHeader>
            <CardTitle>{hasPlanAllowance ? "Monthly Credit Usage" : "AI Credits"}</CardTitle>
            <CardDescription>
              {hasPlanAllowance
                ? "Plan credits reset on the 1st of each month at 00:00 UTC"
                : "Your workspace draws on its one-time trial credits. Upgrade for a monthly allowance"}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {hasPlanAllowance && resetsAt ? (
              <UsageBar
                creditsUsed={credits.creditsUsed}
                creditsTotal={credits.creditsTotal}
                resetsAt={resetsAt}
              />
            ) : (
              <p className="text-sm">
                <span className="font-mono text-foreground">
                  {formatCredits(overview.spendableCredits)}
                </span>{" "}
                <span className="text-muted-foreground">
                  credits remaining. Trial credits are granted once and do not renew.
                </span>
              </p>
            )}
            {/* The credit pack has existed server-side since AD-018 with no way to
                buy it: there was no button anywhere in the product. */}
            {isOwner && creditPack?.purchasable && (
              <div className="flex items-center justify-between gap-4 rounded-md border border-border px-3 py-2">
                <div className="text-sm">
                  <p className="text-foreground">
                    Out of credits? Add {formatCredits(creditPack.credits)} more.
                  </p>
                  <p className="text-muted-foreground">
                    Top-up credits don&apos;t expire, and are drawn on only after your plan and
                    trial credits run out.
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
      {usageRows.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Usage Breakdown</CardTitle>
            <CardDescription>Credits consumed by operation type this month</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid">
              {usageRows.map(([op, value]) => (
                <UsageBreakdownRow key={op} label={operationLabel(op)} value={value} />
              ))}
              <div className="flex items-center justify-between pt-2 mt-1 border-t border-border font-medium">
                <span className="text-sm text-foreground">Total</span>
                <span className="font-mono text-sm text-foreground">
                  {usageTotal >= 1_000 ? `${(usageTotal / 1_000).toFixed(0)}K` : String(usageTotal)}
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
            <CardDescription>Token consumption per AI model this month</CardDescription>
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
          <CreditLedger
            entries={ledgerPage?.entries ?? []}
            operations={ledgerOperations}
            operation={ledgerOperation}
            onOperationChange={(op) => {
              setLedgerOperation(op);
              setLedgerOffset(0);
            }}
            total={ledgerPage?.total ?? 0}
            limit={ledgerPage?.limit || LEDGER_PAGE_SIZE}
            offset={ledgerPage?.offset ?? 0}
            onOffsetChange={setLedgerOffset}
            loading={ledgerLoading}
          />
        </CardContent>
      </Card>
    </div>
  );
}
