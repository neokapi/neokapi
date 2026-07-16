import { useState } from "react";
import {
  PlanCard,
  PlanComparisonTable,
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@neokapi/ui";
import type { ComparisonFeature, PlanFeature } from "@neokapi/ui";
import { ChevronDown } from "lucide-react";

// ---------------------------------------------------------------------------
// Plan data
// ---------------------------------------------------------------------------

// Only capabilities the product actually has and actually gates. A feature listed
// here that nothing enforces is a promise the code does not keep — "Custom MT
// providers" was listed for months after MT providers were removed from the
// product, and Git connectors were listed as paid while every Free workspace
// could use them (both fixed in epic 005).
const freePlanFeatures: PlanFeature[] = [
  { label: "1 project", included: true },
  { label: "Bring your own AI key (no credits used)", included: true },
  { label: "Git connectors", included: false },
  { label: "API access", included: false },
];

const proPlanFeatures: PlanFeature[] = [
  { label: "Up to 10 projects", included: true },
  { label: "3 seats", included: true },
  { label: "Git connectors", included: true },
  { label: "API access", included: true },
  { label: "SSO/SAML", included: false },
];

const teamPlanFeatures: PlanFeature[] = [
  { label: "Everything in Pro", included: true },
  { label: "Unlimited projects", included: true },
  { label: "Unlimited seats", included: true },
  { label: "SSO/SAML", included: false },
];

const enterprisePlanFeatures: PlanFeature[] = [
  { label: "Everything in Team", included: true },
  { label: "SSO/SAML", included: true },
  { label: "Dedicated support", included: true },
  { label: "Custom agreements", included: true },
  { label: "SLA guarantees", included: true },
];

const comparisonFeatures: ComparisonFeature[] = [
  {
    label: "Weekly AI Credits",
    values: { free: "50K", pro: "500K", team: "2M", enterprise: "Custom" },
  },
  {
    label: "Projects",
    values: { free: "1", pro: "10", team: "Unlimited", enterprise: "Unlimited" },
  },
  {
    label: "Seats",
    values: { free: "1", pro: "3", team: "Unlimited", enterprise: "Unlimited" },
  },
  {
    label: "Git Connectors",
    values: { free: false, pro: true, team: true, enterprise: true },
  },
  {
    label: "API Access",
    values: { free: false, pro: true, team: true, enterprise: true },
  },
  {
    label: "Bring your own AI key",
    values: { free: true, pro: true, team: true, enterprise: true },
  },
  {
    label: "SSO/SAML",
    values: { free: false, pro: false, team: false, enterprise: true },
  },
];

const faqItems = [
  {
    q: "How do weekly credits work?",
    a: "Every Monday at 00:00 UTC, your credit balance resets to your plan's weekly allocation. One credit equals one AI token (input or output). AI translation and quality checks consume 1 credit per token.",
  },
  {
    q: "What happens when I run out of credits?",
    a: "AI features pause until the next weekly reset. You can also buy a credit pack ($5 for 200K credits) at any time — nothing is ever purchased automatically on your behalf.",
  },
  {
    q: "Can I use my own AI provider key?",
    a: "Yes, on every plan including Free. Work that runs on your own key uses no credits at all — you pay your provider directly, and Bowrain's weekly allowance stays untouched.",
  },
  {
    q: "Can I change plans at any time?",
    a: "Yes. Upgrades take effect immediately. Downgrades apply at the end of your current billing period. You can manage your subscription from Workspace Settings > Billing.",
  },
  {
    q: "How does seat-based pricing work?",
    a: "The Team plan is priced per seat per month. You only pay for the seats you use, and credits are shared across all workspace members. Self-serve checkout covers up to 50 seats; beyond that, talk to us.",
  },
  {
    q: "Do unused credits roll over?",
    a: "Weekly plan credits do not roll over — they reset each Monday, which keeps costs predictable. Credit packs are different: they never expire, and they are only drawn from after your weekly allowance runs out.",
  },
  {
    q: "Is there a free trial?",
    a: "Every new workspace starts with a 14-day Pro trial — no card required. When it ends, the workspace moves to the Free plan, which is free forever and includes 50K weekly credits and full access to the translation editor.",
  },
];

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

function FAQItem({ question, answer }: { question: string; answer: string }) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex w-full items-center justify-between py-3 text-left text-sm font-medium text-foreground hover:text-primary transition-colors cursor-pointer">
        {question}
        <ChevronDown
          className={`h-4 w-4 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`}
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <p className="pb-3 text-sm text-muted-foreground leading-relaxed">{answer}</p>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function PricingRoute() {
  return (
    <div className="mx-auto max-w-6xl px-4 py-12">
      {/* Header */}
      <div className="text-center mb-12">
        <h1 className="text-3xl font-bold text-foreground sm:text-4xl">
          Simple, transparent pricing
        </h1>
        <p className="mt-3 text-lg text-muted-foreground">
          Start free. Scale as you grow. Weekly credits keep costs predictable.
        </p>
      </div>

      {/* Plan Cards */}
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4 mb-16">
        <PlanCard
          plan="free"
          name="Free"
          price="$0"
          description="Get started with AI-powered localization"
          credits="50K credits / week"
          features={freePlanFeatures}
          ctaLabel="Get Started"
          onSelect={() => (window.location.href = "/api/v1/auth/login")}
        />
        <PlanCard
          plan="pro"
          name="Pro"
          price="$25"
          period="mo"
          description="For professionals and small teams"
          credits="500K credits / week"
          recommended
          features={proPlanFeatures}
          ctaLabel="Start with Pro"
        />
        <PlanCard
          plan="team"
          name="Team"
          price="$20"
          period="seat/mo"
          description="For growing teams"
          credits="2M credits / week"
          features={teamPlanFeatures}
          ctaLabel="Start with Team"
        />
        <PlanCard
          plan="enterprise"
          name="Enterprise"
          price="Custom"
          description="For large organizations"
          credits="Custom credit allocation"
          features={enterprisePlanFeatures}
          ctaLabel="Contact Sales"
        />
      </div>

      {/* Comparison Table */}
      <div className="mb-16">
        <h2 className="text-xl font-semibold text-foreground mb-6 text-center">
          Compare all features
        </h2>
        <PlanComparisonTable features={comparisonFeatures} recommendedPlan="pro" />
      </div>

      {/* FAQ */}
      <div className="mx-auto max-w-2xl">
        <h2 className="text-xl font-semibold text-foreground mb-6 text-center">
          Frequently asked questions
        </h2>
        <div className="divide-y divide-border">
          {faqItems.map((item) => (
            <FAQItem key={item.q} question={item.q} answer={item.a} />
          ))}
        </div>
      </div>
    </div>
  );
}
