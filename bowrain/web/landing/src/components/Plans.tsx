import { Check, Gift, Zap, Users, Building2 } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { CONTACT_EMAIL, SIGNUP_URL } from "../links";
import { useReveal } from "../useReveal";

// Tiers mirror the real billing model one-for-one:
// bowrain/billing/plans.go (plan ids, monthly credits + the one-time trial
// grant, seat/project limits, feature gates) and DECISIONS L4 ($0 / $25 / $20
// per seat / custom; $5 credit pack = 200K credits, packs don't expire).
// Never per-word pricing.
const CREDITS = {
  trial: "200K",
  pro: "2M",
  team: "8M",
} as const;

type Tier = {
  id: "free" | "pro" | "team" | "enterprise";
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

const TIERS: Tier[] = [
  {
    id: "free",
    name: t("Free"),
    icon: Gift,
    description: t("For an individual evaluating the platform or running a single project."),
    price: "$0",
    priceNote: t("forever"),
    cta: t("Get started"),
    ctaHref: SIGNUP_URL,
    features: [
      t("{credits} one-time AI trial credits", { credits: CREDITS.trial }),
      t("1 project, 1 seat"),
      t("All formats and workflow tools"),
      t("Content memory & terminology"),
      t("Shared editor with review"),
      t("Bring your own AI key — uses no credits"),
      t("Community support"),
    ],
  },
  {
    id: "pro",
    name: t("Pro"),
    icon: Zap,
    description: t("For a practitioner running several projects with connectors and the API."),
    price: "$25",
    priceNote: t("/month"),
    cta: t("Start free trial"),
    ctaHref: SIGNUP_URL,
    features: [
      t("Everything in Free, plus:"),
      t("{credits} AI credits / month", { credits: CREDITS.pro }),
      t("Up to 10 projects, 3 seats"),
      t("Git connector"),
      t("REST API access"),
    ],
  },
  {
    id: "team",
    name: t("Team"),
    icon: Users,
    description: t("For teams that need collaboration, shared review, and automation."),
    price: "$20",
    priceNote: t("/seat/month"),
    cta: t("Start free trial"),
    ctaHref: SIGNUP_URL,
    featured: true,
    features: [
      t("Everything in Pro, plus:"),
      t("{credits} AI credits / month", { credits: CREDITS.team }),
      t("Unlimited projects & seats"),
      t("Custom connectors"),
      t("Priority support"),
    ],
  },
  {
    id: "enterprise",
    name: t("Enterprise"),
    icon: Building2,
    description: t("For organizations with SSO, compliance, and deployment requirements."),
    price: t("Custom"),
    priceNote: "",
    cta: t("Talk to us"),
    ctaHref: `mailto:${CONTACT_EMAIL}`,
    features: [
      t("Everything in Team, plus:"),
      t("Unlimited AI credits"),
      t("SSO / SAML"),
      t("Audit trails"),
      t("On-premise deployment option"),
      t("Dedicated support & SLA"),
    ],
  },
];

export function Plans() {
  const ref = useReveal();

  return (
    <section id="pricing" className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">Plans</h2>
          <p className="mt-3 text-muted-foreground">
            Paid plans include a monthly allowance of AI credits, refreshed automatically; every new
            workspace starts with a one-time grant of trial credits. Top-up packs are $5 for 200K
            credits and never expire — and if you bring your own AI key, those runs use no credits
            at all.
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
          toolchain is free — formats, workflow tools, and AI translation, on your own machine. No
          account required.
        </p>
      </div>
    </section>
  );
}
