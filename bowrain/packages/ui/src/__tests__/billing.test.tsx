import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CreditCounter } from "../components/billing/CreditCounter";
import { UsageBar } from "../components/billing/UsageBar";
import { SubscriptionBadge } from "../components/billing/SubscriptionBadge";
import { UpgradePrompt } from "../components/billing/UpgradePrompt";
import { CreditLedger } from "../components/billing/CreditLedger";
import { PlanComparisonTable } from "../components/billing/PlanComparisonTable";
import { PlanCard } from "../components/billing/PlanCard";
import type { CreditLedgerEntry, BillingPlan, BillingStatus } from "../types/api";
import type { ComparisonFeature } from "../components/billing/PlanComparisonTable";
import type { PlanFeature } from "../components/billing/PlanCard";

// ---------------------------------------------------------------------------
// CreditCounter
// ---------------------------------------------------------------------------
describe("CreditCounter", () => {
  it("renders remaining credits in X / Y format", () => {
    render(<CreditCounter creditsUsed={200} creditsTotal={500} />);
    expect(screen.getByText("300 / 500")).toBeInTheDocument();
  });

  it("compact mode shows just the remaining count", () => {
    render(<CreditCounter creditsUsed={200} creditsTotal={500} compact />);
    expect(screen.getByText("300")).toBeInTheDocument();
    // Should not show the "/ 500" part
    expect(screen.queryByText("300 / 500")).not.toBeInTheDocument();
  });

  it("formatCredits shows K abbreviation for thousands", () => {
    render(<CreditCounter creditsUsed={0} creditsTotal={5000} />);
    expect(screen.getByText("5K / 5K")).toBeInTheDocument();
  });

  it("formatCredits shows M abbreviation for millions", () => {
    render(<CreditCounter creditsUsed={0} creditsTotal={2000000} />);
    expect(screen.getByText("2.0M / 2.0M")).toBeInTheDocument();
  });

  it("clamps remaining to zero when used exceeds total", () => {
    render(<CreditCounter creditsUsed={600} creditsTotal={500} />);
    expect(screen.getByText("0 / 500")).toBeInTheDocument();
  });

  it("handles zero total gracefully", () => {
    render(<CreditCounter creditsUsed={0} creditsTotal={0} />);
    expect(screen.getByText("0 / 0")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// UsageBar
// ---------------------------------------------------------------------------
describe("UsageBar", () => {
  const futureDate = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000 + 5 * 60 * 60 * 1000);

  it("shows usage text with credits and percentage", () => {
    render(<UsageBar creditsUsed={300} creditsTotal={1000} weekEnd={futureDate} />);
    expect(screen.getByText("300 / 1K credits")).toBeInTheDocument();
    expect(screen.getByText("30%")).toBeInTheDocument();
  });

  it("shows countdown text", () => {
    render(<UsageBar creditsUsed={0} creditsTotal={1000} weekEnd={futureDate} />);
    expect(screen.getByText(/Resets in 3d/)).toBeInTheDocument();
  });

  it("shows 'Resetting now' when weekEnd is in the past", () => {
    const pastDate = new Date(Date.now() - 1000);
    render(<UsageBar creditsUsed={0} creditsTotal={1000} weekEnd={pastDate} />);
    expect(screen.getByText("Resetting now")).toBeInTheDocument();
  });

  it("handles zero total gracefully", () => {
    render(<UsageBar creditsUsed={0} creditsTotal={0} weekEnd={futureDate} />);
    expect(screen.getByText("0 / 0 credits")).toBeInTheDocument();
    expect(screen.getByText("0%")).toBeInTheDocument();
  });

  it("clamps percentage to 100 when used exceeds total", () => {
    render(<UsageBar creditsUsed={1500} creditsTotal={1000} weekEnd={futureDate} />);
    expect(screen.getByText("100%")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// SubscriptionBadge
// ---------------------------------------------------------------------------
describe("SubscriptionBadge", () => {
  it.each<[BillingPlan, string]>([
    ["free", "Free"],
    ["pro", "Pro"],
    ["team", "Team"],
    ["enterprise", "Enterprise"],
  ])("renders plan label for %s", (plan, label) => {
    render(<SubscriptionBadge plan={plan} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it.each<[BillingStatus, string]>([
    ["active", "Active"],
    ["trialing", "Trial"],
    ["past_due", "Past Due"],
    ["canceled", "Canceled"],
  ])("renders status label '%s' when provided", (status, label) => {
    render(<SubscriptionBadge plan="pro" status={status} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("hides status when not provided", () => {
    render(<SubscriptionBadge plan="pro" />);
    expect(screen.getByText("Pro")).toBeInTheDocument();
    expect(screen.queryByText("Active")).not.toBeInTheDocument();
    expect(screen.queryByText("Trial")).not.toBeInTheDocument();
    expect(screen.queryByText("Past Due")).not.toBeInTheDocument();
    expect(screen.queryByText("Canceled")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// UpgradePrompt
// ---------------------------------------------------------------------------
describe("UpgradePrompt", () => {
  it("shows feature name and plan requirement", () => {
    render(<UpgradePrompt feature="Git connectors" minimumPlan="pro" currentPlan="free" />);
    expect(screen.getByText("Git connectors requires a Pro plan")).toBeInTheDocument();
    expect(screen.getByText(/You are currently on the Free plan/)).toBeInTheDocument();
  });

  it("lists plan highlights", () => {
    render(<UpgradePrompt feature="SSO" minimumPlan="team" currentPlan="free" />);
    expect(screen.getByText("2M weekly credits")).toBeInTheDocument();
    expect(screen.getByText("Unlimited seats")).toBeInTheDocument();
    expect(screen.getByText("@bravo code execution")).toBeInTheDocument();
    expect(screen.getByText("Custom connectors")).toBeInTheDocument();
  });

  it("calls onUpgrade when button clicked", async () => {
    const user = userEvent.setup();
    const handleUpgrade = vi.fn();
    render(
      <UpgradePrompt
        feature="API access"
        minimumPlan="pro"
        currentPlan="free"
        onUpgrade={handleUpgrade}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Upgrade to Pro/ }));
    expect(handleUpgrade).toHaveBeenCalledTimes(1);
  });

  it("shows no highlights for free plan", () => {
    render(<UpgradePrompt feature="Nothing" minimumPlan="free" currentPlan="free" />);
    // Free plan has no highlights, so no list items should appear
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// CreditLedger
// ---------------------------------------------------------------------------
describe("CreditLedger", () => {
  function makeEntry(overrides: Partial<CreditLedgerEntry> = {}): CreditLedgerEntry {
    return {
      id: "entry-1",
      amount: -50,
      balanceAfter: 450,
      operation: "ai_translation",
      createdAt: "2026-03-20T10:30:00Z",
      ...overrides,
    };
  }

  it("renders table with entries", () => {
    const entries = [
      makeEntry({ id: "e1", operation: "ai_translation", amount: -50, balanceAfter: 450 }),
      makeEntry({ id: "e2", operation: "purchase", amount: 1000, balanceAfter: 1450 }),
    ];
    render(<CreditLedger entries={entries} />);
    // Operation labels appear in both filter buttons and table cells
    expect(screen.getAllByText("AI Translation").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Credit Purchase").length).toBeGreaterThanOrEqual(1);
    // Verify table headers are present
    expect(screen.getByText("Date")).toBeInTheDocument();
    expect(screen.getByText("Operation")).toBeInTheDocument();
    expect(screen.getByText("Amount")).toBeInTheDocument();
    expect(screen.getByText("Balance")).toBeInTheDocument();
  });

  it("shows operation labels", () => {
    const entries = [
      makeEntry({ id: "e1", operation: "ai_quality_check" }),
      makeEntry({ id: "e2", operation: "bravo_message" }),
      makeEntry({ id: "e3", operation: "plan_reset" }),
    ];
    render(<CreditLedger entries={entries} />);
    // Each label appears in both filter button and table cell
    expect(screen.getAllByText("AI Quality Check").length).toBe(2);
    expect(screen.getAllByText("@bravo Message").length).toBe(2);
    expect(screen.getAllByText("Weekly Reset").length).toBe(2);
  });

  it("shows 'No transactions found' when empty", () => {
    render(<CreditLedger entries={[]} />);
    expect(screen.getByText("No transactions found")).toBeInTheDocument();
  });

  it("filter buttons work to show subset", async () => {
    const user = userEvent.setup();
    const entries = [
      makeEntry({ id: "e1", operation: "ai_translation", amount: -50 }),
      makeEntry({ id: "e2", operation: "purchase", amount: 1000 }),
    ];
    render(<CreditLedger entries={entries} />);

    // Both entries visible initially: each label appears in filter button + table cell = 2
    expect(screen.getAllByText("AI Translation")).toHaveLength(2);
    expect(screen.getAllByText("Credit Purchase")).toHaveLength(2);

    // Click the "Credit Purchase" filter button
    await user.click(screen.getByRole("button", { name: "Credit Purchase" }));

    // AI Translation only in filter button now (table row hidden), so just 1
    expect(screen.getAllByText("AI Translation")).toHaveLength(1);
    // Credit Purchase still in filter button + table cell
    expect(screen.getAllByText("Credit Purchase")).toHaveLength(2);

    // Click "All" to reset
    await user.click(screen.getByRole("button", { name: "All" }));
    expect(screen.getAllByText("AI Translation")).toHaveLength(2);
    expect(screen.getAllByText("Credit Purchase")).toHaveLength(2);
  });

  it("does not show filter buttons when only one operation type", () => {
    const entries = [
      makeEntry({ id: "e1", operation: "ai_translation" }),
      makeEntry({ id: "e2", operation: "ai_translation" }),
    ];
    render(<CreditLedger entries={entries} />);
    expect(screen.queryByRole("button", { name: "All" })).not.toBeInTheDocument();
  });

  it("shows reference ID truncated to 8 chars", () => {
    const entries = [makeEntry({ id: "e1", referenceId: "abcdefghijklmnop" })];
    render(<CreditLedger entries={entries} />);
    expect(screen.getByText("abcdefgh")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// PlanComparisonTable
// ---------------------------------------------------------------------------
describe("PlanComparisonTable", () => {
  const features: ComparisonFeature[] = [
    {
      label: "Weekly credits",
      values: { free: "10K", pro: "500K", team: "2M", enterprise: "Custom" },
    },
    {
      label: "Git connectors",
      values: { free: false, pro: true, team: true, enterprise: true },
    },
  ];

  it("renders all plan columns", () => {
    render(<PlanComparisonTable features={features} />);
    expect(screen.getByText("Free")).toBeInTheDocument();
    expect(screen.getByText("Pro")).toBeInTheDocument();
    expect(screen.getByText("Team")).toBeInTheDocument();
    expect(screen.getByText("Enterprise")).toBeInTheDocument();
  });

  it("marks recommended plan", () => {
    render(<PlanComparisonTable features={features} recommendedPlan="team" />);
    expect(screen.getByText("Recommended")).toBeInTheDocument();
  });

  it("defaults recommended plan to pro", () => {
    render(<PlanComparisonTable features={features} />);
    // "Recommended" text should appear under the Pro column header
    expect(screen.getByText("Recommended")).toBeInTheDocument();
  });

  it("renders string values as text", () => {
    render(<PlanComparisonTable features={features} />);
    expect(screen.getByText("10K")).toBeInTheDocument();
    expect(screen.getByText("500K")).toBeInTheDocument();
    expect(screen.getByText("2M")).toBeInTheDocument();
    expect(screen.getByText("Custom")).toBeInTheDocument();
  });

  it("renders feature labels", () => {
    render(<PlanComparisonTable features={features} />);
    expect(screen.getByText("Weekly credits")).toBeInTheDocument();
    expect(screen.getByText("Git connectors")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// PlanCard
// ---------------------------------------------------------------------------
describe("PlanCard", () => {
  const defaultFeatures: PlanFeature[] = [
    { label: "API access", included: true },
    { label: "SSO", included: false },
  ];

  it("renders plan name, price, and credits", () => {
    render(
      <PlanCard
        plan="pro"
        name="Pro"
        price="$49"
        period="month"
        credits="500K weekly credits"
        features={defaultFeatures}
      />,
    );
    expect(screen.getByText("Pro")).toBeInTheDocument();
    expect(screen.getByText("$49")).toBeInTheDocument();
    expect(screen.getByText("/month")).toBeInTheDocument();
    expect(screen.getByText("500K weekly credits")).toBeInTheDocument();
  });

  it("renders feature list", () => {
    render(
      <PlanCard plan="pro" name="Pro" price="$49" credits="500K" features={defaultFeatures} />,
    );
    expect(screen.getByText("API access")).toBeInTheDocument();
    expect(screen.getByText("SSO")).toBeInTheDocument();
  });

  it("shows Recommended badge when recommended=true", () => {
    render(
      <PlanCard
        plan="pro"
        name="Pro"
        price="$49"
        credits="500K"
        features={defaultFeatures}
        recommended
      />,
    );
    expect(screen.getByText("Recommended")).toBeInTheDocument();
  });

  it("does not show Recommended badge when recommended=false", () => {
    render(
      <PlanCard plan="pro" name="Pro" price="$49" credits="500K" features={defaultFeatures} />,
    );
    expect(screen.queryByText("Recommended")).not.toBeInTheDocument();
  });

  it("button shows 'Current Plan' and is disabled when current=true", () => {
    render(
      <PlanCard
        plan="pro"
        name="Pro"
        price="$49"
        credits="500K"
        features={defaultFeatures}
        current
      />,
    );
    const button = screen.getByRole("button", { name: "Current Plan" });
    expect(button).toBeInTheDocument();
    expect(button).toBeDisabled();
  });

  it("button shows default 'Select' label when not current", () => {
    render(
      <PlanCard plan="pro" name="Pro" price="$49" credits="500K" features={defaultFeatures} />,
    );
    expect(screen.getByRole("button", { name: "Select" })).toBeInTheDocument();
  });

  it("button shows custom ctaLabel", () => {
    render(
      <PlanCard
        plan="pro"
        name="Pro"
        price="$49"
        credits="500K"
        features={defaultFeatures}
        ctaLabel="Get Started"
      />,
    );
    expect(screen.getByRole("button", { name: "Get Started" })).toBeInTheDocument();
  });

  it("calls onSelect when button clicked", async () => {
    const user = userEvent.setup();
    const handleSelect = vi.fn();
    render(
      <PlanCard
        plan="pro"
        name="Pro"
        price="$49"
        credits="500K"
        features={defaultFeatures}
        onSelect={handleSelect}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Select" }));
    expect(handleSelect).toHaveBeenCalledTimes(1);
  });

  it("does not call onSelect when current (button disabled)", async () => {
    const user = userEvent.setup();
    const handleSelect = vi.fn();
    render(
      <PlanCard
        plan="pro"
        name="Pro"
        price="$49"
        credits="500K"
        features={defaultFeatures}
        current
        onSelect={handleSelect}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Current Plan" }));
    expect(handleSelect).not.toHaveBeenCalled();
  });

  it("renders description when provided", () => {
    render(
      <PlanCard
        plan="pro"
        name="Pro"
        price="$49"
        credits="500K"
        features={defaultFeatures}
        description="Best for small teams"
      />,
    );
    expect(screen.getByText("Best for small teams")).toBeInTheDocument();
  });
});
