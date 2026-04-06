# AD-031: Billing Implementation Plan — Landing Full Plan, Billing & Metering Support

## Status: Plan (pending review)

## Context

AD-030 defined the billing architecture. Significant backend work has been completed, but the system is not yet end-to-end functional. This document is the execution plan to land full billing support across Bowrain.

### What Exists Today (Implemented)

**Backend — `platform/billing/` package (fully implemented, tested):**

- `plans.go` — 4-tier plan model (Free/Pro/Team/Enterprise), feature matrix, weekly credits, `HasFeature()`, `MinimumPlanFor()`
- `types.go` — All data types (`Subscription`, `CreditAllocation`, `LedgerEntry`, `FeatureOverride`, `WorkspaceNote`, `UpsellOpportunity`, `BillingEvent`, `PlatformMetrics`)
- `store.go` — `BillingStore` interface (subscriptions, credits, ledger, feature overrides, notes, upsells, metrics, events)
- `postgres.go` — Full PostgreSQL implementation with 6 tables + migrations (`subscriptions`, `credit_allocations`, `credit_ledger`, `feature_overrides`, `workspace_notes`, `billing_events`)
- `stripe.go` — Stripe client: `CreateCustomer`, `CreateCheckoutSession`, `CreatePortalSession`, `ReportMeterEvent` (v2 Meters API), `GetInvoices`
- `webhooks.go` — 5 event handlers with signature verification (`checkout.session.completed`, `customer.subscription.updated/deleted`, `invoice.paid`, `invoice.payment_failed`)
- `credits.go` — Weekly allocation, `EnsureWeeklyAllocation`, `ContainerTimeCredits`, `TokensToCredits`, `WeekStart`/`WeekEnd`
- `middleware.go` — `PlanGuard(feature)`, `QuotaGuard(store)`, `AdminGuard(verifier)` Echo middleware
- `upsells.go` — Upsell signal detection queries
- `posthog.go` — PostHog client wrapper
- 9 test files with comprehensive unit tests

**Server — `platform/server/` (handlers exist, routes registered):**

- `handlers_billing.go` — 6 customer self-service handlers (GetBilling, GetBillingUsage, CreateCheckout, CreatePortal, GetInvoices, StripeWebhook)
- `handlers_admin.go` — Full admin API handlers (workspaces, users, metrics, events, overrides, notes, credits, upsells)
- `server.go` — Billing routes registered at `/api/v1/workspaces/:ws/billing/*`, admin routes at `/api/admin/*`, webhook at `/api/webhooks/stripe`
- Stripe client + webhook handler initialized from env vars
- Handler tests exist

**Frontend — Control Plane (`platform/apps/ctrl/`):**

- Full admin SPA: dashboard, workspace list/detail, users, events, overrides, upsells
- Components: AdminSidebar, MetricsCards, WorkspaceTable, UserTable, EventFeed, UpsellTable, ChangePlanDialog, GrantCreditsDialog, FeatureOverrideDialog, AddNoteDialog
- Admin API client, OIDC auth against bowrain-admin realm

**Frontend — Customer Self-Service (`platform/apps/web/`):**

- `routes/pricing.tsx` — Public pricing page with PlanCard + PlanComparisonTable
- `routes/workspace/settings-billing.tsx` — Billing settings with SubscriptionBadge, UsageBar, CreditLedger, usage breakdown, invoice history, checkout/portal integration

**Shared UI (`packages/ui/`):**

- Billing types exported (BillingOverview, CreditLedgerEntry, BillingUsageBreakdown, etc.)
- Components: PlanCard, PlanComparisonTable, SubscriptionBadge, UsageBar, CreditLedger

### What's NOT Connected Yet (Gaps)

These are the gaps preventing the billing system from being end-to-end functional:

#### Gap 1: PlanGuard / QuotaGuard Not Wired to Protected Routes

The middleware functions exist and are tested, but `PlanGuard` and `QuotaGuard` are **not applied** to any feature routes in `server.go`. The AD-030 design specifies them on connectors, API tokens, agent exec, etc.

#### Gap 2: No Credit Deduction in the Hot Path

`DeductCredits()` is implemented and tested, but never called from the AI tool execution, @bravo agent, or any operation handler. The `jobs/quota.go` `RecordUsage` does not invoke `BillingStore.DeductCredits()`. Same for agent usage tracking. This means usage is tracked but credits are never consumed.

#### Gap 3: No Stripe Meter Event Reporting from Usage

`ReportMeterEvent()` is implemented but never called from any operation. Stripe Meters will show zero usage.

#### Gap 4: Weekly Allocation Bootstrap Not Wired

`EnsureWeeklyAllocation()` exists but is never called on login, request, or cron. New workspaces won't get their weekly credit allocation.

#### Gap 5: Workspace Plan Field Not Synced

Webhooks update the `subscriptions` table, but the `workspaces.plan` column (the cached plan read by `PlanGuard`) is never updated by webhook handlers. The plan field on workspace will always remain "free".

#### Gap 6: No Stripe Customer Creation Flow

`CreateCustomer()` exists but the checkout handler doesn't create a new Stripe customer when one doesn't exist. It passes an empty `customerID` to `CreateCheckoutSession`, which will fail.

#### Gap 7: No Trial Support

AD-030 mentions trials as an open question. Stripe supports trial periods natively, but no trial logic is implemented.

#### Gap 8: No Email Notifications for Billing Events

AD-030 specifies 5 email triggers (80% credits, exhausted, weekly reset, payment failed, subscription change). None are implemented.

#### Gap 9: No Seat Enforcement

`PlanLimits` defines `max-seats` but no middleware or service-layer check enforces the seat count when adding workspace members.

#### Gap 10: No Project Limit Enforcement

`PlanLimits` defines `max-projects` but no check enforces this when creating projects.

#### Gap 11: PostHog Events Not Fired from Billing Operations

PostHog client exists but no conversion events are tracked (viewed pricing, started checkout, completed checkout, hit feature gate).

#### Gap 12: No Stripe Products/Prices Setup Documentation or Automation

The Stripe dashboard needs products, prices, and meters configured. No Terraform, Stripe CLI seed script, or documentation exists.

#### Gap 13: No Keycloak Admin Realm Config

AD-030 specifies `docker/keycloak/admin-realm.json` for local dev. This file does not exist yet.

#### Gap 14: No Credit Pack Purchase Flow

Pro/Team overage handling mentions $5 credit packs. No one-time purchase checkout flow is implemented.

#### Gap 15: No Graceful Degradation When Billing Is Disabled

For self-hosted or development deployments without Stripe, there's no clean way to run without billing. The `BillingStore == nil` checks exist in handlers but PlanGuard/QuotaGuard would panic.

---

## Implementation Plan

### Phase 1: Wire the Core Billing Loop (Critical Path)

Make billing actually work end-to-end: credits deducted, plans enforced, Stripe in sync.

#### 1.1 Wire Workspace Plan Sync from Webhooks

**Files:** `platform/billing/webhooks.go`, `platform/server/server.go`

After `UpsertSubscription()` in each webhook handler, also update the `workspaces.plan` and `workspaces.stripe_customer_id` columns. This requires the webhook handler to have access to `AuthStore.UpdateWorkspace()`.

```go
// In WebhookHandler, add authStore field
type WebhookHandler struct {
    store         BillingStore
    authStore     auth.AuthStore   // NEW
    webhookSecret string
}

// After UpsertSubscription in handleCheckoutCompleted:
authStore.UpdateWorkspace(ctx, workspaceID, map[string]any{
    "plan":               string(sub.Plan),
    "stripe_customer_id": sub.StripeCustomerID,
})
```

#### 1.2 Wire Credit Deduction into AI Operations

**Files:** `platform/jobs/quota.go`, `platform/service/agent.go` (or wherever @bravo records usage)

After `RecordUsage()`, call `BillingStore.DeductCredits()`:

```go
// In quota.go, after recording usage:
if billingStore != nil {
    _ = billingStore.DeductCredits(ctx, workspaceID, billing.TokensToCredits(tokens), "ai_translation", jobID)
}
```

Same for @bravo agent container time:

```go
if billingStore != nil {
    _ = billingStore.DeductCredits(ctx, workspaceID, billing.ContainerTimeCredits(duration), "bravo_container", conversationID)
}
```

#### 1.3 Wire Stripe Meter Event Reporting

**Files:** `platform/jobs/quota.go`, `platform/service/agent.go`

After credit deduction, report to Stripe Meters (fire-and-forget):

```go
if stripeClient != nil {
    go stripeClient.ReportMeterEvent(ctx, sub.StripeCustomerID, "ai_token_usage", int64(tokens), map[string]string{
        "workspace_id":  workspaceID,
        "operation_type": "ai_translation",
    })
}
```

#### 1.4 Wire Weekly Allocation Bootstrap

**Files:** `platform/server/middleware_auth.go` or `platform/server/server.go`

Call `EnsureWeeklyAllocation()` lazily when `QuotaGuard` or `HandleGetBilling` is hit. The cleanest approach: make `QuotaGuard` call it if `CheckCredits` returns an allocation-not-found error.

Alternatively, add a call in `WorkspaceAccessMiddleware` (runs on every authenticated workspace request):

```go
// After loading workspace, ensure weekly allocation exists
if ws.Plan != "" {
    billing.EnsureWeeklyAllocation(ctx, billingStore, ws.ID, billing.Plan(ws.Plan))
}
```

#### 1.5 Fix Stripe Customer Creation in Checkout Flow

**File:** `platform/server/handlers_billing.go`

When no existing Stripe customer exists, create one before creating the checkout session:

```go
if customerID == "" {
    user := auth.UserFromContext(c)
    ws := auth.WorkspaceFromContext(c)
    customerID, err = s.StripeClient.CreateCustomer(ctx, wsID, user.Email, ws.Name)
    if err != nil {
        return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
    }
    // Also pass workspace_id in checkout session metadata for webhook routing
}
```

#### 1.6 Apply PlanGuard and QuotaGuard to Routes

**File:** `platform/server/server.go`

Wire middleware to protected route groups per AD-030:

```go
// Git connectors (requires Pro+)
connectors := wsSpecific.Group("/connectors")
connectors.Use(billing.PlanGuard(billing.FeatureConnectorsGit))

// API tokens (requires Pro+)
apiTokens := wsSpecific.Group("/api-tokens")
apiTokens.Use(billing.PlanGuard(billing.FeatureAPIAccess))

// @bravo code execution (requires Team+)
agentExec := wsSpecific.Group("/agent/exec")
agentExec.Use(billing.PlanGuard(billing.FeatureBravoCodeExec))

// AI operations (credit check)
aiOps := wsSpecific.Group("/ai")
aiOps.Use(billing.QuotaGuard(s.BillingStore))
```

Must also handle nil `BillingStore` case — when billing is not configured, PlanGuard/QuotaGuard should pass through (allow all).

### Phase 2: Enforce Limits & Seat/Project Guards

#### 2.1 Seat Count Enforcement

**Files:** `platform/service/auth.go` (or workspace member service)

Before adding a workspace member, check:

```go
members, _ := store.ListMembers(ctx, workspaceID)
limit := billing.GetLimit(billing.Plan(ws.Plan), "max-seats")
if limit > 0 && len(members) >= limit {
    return ErrSeatLimitReached
}
```

Return HTTP 403 with `upgrade_required` + `minimum_plan` for the next tier.

#### 2.2 Project Count Enforcement

**Files:** `platform/service/project.go` (or content store service)

Before creating a project:

```go
projects, _ := store.ListProjects(ctx, workspaceID)
limit := billing.GetLimit(billing.Plan(ws.Plan), "max-projects")
if limit > 0 && len(projects) >= limit {
    return ErrProjectLimitReached
}
```

#### 2.3 Credit Pack Purchase (One-Time Checkout)

**File:** `platform/server/handlers_billing.go`

Add a handler for one-time credit pack purchase using Stripe Checkout in `payment` mode:

```go
// POST /api/v1/workspaces/:ws/billing/buy-credits
func (s *Server) HandleBuyCredits(c echo.Context) error {
    // Create checkout session with mode=payment, price=STRIPE_CREDIT_PRICE_ID
    // On webhook: invoice.paid with credit pack metadata → GrantCredits
}
```

### Phase 3: Billing Notifications & PostHog

#### 3.1 Email Notifications

**Files:** New `platform/billing/notifications.go`

Wire billing events to the existing mailer/email service:

| Trigger             | When                                          | Template                                      |
| ------------------- | --------------------------------------------- | --------------------------------------------- |
| Credits at 80%      | After DeductCredits when used/total &gt;= 0.8 | Warning + usage breakdown + reset date        |
| Credits exhausted   | After DeductCredits when remaining &lt;= 0    | Block notice + upgrade CTA or reset countdown |
| Weekly reset        | Monday 00:00 UTC cron job                     | Last week's usage summary                     |
| Payment failed      | Webhook: invoice.payment_failed               | Grace period notice                           |
| Subscription change | Webhook: subscription.updated/deleted         | Confirmation with new limits                  |

#### 3.2 PostHog Conversion Events

**Files:** `platform/server/handlers_billing.go`, `platform/billing/middleware.go`

Fire server-side PostHog events:

- `billing.pricing_page_viewed` — when pricing page API is hit
- `billing.checkout_started` — in HandleCreateCheckout
- `billing.checkout_completed` — in checkout.session.completed webhook
- `billing.feature_gate_hit` — in PlanGuard when returning 403
- `billing.credits_exhausted` — in QuotaGuard when returning 429

**Files:** `platform/apps/web/src/lib/posthog.ts`

Client-side PostHog JS SDK for page views and UI interactions.

### Phase 4: Trial Support

#### 4.1 14-Day Pro Trial for New Signups

**Files:** `platform/service/auth.go`, `platform/billing/stripe.go`

When creating a new workspace via OIDC signup:

1. Create Stripe customer immediately
2. Create subscription with `trial_period_days: 14` via Stripe API
3. Set workspace plan to `pro` with status `trialing`
4. Listen for `customer.subscription.trial_will_end` webhook (3 days before)
5. Send trial-ending email notification

```go
// In CreateCheckoutSession, add trial support:
params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
    TrialPeriodDays: stripe.Int64(14),
}
```

### Phase 5: Stripe Setup & DevEx

#### 5.1 Stripe Product/Price Seed Script

**File:** New `scripts/stripe-seed.sh` or `scripts/stripe-setup.go`

Idempotent script using Stripe CLI to create:

- Products: `bowrain-pro`, `bowrain-team-seat`, `bowrain-credits`
- Prices with `bowrain_plan` metadata
- Meter: `bowrain_ai_tokens` with `sum` aggregation and `workspace_id`/`operation_type` dimensions

```bash
# stripe products create --name "Bowrain Pro" --metadata bowrain_plan=pro
# stripe prices create --product prod_xxx --currency usd --unit-amount 2500 --recurring-interval month
# stripe billing meters create --event-name ai_token_usage --aggregation-formula sum
```

#### 5.2 Keycloak Admin Realm

**File:** New `docker/keycloak/admin-realm.json`

Create the `bowrain-admin` realm config per AD-030 (registration disabled, dev admin account).

**Files:** `compose.yaml`, Keycloak Dockerfile

Mount and import the admin realm alongside the customer realm.

#### 5.3 Graceful Billing-Disabled Mode

**Files:** `platform/billing/middleware.go`, `platform/server/server.go`

When `BillingStore` is nil (no Stripe keys configured):

- `PlanGuard` → pass through (all features enabled)
- `QuotaGuard` → pass through (no credit checks)
- Billing routes return 503
- Self-hosted and dev deployments work without Stripe

```go
func PlanGuard(feature Feature) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            // When billing is not configured, allow all features
            planStr, ok := c.Get(contextKeyWorkspacePlan).(string)
            if !ok || planStr == "" {
                return next(c)
            }
            // ... existing logic
        }
    }
}
```

### Phase 6: Stripe Best Practices Alignment

Based on research of [Stripe's AI billing docs](https://docs.stripe.com/billing/token-billing), [usage-based billing guide](https://docs.stripe.com/billing/subscriptions/usage-based), [credit-based pricing](https://docs.stripe.com/billing/subscriptions/usage-based/use-cases/credits-based-pricing-model), and [Stripe AI toolkit](https://github.com/stripe/ai):

#### 6.1 Idempotency Keys for Meter Events

**File:** `platform/billing/stripe.go`

Add idempotency keys to `ReportMeterEvent` to prevent duplicate billing:

```go
// Use job_id or conversation_id + timestamp as idempotency key
params.Identifier = stripe.String(fmt.Sprintf("%s-%s-%d", workspaceID, refID, time.Now().UnixMilli()))
```

#### 6.2 Evaluate Stripe Token Billing (Private Preview)

Stripe's AI token billing (private preview) auto-tracks tokens per model and handles price syncing for OpenAI/Anthropic/Google. Worth evaluating when it reaches GA:

- **Self-reporting mode**: Bowrain already reports via Meters API; could switch to Stripe's token-specific endpoints for automatic model-price tracking
- **Stripe AI Gateway**: Not applicable (Bowrain manages its own provider routing)
- No action needed now — current Meters API approach is correct

#### 6.3 Usage Alerts via Stripe

Configure Stripe billing alerts for high-usage customers:

- 80% of included credits → trigger webhook → email notification
- 100% of credits → trigger webhook → enforcement

This supplements the server-side credit checks with Stripe-native alerting.

#### 6.4 Restricted API Keys

For any future agent/MCP integration (per Stripe AI toolkit patterns), use Restricted API Keys (rk\_\*) with minimal permissions rather than the main secret key.

---

## Phase Summary & Prioritization

| Phase       | Scope                                              | Priority          | Effort    |
| ----------- | -------------------------------------------------- | ----------------- | --------- |
| **Phase 1** | Core billing loop (credits, plans, Stripe sync)    | P0 — must have    | ~3-4 days |
| **Phase 2** | Limits enforcement (seats, projects, credit packs) | P0 — must have    | ~2 days   |
| **Phase 3** | Notifications & analytics (email, PostHog)         | P1 — should have  | ~2 days   |
| **Phase 4** | Trial support (14-day Pro trial)                   | P1 — should have  | ~1 day    |
| **Phase 5** | DevEx (seed scripts, Keycloak, graceful disable)   | P1 — should have  | ~1-2 days |
| **Phase 6** | Stripe best practices (idempotency, alerts)        | P2 — nice to have | ~1 day    |

**Total: ~10-13 days of focused implementation work.**

The critical insight is that the hard architectural work is done — the billing package, data model, server handlers, admin control plane, and customer-facing UI all exist. What's missing is the **wiring**: connecting the billing infrastructure to the actual feature gates, usage tracking, and Stripe lifecycle events.

---

## Open Decisions (from AD-030, still open)

1. **Trial period**: Recommend yes — 14-day Pro trial for new signups. Low friction, high conversion potential.
2. **Annual billing**: Recommend deferring — add when sales pipeline demands it. Stripe supports it trivially.
3. **Credit rollover**: Recommend no — keep it simple, weekly reset.
4. **Grandfathering**: Recommend 3-month Pro trial for existing workspaces at launch.
5. **Self-hosted**: Recommend billing-disabled mode (Phase 5.3) — framework is open-source, billing is platform-only.

## References

- [AD-030: Billing, Plans & Usage Quotas](030-billing-plans.md)
- [Stripe Usage-Based Billing](https://docs.stripe.com/billing/subscriptions/usage-based)
- [Stripe Token Billing (Private Preview)](https://docs.stripe.com/billing/token-billing)
- [Stripe Credit-Based Pricing](https://docs.stripe.com/billing/subscriptions/usage-based/use-cases/credits-based-pricing-model)
- [Stripe AI Toolkit](https://github.com/stripe/ai)
- [Stripe Meters API](https://docs.stripe.com/api/billing/meter)
- [Stripe Pricing Strategies for AI](https://stripe.com/resources/more/pricing-strategies-for-ai-companies)
