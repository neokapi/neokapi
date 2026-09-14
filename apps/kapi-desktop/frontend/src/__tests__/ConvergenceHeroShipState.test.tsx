import { render, screen } from "./testUtils";
import { describe, it, expect, vi } from "vitest";
import { ConvergenceHero } from "../components/ConvergenceHero";
import type { ConvergePlan, ConvergenceReport } from "../types/api";

// The hero reads each scope's ship state. Green gates say nothing about the
// languages no gate matches, and a language no gate matches can still be
// withheld by a failing check, so neither may read as "up to date" with the
// gates alone.

const convergedPlan: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: null,
    totals: { missingTarget: 0, tmExact: 0, aiRemaining: 0, tokenEstimate: 0 },
    note: "",
  },
  changedFiles: 0,
  removedFiles: 0,
  storeMissing: false,
  versionStale: false,
};

describe("ConvergenceHero ship states", () => {
  it("names the languages no gate matches beside green gates", () => {
    const report: ConvergenceReport = {
      locales: [
        {
          locale: "fr-FR",
          collection: "docs",
          total: 40,
          pct: { translated: 100 },
          gated: true,
          shippable: true,
          shipState: "shippable",
        },
        {
          locale: "nb",
          collection: "docs",
          total: 40,
          pct: { translated: 100 },
          gated: false,
          shippable: true,
          shipState: "not_gated",
        },
      ],
      review: [],
    };
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={report}
        plan={convergedPlan}
        lastRunError={null}
      />,
    );
    expect(screen.getByText("Up to date · all gates green")).toBeInTheDocument();
    const notGated = document.querySelector("[data-slot='hero-not-gated']");
    expect(notGated).not.toBeNull();
    expect(notGated?.textContent).toContain("nb");
  });

  it("does not call an ungated scope withheld by a failing check up to date", () => {
    const report: ConvergenceReport = {
      locales: [
        {
          locale: "nb",
          collection: "docs",
          total: 40,
          pct: { translated: 100 },
          gated: false,
          shippable: false,
          shipState: "withheld",
        },
      ],
      review: [],
    };
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={report}
        plan={convergedPlan}
        lastRunError={null}
      />,
    );
    expect(screen.queryByText("Up to date")).not.toBeInTheDocument();
    expect(screen.getByText(/1 scope\(s\) withheld/)).toBeInTheDocument();
  });
});
