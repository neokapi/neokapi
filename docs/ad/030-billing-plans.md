# AD-030: Billing, Plans & Usage Quotas (Stripe + PostHog)

## Context

Bowrain needs a billing system to support paid plans with AI usage quotas. The platform already has:

- **Two usage tracking systems** (passive, no enforcement):
  - `jobs/quota.go`: AI token usage per workspace (monthly, PostgreSQL, 10M default)
  - `core/agent/agent.go`: @bravo agent usage (tokens + container time, time-range queries)
- **Workspace as the billing unit**: all usage is per-workspace, with roles (owner/admin/member/viewer)
- **No existing Stripe, PostHog, or plan/tier infrastructure**

## Decision

### Plan Structure: Hybrid Subscription + Usage Credits

Inspired by Claude's multiplier model and Stripe's latest AI billing features, Bowrain uses a **tiered subscription with weekly usage credits** model.

#### Plans

| Plan | Price | AI Credits/week | @bravo | Seats | Billing |
|------|-------|-----------------|--------|-------|---------|
| **Free** | $0 | 50K tokens | 5 messages/day | 1 | — |
| **Pro** | $25/mo | 500K tokens (10x) | Unlimited messages | 3 | Monthly |
| **Team** | $20/seat/mo | 2M tokens (40x) | Unlimited + code exec | Unlimited | Monthly |
| **Enterprise** | Custom | Custom | Custom | Unlimited | Annual |

**Why weekly credits instead of monthly?**
- **Prevents binge/drought**: monthly quotas let users burn everything in week 1, then churn
- **Claude precedent**: Claude uses 5-hour windows; weekly is the SaaS equivalent — short enough to feel generous, long enough to be manageable
- **Smoother cost distribution**: aligns better with AI provider billing cycles
- **Psychological benefit**: "resets Monday" feels like a fresh start, not a punishment

#### Usage Credit System

One "credit" = 1 AI token (input or output). Different operations cost different amounts:

| Operation | Credit Cost |
|-----------|-------------|
| AI translation (per token) | 1 credit |
| AI quality check (per token) | 1 credit |
| @bravo message (per token) | 1 credit |
| @bravo container time | 10 credits/sec |

**Overage handling**: When credits are exhausted:
1. Free: hard block, wait for weekly reset (Monday 00:00 UTC)
2. Pro: soft block with option to buy a one-time credit pack ($5 = 200K tokens)
3. Team: configurable — soft block or auto-purchase credit packs
4. Enterprise: no limits (custom agreement)

### Architecture

```
ctrl.bowrain.cloud               app.bowrain.cloud
(admin control plane)            (customer self-service)
       │                                │
  ┌────▼────┐                     ┌─────▼─────┐
  │ctrl-web │                     │ bowrain-   │
  │ (nginx) │                     │   web      │
  └────┬────┘                     └─────┬──────┘
       │                                │
       │ OIDC: bowrain-admin realm      │ OIDC: bowrain realm
       │ /api/admin/*                   │ /api/v1/*
       └──────────┬─────────────────────┘
            ┌─────▼──────┐     ┌─────────────────┐
            │  bowrain-   │────▶│   PostgreSQL    │
            │  server     │     │  subscriptions  │
            │  (Echo v4)  │     │  credit_ledger  │
            │             │     └─────────────────┘
            │ Middleware:  │            │
            │  PlanGuard  │     ┌──────▼──────────┐
            │  QuotaGuard │     │   Keycloak      │
            │  AdminGuard │     │  ┌────────────┐ │
            └──────┬──────┘     │  │ bowrain    │ │
                   │            │  │ (customers)│ │
            ┌──────▼───┐        │  ├────────────┤ │
            │  Stripe  │        │  │ bowrain-   │ │
            │ Billing  │        │  │ admin      │ │
            └──────────┘        │  │ (operators)│ │
                   │            │  └────────────┘ │
            ┌──────▼───┐        └─────────────────┘
            │ PostHog  │
            │ Analytics│
            └──────────┘
```

### Stripe Integration

#### Products & Prices

```
Stripe Products:
├── bowrain-pro          → $25/mo flat (Subscription)
├── bowrain-team-seat    → $20/mo per seat (Subscription, quantity-based)
├── bowrain-credits      → $5 per 200K pack (one-time, metered via Stripe Meters)
└── bowrain-enterprise   → custom (manual invoicing)
```

#### Stripe Meters

Use Stripe's Meters API (v2, required since API version 2025-03-31.basil) to track real-time AI consumption:

```
Meter: "bowrain_ai_tokens"
  - event_name: "ai_token_usage"
  - aggregation: sum
  - dimensions: [workspace_id, operation_type]
```

This enables Stripe to show usage dashboards and handle overage billing for credit packs automatically.

#### Webhook Events

| Event | Action |
|-------|--------|
| `checkout.session.completed` | Activate subscription, set plan on workspace |
| `customer.subscription.updated` | Update plan, adjust quotas |
| `customer.subscription.deleted` | Downgrade to Free |
| `invoice.paid` | Confirm payment, grant weekly credits |
| `invoice.payment_failed` | Grace period (3 days), then downgrade |
| `customer.subscription.trial_will_end` | Send reminder email |

### Feature Gating

Features are gated by plan using a **code-first approach with optional overrides**.

#### Plan Feature Matrix (source of truth, in `billing/plans.go`)

```go
type Feature string

const (
    FeatureBravoCodeExec    Feature = "bravo-code-exec"
    FeatureConnectorsGit    Feature = "connectors-git"
    FeatureConnectorsCustom Feature = "connectors-custom"
    FeatureAPIAccess        Feature = "api-access"
    FeatureSSOSAML          Feature = "sso-saml"
    FeatureCustomMT         Feature = "custom-mt-providers"
)

var PlanFeatures = map[Plan]map[Feature]bool{
    PlanFree: {
        FeatureBravoCodeExec:    false,
        FeatureConnectorsGit:    false,
        FeatureConnectorsCustom: false,
        FeatureAPIAccess:        false,
        FeatureSSOSAML:          false,
        FeatureCustomMT:         false,
    },
    PlanPro: {
        FeatureBravoCodeExec:    false,
        FeatureConnectorsGit:    true,
        FeatureConnectorsCustom: false,
        FeatureAPIAccess:        true,
        FeatureSSOSAML:          false,
        FeatureCustomMT:         true,
    },
    PlanTeam: {
        FeatureBravoCodeExec:    true,
        FeatureConnectorsGit:    true,
        FeatureConnectorsCustom: true,
        FeatureAPIAccess:        true,
        FeatureSSOSAML:          false,
        FeatureCustomMT:         true,
    },
    PlanEnterprise: {
        FeatureBravoCodeExec:    true,
        FeatureConnectorsGit:    true,
        FeatureConnectorsCustom: true,
        FeatureAPIAccess:        true,
        FeatureSSOSAML:          true,
        FeatureCustomMT:         true,
    },
}

// PlanLimits defines numeric limits per plan.
var PlanLimits = map[Plan]map[string]int{
    PlanFree:       {"max-projects": 1,  "max-seats": 1},
    PlanPro:        {"max-projects": 10, "max-seats": 3},
    PlanTeam:       {"max-projects": -1, "max-seats": -1},  // -1 = unlimited
    PlanEnterprise: {"max-projects": -1, "max-seats": -1},
}
```

This is the **default path**: zero latency, no external calls, deployed with the binary.

#### Feature Matrix Summary

| Feature | Free | Pro | Team | Enterprise |
|---------|------|-----|------|------------|
| @bravo chat | yes | yes | yes | yes |
| @bravo code exec | - | - | yes | yes |
| Git connectors | - | yes | yes | yes |
| Custom connectors | - | - | yes | yes |
| API access | - | yes | yes | yes |
| SSO/SAML | - | - | - | yes |
| Custom MT providers | - | yes | yes | yes |
| Max projects | 1 | 10 | unlimited | unlimited |
| Max seats | 1 | 3 | unlimited | unlimited |

#### PlanGuard Middleware

`PlanGuard` enforces the matrix per route. It reads the workspace's `plan` field (already loaded by `WorkspaceAccessMiddleware`) — no database query needed:

```go
func PlanGuard(feature Feature) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            ws := auth.WorkspaceFromContext(c)
            if HasFeature(Plan(ws.Plan), feature) {
                return next(c)
            }
            return c.JSON(http.StatusForbidden, map[string]any{
                "error":        "upgrade_required",
                "feature":      feature,
                "minimum_plan": MinimumPlanFor(feature),
            })
        }
    }
}
```

Applied per route group in `server.go`:

```go
connectors := ws.Group("/connectors")
connectors.Use(billing.PlanGuard(billing.FeatureConnectorsGit))

apiTokens := ws.Group("/api-tokens")
apiTokens.Use(billing.PlanGuard(billing.FeatureAPIAccess))

agentExec := ws.Group("/agent/exec")
agentExec.Use(billing.PlanGuard(billing.FeatureBravoCodeExec))
```

The `403` response body (`upgrade_required` + `minimum_plan`) gives the frontend everything it needs to show contextual upgrade prompts.

#### Per-Workspace Feature Overrides

Feature overrides are stored in the database and managed via the control plane (`ctrl.bowrain.cloud`). This replaces the need for PostHog-based feature gating.

Use cases:
- **Beta programs**: give a free workspace access to Git connectors during a beta
- **Support compensation**: temporarily enable a feature after an outage
- **Partner deals**: custom feature sets for strategic partners
- **Gradual rollout**: enable a new paid feature for specific workspaces before general availability

Overrides are loaded once per request (cached on the workspace context by `WorkspaceAccessMiddleware`) and checked by `PlanGuard` before the plan matrix:

```go
func HasFeature(plan Plan, feature Feature, overrides map[Feature]bool) bool {
    // 1. Check per-workspace override (from DB, cached on context)
    if enabled, ok := overrides[feature]; ok {
        return enabled
    }
    // 2. Fall back to plan matrix
    if features, ok := PlanFeatures[plan]; ok {
        return features[feature]
    }
    return false
}
```

Overrides can have an optional `expires_at` — expired overrides are ignored by `HasFeature` and cleaned up by a periodic job.

### PostHog Integration

PostHog's role is **analytics and experiments**, not billing or gating.

#### Product Analytics
- **Usage patterns**: which AI features are used, by whom, how often
- **Conversion funnel**: free → pro upgrade triggers (viewed pricing → started checkout → completed)
- **Churn prediction**: declining usage patterns before cancellation
- **Revenue analytics**: Stripe data source integration for MRR/churn dashboards

#### Experiments
- "Does offering 2x credits for the first month increase Pro conversion?"
- "Does removing the project limit on Free reduce churn?"
- "Does showing upgrade prompts in @bravo increase Team plan adoption?"

### Data Model

#### New Tables

```sql
-- Workspace subscription state (source of truth is Stripe, this is a cache)
CREATE TABLE subscriptions (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id        TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    stripe_customer_id  TEXT NOT NULL,
    stripe_subscription_id TEXT,
    plan                TEXT NOT NULL DEFAULT 'free',  -- free, pro, team, enterprise
    status              TEXT NOT NULL DEFAULT 'active', -- active, past_due, canceled, trialing
    seat_count          INTEGER NOT NULL DEFAULT 1,
    current_period_start TIMESTAMPTZ,
    current_period_end   TIMESTAMPTZ,
    cancel_at            TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id),
    UNIQUE(stripe_customer_id)
);

-- Weekly credit allocation and tracking
CREATE TABLE credit_allocations (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id    TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    credits_total   BIGINT NOT NULL,         -- total credits for this week
    credits_used    BIGINT NOT NULL DEFAULT 0,
    week_start      TIMESTAMPTZ NOT NULL,    -- Monday 00:00 UTC
    week_end        TIMESTAMPTZ NOT NULL,    -- Next Monday 00:00 UTC
    source          TEXT NOT NULL DEFAULT 'plan', -- 'plan' or 'purchased'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, week_start, source)
);

-- Immutable ledger of credit transactions
CREATE TABLE credit_ledger (
    id              BIGSERIAL PRIMARY KEY,
    workspace_id    TEXT NOT NULL,
    allocation_id   TEXT REFERENCES credit_allocations(id),
    amount          BIGINT NOT NULL,          -- negative = debit, positive = credit
    balance_after   BIGINT NOT NULL,          -- running balance
    operation       TEXT NOT NULL,             -- 'ai_translation', 'bravo_message', 'bravo_container', 'purchase', 'grant', 'expire'
    reference_id    TEXT,                      -- job_id, conversation_id, etc.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credit_ledger_workspace ON credit_ledger(workspace_id, created_at);
```

#### Changes to Existing Tables

```sql
-- Add to workspaces table (cached for fast access)
ALTER TABLE workspaces ADD COLUMN plan TEXT NOT NULL DEFAULT 'free';
ALTER TABLE workspaces ADD COLUMN stripe_customer_id TEXT;
```

### Go Implementation

#### New Package: `platform/billing/`

```
platform/billing/
├── plans.go          # Plan definitions, feature matrix, credit amounts
├── store.go          # BillingStore interface
├── postgres.go       # PostgreSQL implementation
├── stripe.go         # Stripe client wrapper (products, subscriptions, meters)
├── webhooks.go       # Stripe webhook handler
├── credits.go        # Credit allocation, deduction, balance checking
├── middleware.go     # PlanGuard and QuotaGuard Echo middleware
└── posthog.go        # PostHog client wrapper (events, feature flags)
```

#### Key Interfaces

```go
// BillingStore persists subscription and credit data.
type BillingStore interface {
    // Subscriptions
    GetSubscription(ctx context.Context, workspaceID string) (*Subscription, error)
    UpsertSubscription(ctx context.Context, sub *Subscription) error

    // Credits
    GetCurrentAllocation(ctx context.Context, workspaceID string) (*CreditAllocation, error)
    DeductCredits(ctx context.Context, workspaceID string, amount int64, op string, refID string) error
    CheckCredits(ctx context.Context, workspaceID string) (remaining int64, err error)
    GrantCredits(ctx context.Context, workspaceID string, amount int64, source string) error

    // Ledger
    GetLedger(ctx context.Context, workspaceID string, from, to time.Time) ([]LedgerEntry, error)
}

// PlanGuard middleware rejects requests when the workspace plan
// doesn't include the required feature.
func PlanGuard(feature string) echo.MiddlewareFunc

// QuotaGuard middleware rejects requests when weekly credits are exhausted.
// Returns 429 with Retry-After header set to next Monday 00:00 UTC.
func QuotaGuard() echo.MiddlewareFunc
```

#### Integration with Existing Quota System

The existing `jobs/quota.go` (AI token usage) and `agent/agent.go` (bravo usage) continue to record granular usage. The new `billing/credits.go` layer sits above them:

```
AI Tool / @bravo
    │
    ├──▶ jobs.QuotaStore.RecordUsage()      ← detailed tracking (unchanged)
    ├──▶ agent.AgentStore.RecordUsage()      ← detailed tracking (unchanged)
    └──▶ billing.BillingStore.DeductCredits() ← NEW: credit deduction
              │
              ├──▶ PostgreSQL credit_ledger
              └──▶ Stripe Meter Event (async, for billing)
```

### SDK Dependencies

```go
// platform/go.mod additions
require (
    github.com/stripe/stripe-go/v82  // Stripe Go SDK (v80+ for Meters API)
    github.com/posthog/posthog-go    // PostHog Go SDK
)
```

### Server Configuration

```go
// New fields in server/config.go
type BillingConfig struct {
    StripeSecretKey      string `env:"STRIPE_SECRET_KEY"`
    StripeWebhookSecret  string `env:"STRIPE_WEBHOOK_SECRET"`
    StripeProPriceID     string `env:"STRIPE_PRO_PRICE_ID"`
    StripeTeamPriceID    string `env:"STRIPE_TEAM_PRICE_ID"`
    StripeCreditPriceID  string `env:"STRIPE_CREDIT_PRICE_ID"`
    PostHogAPIKey        string `env:"POSTHOG_API_KEY"`
    PostHogHost          string `env:"POSTHOG_HOST" default:"https://us.i.posthog.com"`
}
```

### API Endpoints

```
# Customer self-service billing (workspace-scoped, JWT auth)
GET    /api/v1/workspaces/:ws/billing              # Current plan, usage, credits
GET    /api/v1/workspaces/:ws/billing/usage         # Credit usage breakdown
POST   /api/v1/workspaces/:ws/billing/checkout      # Create Stripe Checkout session
POST   /api/v1/workspaces/:ws/billing/portal        # Create Stripe Customer Portal session
GET    /api/v1/workspaces/:ws/billing/invoices       # Invoice history

# Admin API (admin realm JWT auth, used by control plane)
GET    /api/admin/workspaces                         # List all workspaces with plan/usage summary
GET    /api/admin/workspaces/:id                     # Full workspace detail (subscription, credits, members, usage)
PUT    /api/admin/workspaces/:id/plan                # Override plan (upgrade, downgrade, set custom limits)
POST   /api/admin/workspaces/:id/credits             # Grant bonus credits (with reason/note)
GET    /api/admin/workspaces/:id/feature-overrides   # List overrides for workspace
PUT    /api/admin/workspaces/:id/feature-overrides   # Set per-workspace feature overrides
GET    /api/admin/workspaces/:id/notes               # List internal notes
POST   /api/admin/workspaces/:id/notes               # Add internal note
GET    /api/admin/users                              # List/search users
GET    /api/admin/users/:id                          # User detail (workspaces, activity, login history)
GET    /api/admin/metrics                            # Platform-wide metrics (MRR, active workspaces, churn, usage)
GET    /api/admin/events                             # Recent billing events (subscriptions, payments, downgrades)
GET    /api/admin/upsells                            # Ranked upsell opportunities
GET    /api/admin/overrides                          # All feature overrides across all workspaces

# Webhooks (no auth, signature-verified)
POST   /api/webhooks/stripe                          # Stripe webhook endpoint
```

### Control Plane Admin App (`ctrl.bowrain.cloud`)

An internal admin dashboard for customer service and platform operations. Deployed as a separate container in the cluster, talking to the same bowrain-server via `/api/admin/*` routes.

#### Auth: Separate Keycloak Realm

The control plane uses a dedicated `bowrain-admin` Keycloak realm, completely separate from the customer `bowrain` realm. This provides:

- **Hard identity separation**: admin accounts don't exist in the customer realm, no accidental role escalation
- **Independent auth policies**: require MFA, restrict registration (invite-only), shorter session timeouts
- **No user model changes**: no `super_admin` column on the users table — admin identity is determined by which realm issued the JWT
- **Simple networking**: `ctrl.bowrain.cloud` can be VPN-only or IP-allowlisted at the ingress level

```
Keycloak
├── bowrain realm          → customers (app.bowrain.cloud)
│   ├── client: bowrain
│   ├── registration: open (email + passkey)
│   └── identity providers: Google, GitHub
│
└── bowrain-admin realm    → operators (ctrl.bowrain.cloud)
    ├── client: bowrain-admin
    ├── registration: disabled (invite-only)
    ├── MFA: required
    └── identity providers: none (email/password only)
```

**Server-side validation**: bowrain-server loads two OIDC verifiers — one per realm. The `AdminGuard` middleware validates the JWT was issued by the `bowrain-admin` realm:

```go
// In server/config.go
type AdminConfig struct {
    AdminOIDCIssuerURL    string `env:"BOWRAIN_ADMIN_OIDC_ISSUER_URL"`   // e.g. https://auth.bowrain.cloud/realms/bowrain-admin
    AdminOIDCClientID     string `env:"BOWRAIN_ADMIN_OIDC_CLIENT_ID"`
    AdminOIDCClientSecret string `env:"BOWRAIN_ADMIN_OIDC_CLIENT_SECRET"`
}

// AdminGuard middleware — verifies JWT is from the admin realm.
func AdminGuard(adminVerifier *oidc.IDTokenVerifier) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            token := extractBearerToken(c)
            idToken, err := adminVerifier.Verify(c.Request().Context(), token)
            if err != nil {
                return echo.ErrUnauthorized
            }
            var claims struct {
                Email string `json:"email"`
                Name  string `json:"name"`
            }
            idToken.Claims(&claims)
            c.Set("admin_email", claims.Email)
            c.Set("admin_name", claims.Name)
            return next(c)
        }
    }
}
```

**Local dev**: the `bowrain-admin` realm is added to the existing `docker/keycloak/realm.json` import and auto-seeded with a dev admin account (`admin@bowrain.cloud` / `admin`).

#### Keycloak Realm Config (`docker/keycloak/admin-realm.json`)

```json
{
  "realm": "bowrain-admin",
  "enabled": true,
  "loginTheme": "bowrain",
  "registrationAllowed": false,
  "resetPasswordAllowed": true,
  "verifyEmail": true,
  "loginWithEmailAllowed": true,
  "sslRequired": "none",
  "clients": [
    {
      "clientId": "bowrain-admin",
      "name": "Bowrain Control Plane",
      "enabled": true,
      "publicClient": false,
      "secret": "bowrain-admin-secret",
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": false,
      "redirectUris": [
        "http://localhost:3100/*",
        "https://ctrl.bowrain.mymac/*",
        "https://ctrl.bowrain.cloud/*"
      ],
      "webOrigins": [
        "http://localhost:3100",
        "https://ctrl.bowrain.mymac",
        "https://ctrl.bowrain.cloud"
      ]
    }
  ],
  "users": [
    {
      "username": "admin@bowrain.cloud",
      "email": "admin@bowrain.cloud",
      "enabled": true,
      "emailVerified": true,
      "firstName": "Admin",
      "lastName": "Bowrain",
      "credentials": [{ "type": "password", "value": "admin", "temporary": false }]
    }
  ]
}
```

#### Workspaces vs Subscriptions

A **subscription** is owned by a **workspace**, not a user. This is the billing unit:

```
User ──┬── Workspace A (personal, Free plan)
       │       └── Subscription: free, no Stripe customer
       │
       ├── Workspace B (team, Pro plan) ← owner
       │       ├── Subscription: pro, stripe_customer_id=cus_xxx
       │       ├── Member: alice (admin)
       │       └── Member: bob (member)
       │
       └── Workspace C (team, Team plan) ← member
               ├── Subscription: team, 8 seats, stripe_customer_id=cus_yyy
               └── ...
```

**Key implications:**
- One user can be in multiple workspaces on different plans
- The workspace owner manages billing (only owners can access Settings > Billing)
- Seat count is per workspace, enforced when adding members
- Credits are per workspace, shared by all members
- A personal workspace always has exactly 1 seat

The control plane shows both views:
- **Workspace-centric** (primary): subscription, credits, usage, members — this is the billing/support view
- **User-centric**: which workspaces a user belongs to, useful for support escalations ("I can't access my workspace")

#### Control Plane Pages

The ctrl app reuses the same `@neokapi/ui` shared component library and the same `AppShell` layout as the main web app. Instead of the workspace sidebar/rail, it has an admin navigation sidebar.

**Shared from `packages/ui/`:**
- All primitives (`ui/button`, `ui/card`, `ui/input`, `ui/badge`, `ui/tabs`, etc.)
- `AppShell` layout (top bar + sidebar + content area)
- `FilterBar`, `ConfirmDialog`, `skeletons`
- Chart components (`ui/chart`)
- Any new billing components (`PlanCard`, `UsageBar`, `CreditCounter`) — built in `packages/ui/` so both apps use them

**Ctrl-specific (not shared):**
- Admin sidebar navigation (Workspaces, Users, Events, Overrides, Upsells)
- Admin API client (`/api/admin/*`)
- OIDC auth against the `bowrain-admin` realm

| Page | Purpose |
|------|---------|
| **Dashboard** | Platform KPIs: MRR, active workspaces, new signups (7d/30d), credit utilization rate, churn rate, top workspaces by usage |
| **Workspaces** | Searchable/filterable table. Columns: name, owner, plan, status, credit usage %, members, created. Click → detail |
| **Workspace Detail** | Full customer view: subscription info (plan, status, period, Stripe link), credit balance + usage bar + ledger, member list, recent activity, usage charts. Actions: change plan, grant credits, feature overrides, internal notes |
| **Users** | Search by email/name. View: workspaces they belong to, last login, account age |
| **Billing Events** | Live feed: new subscriptions, upgrades, downgrades, payment failures, credit purchases. Filterable by type + date |
| **Feature Overrides** | All per-workspace overrides. Add/remove with reason + optional expiry |
| **Upsells** | Workspaces ripe for upgrade (see below) |

#### Upsell View

The upsell page surfaces workspaces that are likely candidates for an upgrade, helping the team prioritize outreach:

| Signal | Description | Suggested Action |
|--------|-------------|-----------------|
| **Credit exhaustion** | Free workspaces that hit 100% credit usage 2+ weeks in a row | Reach out with Pro trial offer |
| **Seat pressure** | Pro workspaces at seat limit (3/3) with pending invites or removed members | Suggest Team plan |
| **Feature gate hits** | Workspaces receiving repeated `403 upgrade_required` responses | Offer trial of the gated feature |
| **High usage, low plan** | Workspaces consistently using >80% of weekly credits | Proactive upgrade conversation |
| **Trial expiring** | Workspaces on trial ending within 3 days | Follow up on conversion |
| **Dormant paid** | Paid workspaces with less than 10% credit usage for 4+ weeks | Check in to prevent churn |

The upsell view is powered by a `/api/admin/upsells` endpoint that queries usage patterns and returns ranked opportunities:

```go
type UpsellOpportunity struct {
    WorkspaceID   string    `json:"workspace_id"`
    WorkspaceName string    `json:"workspace_name"`
    CurrentPlan   Plan      `json:"current_plan"`
    Signal        string    `json:"signal"`        // "credit_exhaustion", "seat_pressure", etc.
    Score         int       `json:"score"`         // priority score (higher = more urgent)
    Detail        string    `json:"detail"`        // human-readable detail
    SuggestedPlan Plan      `json:"suggested_plan"`
    DetectedAt    time.Time `json:"detected_at"`
}
```

Each row links to the workspace detail page where the admin can take action (grant trial credits, offer a plan change, add a note).

#### Workspace Detail — Customer Service Actions

| Action | API | Effect |
|--------|-----|--------|
| **Change plan** | `PUT /api/admin/workspaces/:id/plan` | Override subscription plan. Syncs to Stripe |
| **Grant credits** | `POST /api/admin/workspaces/:id/credits` | Bonus credits with reason note. Recorded in ledger |
| **Feature override** | `PUT /api/admin/workspaces/:id/feature-overrides` | Enable/disable features, overriding the plan matrix |
| **View as customer** | Link to `app.bowrain.cloud/ws/{slug}` | Opens workspace in the main app |
| **Open in Stripe** | External link | Deep link to Stripe customer/subscription |
| **Add internal note** | `POST /api/admin/workspaces/:id/notes` | Internal-only, visible to other admins |

#### Feature Overrides Table

```sql
CREATE TABLE feature_overrides (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    feature      TEXT NOT NULL,
    enabled      BOOLEAN NOT NULL,
    reason       TEXT,             -- "beta program", "support compensation", etc.
    created_by   TEXT NOT NULL,    -- admin email (from admin realm JWT)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ,     -- optional auto-expiry
    UNIQUE(workspace_id, feature)
);
```

Feature overrides are first-class, stored in the DB, visible in the control plane, and checked by `PlanGuard`:

```go
func HasFeature(plan Plan, feature Feature, overrides map[Feature]bool) bool {
    // 1. Check per-workspace override (from DB, cached on request context)
    if enabled, ok := overrides[feature]; ok {
        return enabled
    }
    // 2. Fall back to plan matrix
    if features, ok := PlanFeatures[plan]; ok {
        return features[feature]
    }
    return false
}
```

Overrides with an `expires_at` are ignored once expired and cleaned up by a periodic job.

#### Internal Notes Table

```sql
CREATE TABLE workspace_notes (
    id           BIGSERIAL PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    author_email TEXT NOT NULL,    -- admin email (from admin realm JWT)
    content      TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Customer Self-Service (Primary — `app.bowrain.cloud`)

Customer self-service within the main Bowrain web app remains the primary way to manage subscriptions. The control plane is for support and operations only.

#### Pricing Page (public, no auth)
- Plan comparison table with feature matrix
- CTA buttons → Stripe Checkout
- FAQ section

#### Workspace Settings > Billing (owner-only)
- Current plan + status badge
- Credit usage bar (weekly, with reset countdown timer)
- Usage breakdown by operation type (AI translation, @bravo, etc.)
- Upgrade/downgrade buttons → Stripe Checkout / Customer Portal
- Invoice history table
- Seat management (Team plan): current seats, add/remove, seat limit indicator

#### Upgrade Prompts (contextual)
When a user hits a plan limit, the UI shows a contextual upgrade prompt instead of a generic error:
- **Feature gate**: "Git connectors require a Pro plan. [Upgrade →]"
- **Credit exhaustion**: "Weekly credits used. Resets Monday. [Buy credits →] or [Upgrade →]"
- **Seat limit**: "Your plan includes 3 seats. [Upgrade to Team →]"
- **Project limit**: "Free plan allows 1 project. [Upgrade to Pro →]"

These prompts use the `UpgradePrompt` component from `packages/ui/` (shared with the ctrl app's workspace detail).

#### PostHog JS SDK
- Loaded on both `app.bowrain.cloud` and `ctrl.bowrain.cloud`
- Identifies user by user ID + workspace context
- Tracks conversion events: viewed pricing, started checkout, completed checkout, hit feature gate

## Implementation

All files to create or modify, in dependency order:

### 1. Dependencies

Add to `platform/go.mod`:
```
github.com/stripe/stripe-go/v82
github.com/posthog/posthog-go
```

### 2. New Package: `platform/billing/`

| File | Purpose |
|------|---------|
| `plans.go` | Plan constants, feature matrix, weekly credit amounts per plan, `HasFeature()` |
| `types.go` | `Subscription`, `CreditAllocation`, `LedgerEntry`, `FeatureOverride`, `WorkspaceNote`, `UpsellOpportunity` structs |
| `store.go` | `BillingStore` interface (subscriptions, credits, ledger, feature overrides, notes, upsell queries) |
| `postgres.go` | PostgreSQL implementation with migrations (subscriptions, credit_allocations, credit_ledger, feature_overrides, workspace_notes tables) |
| `credits.go` | Credit allocation (weekly grant), deduction, balance checking, weekly reset logic |
| `stripe.go` | Stripe client: create customers, checkout sessions, portal sessions, meter events |
| `webhooks.go` | Stripe webhook handler with signature verification, subscription lifecycle |
| `middleware.go` | `PlanGuard(feature)`, `QuotaGuard()`, and `AdminGuard(verifier)` Echo middleware |
| `upsells.go` | Upsell signal detection queries (credit exhaustion, seat pressure, feature gate hits, dormant paid) |
| `posthog.go` | PostHog client: event capture, user identification |

### 3. Existing Files to Modify

| File | Change |
|------|--------|
| `platform/core/auth/types.go` | Add `Plan` and `StripeCustomerID` fields to `Workspace` |
| `platform/auth/postgres.go` | Add migration for `plan`, `stripe_customer_id` columns on workspaces |
| `platform/auth/sqlite.go` | Same migration for SQLite |
| `platform/server/config.go` | Add `BillingConfig` and `AdminConfig` (admin OIDC) fields to `ServerConfig` |
| `platform/server/server.go` | Add `BillingStore` to Server struct, register billing routes + admin routes + webhook endpoint, wire `PlanGuard`/`QuotaGuard` on protected routes, wire `AdminGuard` on `/api/admin/*` routes, initialize admin OIDC verifier |
| `platform/server/handlers_billing.go` | New file: customer self-service billing handlers (get plan, usage, checkout, portal, invoices) |
| `platform/server/handlers_admin.go` | New file: admin API handlers (list workspaces, user search, plan overrides, grant credits, feature overrides, metrics, notes, upsells) |
| `platform/cmd/bowrain-server/main.go` | Initialize `BillingStore`, Stripe client, PostHog client, admin OIDC verifier from config |
| `platform/jobs/quota.go` | After `RecordUsage`, also call `BillingStore.DeductCredits` |
| `platform/core/agent/agent.go` | After `RecordUsage`, also call `BillingStore.DeductCredits` |

### 4. Keycloak Admin Realm

| File | Change |
|------|--------|
| `docker/keycloak/admin-realm.json` | New file: `bowrain-admin` realm config (registration disabled, MFA required, dev admin account) |
| `docker/keycloak/Dockerfile.dev` | Import both `realm.json` and `admin-realm.json` |
| `compose.yaml` | Mount `admin-realm.json` alongside `realm.json` |
| `compose.override.yaml` | Add `ctrl.bowrain.mymac` route to Traefik for local dev |

### 5. Shared UI Components (`packages/ui/`)

New billing components built in the shared library so both `apps/web/` and `apps/ctrl/` use the same implementation:

| Component | Used By | Purpose |
|-----------|---------|---------|
| `PlanCard` | web (pricing page), ctrl (workspace detail) | Plan tier card with features, price, CTA |
| `PlanComparisonTable` | web (pricing page), ctrl (upsell view) | Full feature matrix comparison across plans |
| `UsageBar` | web (billing settings, @bravo), ctrl (workspace detail) | Credit usage progress bar with reset countdown |
| `CreditCounter` | web (@bravo chat), ctrl (workspace detail) | Compact remaining credits display |
| `UpgradePrompt` | web (feature gates), ctrl (workspace detail) | Contextual upgrade suggestion with plan + feature info |
| `CreditLedger` | web (billing settings), ctrl (workspace detail) | Ledger table showing credit transactions |
| `SubscriptionBadge` | web (settings), ctrl (workspace list + detail) | Plan name + status badge (active, trial, past_due) |

### 6. Customer Self-Service Frontend (`apps/web/`)

| File | Purpose |
|------|--------|
| `apps/web/src/routes/pricing.tsx` | Public pricing page using `PlanComparisonTable` + Stripe Checkout CTAs |
| `apps/web/src/routes/workspace/settings-billing.tsx` | Billing settings: `SubscriptionBadge`, `UsageBar`, usage breakdown, `CreditLedger`, invoice history, seat management |
| `apps/web/src/lib/api.ts` | Add billing API methods (getSubscription, getUsage, createCheckout, createPortal) |
| `apps/web/src/lib/posthog.ts` | PostHog JS SDK init, user identification, conversion event tracking |
| `apps/web/src/components/bravo/` | Add `CreditCounter` and exhaustion `UpgradePrompt` to @bravo chat UI |

### 7. Control Plane Admin App (`apps/ctrl/`)

New Vite + React + TanStack Router app. Same stack and same `@neokapi/ui` dependency as `apps/web/`. Reuses the `AppShell` layout with an admin-specific sidebar. Deployed at `ctrl.bowrain.cloud`.

| File | Purpose |
|------|--------|
| `apps/ctrl/package.json` | Dependencies: same core as `apps/web/` (react, tanstack/router, tanstack/query, tailwindcss, `@neokapi/ui`) |
| `apps/ctrl/src/routes/root-layout.tsx` | `AppShell` with admin sidebar (Dashboard, Workspaces, Users, Events, Overrides, Upsells) |
| `apps/ctrl/src/routes/dashboard.tsx` | Platform KPIs: MRR, active workspaces, signups, credit utilization, churn. Uses `ui/chart` |
| `apps/ctrl/src/routes/workspaces.tsx` | `FilterBar` + table of all workspaces. Columns: name, owner, `SubscriptionBadge`, `UsageBar` (compact), members, created |
| `apps/ctrl/src/routes/workspace-detail.tsx` | Full customer view. Tabs: Overview (`SubscriptionBadge`, `UsageBar`, usage chart), Credits (`CreditLedger`, grant form), Members, Activity, Notes. Actions sidebar: change plan (`PlanCard` selector), feature overrides, Stripe link, "view as customer" link |
| `apps/ctrl/src/routes/users.tsx` | User search by email/name. Shows workspace memberships, last login |
| `apps/ctrl/src/routes/events.tsx` | Billing event feed with `FilterBar`. Uses `ui/badge` for event types |
| `apps/ctrl/src/routes/overrides.tsx` | All feature overrides across workspaces. Add/remove with reason + expiry |
| `apps/ctrl/src/routes/upsells.tsx` | Ranked upsell opportunities table. Columns: workspace, signal, current plan, suggested plan, score. Click → workspace detail |
| `apps/ctrl/src/lib/api.ts` | Admin API client (wraps `/api/admin/*` endpoints) |
| `apps/ctrl/src/lib/auth.ts` | OIDC auth against `bowrain-admin` realm |
| `docker/bowrain-ctrl/Dockerfile` | nginx container serving the ctrl SPA (same pattern as `docker/bowrain-web/`) |
| `docker/bowrain-ctrl/nginx.conf` | SPA fallback, proxies `/api/admin/*` to bowrain-server |

### 8. Email Notifications

| Trigger | Template |
|---------|----------|
| Credits at 80% used | Warning with usage breakdown and reset date |
| Credits exhausted | Blocked notice with upgrade CTA (Pro/Team) or reset countdown (Free) |
| Weekly credit reset | Summary of last week's usage |
| Payment failed | Grace period notice (3 days), then downgrade warning |
| Subscription change | Confirmation of upgrade/downgrade with new limits |

## Open Questions

1. **Trial period**: 14-day Pro trial for new signups? Stripe supports this natively.
2. **Annual billing**: 20% discount for annual? Easy to add as separate Stripe prices.
3. **Credit rollover**: Do unused credits roll over? Recommendation: no, to keep it simple.
4. **Grandfathering**: Existing users get Pro for free for N months?
5. **Self-hosted**: Should self-hosted deployments bypass billing entirely? (Likely yes — the framework is open-source.)
