import { Check, Gift, Zap, Users, Building2 } from "lucide-react";

// Tiers mirror the real billing model in bowrain/billing/plans.go:
// plan IDs free/pro/team/enterprise, weekly AI credits, and seat/project
// limits. Prices are not defined in code, so they are shown as placeholders
// (tier + credits + cadence) rather than invented dollar amounts.
const CREDITS = {
  free: "50K",
  pro: "500K",
  team: "2M",
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
  ctaStyle: string;
  featured?: boolean;
  features: string[];
};

const TIERS: Tier[] = [
  {
    id: "free",
    name: "Free",
    icon: Gift,
    description: "For an individual evaluating the platform or running a single project.",
    price: "$0",
    priceNote: "/mo",
    cta: "Get started",
    ctaHref: "#get-started",
    ctaStyle:
      "border border-neutral-700 bg-neutral-900/50 text-neutral-200 hover:border-neutral-500 hover:text-white",
    features: [
      `${CREDITS.free} AI credits / week`,
      "1 project, 1 seat",
      "All formats and workflow tools",
      "Translation memory & terminology",
      "Visual translation editor",
      "Community support",
    ],
  },
  {
    id: "pro",
    name: "Pro",
    icon: Zap,
    description: "For a practitioner running several projects with connectors and the API.",
    price: "Pricing",
    priceNote: "billed monthly or annually",
    cta: "Start free trial",
    ctaHref: "#get-started",
    ctaStyle:
      "border border-neutral-700 bg-neutral-900/50 text-neutral-200 hover:border-neutral-500 hover:text-white",
    features: [
      "Everything in Free, plus:",
      `${CREDITS.pro} AI credits / week`,
      "Up to 10 projects, 3 seats",
      "Git connector",
      "REST API access",
      "Custom MT providers",
    ],
  },
  {
    id: "team",
    name: "Team",
    icon: Users,
    description: "For teams that need collaboration, every connector, and automation.",
    price: "Pricing",
    priceNote: "billed monthly or annually",
    cta: "Start free trial",
    ctaHref: "#get-started",
    ctaStyle: "bg-brand-500 text-white hover:bg-brand-600",
    featured: true,
    features: [
      "Everything in Pro, plus:",
      `${CREDITS.team} AI credits / week`,
      "Unlimited projects & seats",
      "Custom connectors",
      "Bravo code execution",
      "Team collaboration & review",
      "Priority support",
    ],
  },
  {
    id: "enterprise",
    name: "Enterprise",
    icon: Building2,
    description: "For organizations with SSO, compliance, and deployment requirements.",
    price: "Custom",
    priceNote: "",
    cta: "Talk to us",
    ctaHref: "mailto:hello@bowrain.com",
    ctaStyle:
      "border border-neutral-700 bg-neutral-900/50 text-neutral-200 hover:border-neutral-500 hover:text-white",
    features: [
      "Everything in Team, plus:",
      "Unlimited AI credits",
      "SSO / SAML",
      "Audit trails",
      "On-premise deployment option",
      "Dedicated support & SLA",
    ],
  },
];

export function Plans() {
  return (
    <section id="plans" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">Plans</h2>
        <p className="mt-3 text-neutral-400">
          The open-source{" "}
          <code className="rounded bg-neutral-800/50 px-1.5 py-0.5 text-xs text-neutral-300">
            kapi
          </code>{" "}
          toolchain is free and runs anywhere. Add Bowrain when a team needs shared governance,
          connectors, collaboration, and version history on the server. Every plan includes a weekly
          allowance of AI translation credits.
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
                  ? "border-brand-500/50 bg-brand-500/5 shadow-lg shadow-brand-500/5"
                  : "border-neutral-800 bg-neutral-900/30"
              }`}
            >
              {tier.featured && (
                <div className="absolute -top-3 left-1/2 -translate-x-1/2 rounded-full bg-brand-500 px-3 py-0.5 text-xs font-medium text-white">
                  Most popular
                </div>
              )}

              <div className="mb-4 flex items-center gap-3">
                <div
                  className={`flex h-10 w-10 items-center justify-center rounded-lg ${
                    tier.featured ? "bg-brand-500/20" : "bg-neutral-800"
                  }`}
                >
                  <Icon
                    className={`h-5 w-5 ${tier.featured ? "text-brand-400" : "text-neutral-400"}`}
                  />
                </div>
                <h3 className="text-xl font-semibold text-white">{tier.name}</h3>
              </div>

              <p className="text-sm text-neutral-400">{tier.description}</p>

              <div className="mt-6 mb-6">
                <div className="text-3xl font-bold text-white">{tier.price}</div>
                {hasNote && <div className="mt-1 text-sm text-neutral-500">{tier.priceNote}</div>}
              </div>

              <a
                href={tier.ctaHref}
                className={`mb-8 flex items-center justify-center rounded-xl px-6 py-3 text-sm font-medium transition ${tier.ctaStyle}`}
              >
                {tier.cta}
              </a>

              <ul className="flex-1 space-y-3">
                {tier.features.map((feature, i) => {
                  const isHeader = feature.endsWith(":");
                  return (
                    <li
                      key={i}
                      className={`flex items-start gap-2 text-sm ${isHeader ? "text-neutral-300 font-medium" : "text-neutral-400"}`}
                    >
                      {!isHeader && (
                        <Check
                          className={`mt-0.5 h-4 w-4 shrink-0 ${tier.featured ? "text-brand-400" : "text-neutral-600"}`}
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

      <p className="mt-8 text-center text-sm text-neutral-600">
        The open-source{" "}
        <code className="rounded bg-neutral-800/50 px-1.5 py-0.5 text-xs text-neutral-400">
          kapi
        </code>{" "}
        toolchain is free — formats, workflow tools, and AI translation, on your own machine. No
        account required.
      </p>
    </section>
  );
}
