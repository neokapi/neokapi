import { Check, Gift, Zap, Users, Building2 } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { CONTACT_EMAIL, SIGNUP_URL, signupUrl } from "../links";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_PLANS } from "../sections";
import plansCatalog from "../generated/plans.json";

// ── Facts (generated) vs marketing copy (hand-authored) ──────────────────────
// The FACTUAL fields — plan ids, display credits, seat/project limits, per-seat
// and self-serve flags — come from plans.json, which is generated from
// bowrain/billing/plans.go by `go generate ./...` and drift-gated by
// billing/plans_gen_test.go. Change a credit allowance or limit there and
// regenerate; never hand-edit the numbers here.
//
// Everything below is hand-authored marketing copy the backend has no equivalent
// for — dollar prices (they live in Stripe, DECISIONS L4), descriptions, feature
// phrasing, icons, and CTA labels — keyed by plan id and woven together with the
// generated facts at render time.

type PlanId = "free" | "pro" | "team" | "enterprise";

type PlanFacts = (typeof plansCatalog.plans)[number];

/** Compact human credits: 200000 → "200K", 2_000_000 → "2M". */
function formatCredits(n: number): string {
  if (n >= 1_000_000) return `${n / 1_000_000}M`;
  if (n >= 1_000) return `${n / 1_000}K`;
  return String(n);
}

type Marketing = {
  icon: typeof Zap;
  name: string;
  description: string;
  price: string;
  priceNote: string;
  cta: string;
  featured?: boolean;
  /** Seat hint carried on the CTA for per-seat plans (server clamps to members). */
  defaultSeats?: number;
  /** Feature bullets: hand-authored phrasing, generated numbers woven in. */
  features: (f: PlanFacts) => string[];
};

const MARKETING: Record<PlanId, Marketing> = {
  free: {
    icon: Gift,
    name: t("Free"),
    description: t("For an individual evaluating the platform or running a single project."),
    price: "$0",
    priceNote: t("forever"),
    cta: t("Get started"),
    features: (f) => [
      t("{credits} one-time AI trial credits", {
        credits: formatCredits(plansCatalog.trialGrantCredits),
      }),
      t("{markets} markets, {brands} brand", { markets: f.maxMarkets, brands: f.maxBrands }),
      t("Unlimited members and unlimited checks"),
      t("All formats and workflow tools"),
      t("Content memory & terminology"),
      t("Shared editor with review"),
      t("Bring your own AI key: uses no credits"),
      t("REST API access"),
      t("Community support"),
    ],
  },
  pro: {
    icon: Zap,
    name: t("Pro"),
    description: t("For a practitioner running several projects with connectors and the API."),
    price: "$25",
    priceNote: t("/month"),
    cta: t("Start free trial"),
    features: (f) => [
      t("Everything in Free, plus:"),
      t("{credits} AI credits / month", { credits: formatCredits(f.monthlyCredits) }),
      t("Up to {markets} markets, {brands} brand", { markets: f.maxMarkets, brands: f.maxBrands }),
      t("{custodians} custodian: someone who governs a brand, product or channel", {
        custodians: f.maxCustodians,
      }),
      t("Git connector"),
    ],
  },
  team: {
    icon: Users,
    name: t("Team"),
    description: t("For teams that need collaboration, shared review, and automation."),
    price: "$20",
    priceNote: t("/seat/month"),
    cta: t("Start free trial"),
    featured: true,
    defaultSeats: 3,
    features: (f) => [
      t("Everything in Pro, plus:"),
      t("{credits} AI credits / month", { credits: formatCredits(f.monthlyCredits) }),
      t("Up to {markets} markets across {brands} brands", {
        markets: f.maxMarkets,
        brands: f.maxBrands,
      }),
      t("Up to {custodians} custodians", { custodians: f.maxCustodians }),
      t("Custom connectors"),
      t("Priority support"),
    ],
  },
  enterprise: {
    icon: Building2,
    name: t("Enterprise"),
    description: t("For organizations with SSO, compliance, and deployment requirements."),
    price: t("Custom"),
    priceNote: "",
    cta: t("Talk to us"),
    features: () => [
      t("Everything in Team, plus:"),
      t("Unlimited markets, brands and custodians"),
      t("Unlimited AI credits"),
      t("SSO / SAML"),
      t("Audit trails"),
      t("On-premise deployment option"),
      t("Dedicated support & SLA"),
    ],
  },
};

/**
 * CTA target for a tier. A self-serve paid tier carries `?plan=<id>` (Team also a
 * default `&seats=`) so the app pre-selects it after signup; Free lands at the app
 * root (no param needed), Enterprise opens a mailto. The self-serve set is read
 * from the generated catalog, so retiring or adding a self-serve plan flows
 * through automatically.
 */
function ctaHref(id: PlanId, facts: PlanFacts): string {
  if (id === "enterprise") return `mailto:${CONTACT_EMAIL}`;
  if (!facts.selfServe) return SIGNUP_URL;
  return signupUrl({
    plan: id,
    seats: facts.perSeat ? MARKETING[id].defaultSeats : undefined,
  });
}

type Tier = {
  id: PlanId;
  name: string;
  icon: typeof Zap;
  description: string;
  price: string;
  priceNote: string;
  cta: string;
  ctaHref: string;
  featured?: boolean;
  features: string[];
};

// Tier order follows the generated catalog (free → pro → team → enterprise).
const TIERS: Tier[] = plansCatalog.plans.map((facts): Tier => {
  const id = facts.id as PlanId;
  const m = MARKETING[id];
  return {
    id,
    name: m.name,
    icon: m.icon,
    description: m.description,
    price: m.price,
    priceNote: m.priceNote,
    cta: m.cta,
    ctaHref: ctaHref(id, facts),
    featured: m.featured,
    features: m.features(facts),
  };
});

export function Plans() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_PLANS);
  const ref = useReveal();

  return (
    <section id="pricing" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">Plans</h2>
          <p className="mt-3 text-muted-foreground">
            Paid plans include a monthly allowance of AI credits, refreshed automatically; every new
            workspace starts with a one-time grant of trial credits. Top-up packs are $5 for 200K
            credits and never expire, and if you bring your own AI key, those runs use no credits at
            all.
          </p>
        </div>

        <div className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {TIERS.map((tier) => {
            const Icon = tier.icon;
            const hasNote = tier.priceNote !== "";

            return (
              <div
                key={tier.id}
                className={`relative flex flex-col rounded-xl border p-6 ${
                  tier.featured
                    ? "border-primary/50 bg-primary/5 shadow-lg shadow-primary/5"
                    : "border-border bg-card"
                }`}
              >
                {tier.featured && (
                  <div className="absolute -top-3 left-1/2 -translate-x-1/2 rounded-full bg-primary px-3 py-0.5 text-xs font-medium text-primary-foreground">
                    Most popular
                  </div>
                )}

                <div className="mb-4 flex items-center gap-3">
                  <div
                    className={`flex h-10 w-10 items-center justify-center rounded-lg ${
                      tier.featured ? "bg-primary/15" : "bg-secondary"
                    }`}
                  >
                    <Icon
                      className={`h-5 w-5 ${tier.featured ? "text-primary" : "text-muted-foreground"}`}
                    />
                  </div>
                  <h3 className="text-xl font-semibold">{tier.name}</h3>
                </div>

                <p className="text-sm text-muted-foreground">{tier.description}</p>

                <div className="mt-6 mb-6">
                  <div className="text-3xl font-bold">{tier.price}</div>
                  {hasNote && (
                    <div className="mt-1 text-sm text-muted-foreground">{tier.priceNote}</div>
                  )}
                </div>

                <a
                  href={tier.ctaHref}
                  className={`mb-8 flex items-center justify-center rounded-xl px-6 py-3 text-sm font-medium transition ${
                    tier.featured
                      ? "bg-primary text-primary-foreground hover:opacity-90"
                      : "border border-border bg-card hover:border-muted-foreground"
                  }`}
                >
                  {tier.cta}
                </a>

                <ul className="flex-1 space-y-3">
                  {tier.features.map((feature, i) => {
                    const isHeader = feature.endsWith(":");
                    return (
                      <li
                        key={i}
                        className={`flex items-start gap-2 text-sm ${
                          isHeader ? "font-medium" : "text-muted-foreground"
                        }`}
                      >
                        {!isHeader && (
                          <Check
                            className={`mt-0.5 h-4 w-4 shrink-0 ${
                              tier.featured ? "text-primary" : "text-muted-foreground/60"
                            }`}
                          />
                        )}
                        {feature}
                      </li>
                    );
                  })}
                </ul>
              </div>
            );
          })}
        </div>

        <p className="mt-8 text-center text-sm text-muted-foreground/80">
          The open-source{" "}
          <code className="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">
            kapi
          </code>{" "}
          toolchain is free: formats, workflow tools, and AI translation, on your own machine. No
          account required.
        </p>
      </div>
    </section>
  );
}
