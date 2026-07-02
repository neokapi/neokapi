import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ConvergenceHero, ConvergePlanDialog } from "../components/ConvergenceHero";
import type { ConvergePlan, ConvergenceReport } from "../types/api";

const driftedPlan: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: [
      {
        locale: "fr-FR",
        collection: "docs",
        missingTarget: 12,
        tmExact: 4,
        aiRemaining: 8,
        tokenEstimate: 640,
      },
      {
        locale: "de-DE",
        collection: "docs",
        missingTarget: 9,
        tmExact: 0,
        aiRemaining: 9,
        tokenEstimate: 720,
      },
    ],
    totals: { missingTarget: 21, tmExact: 4, aiRemaining: 17, tokenEstimate: 1360 },
    note: "TM leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls).",
  },
  changedFiles: 3,
  removedFiles: 0,
  storeMissing: false,
  versionStale: false,
};

const driftedReport: ConvergenceReport = {
  locales: [
    {
      locale: "fr-FR",
      collection: "docs",
      total: 40,
      pct: { translated: 70 },
      gated: true,
      shippable: false,
    },
  ],
  review: [
    { locale: "fr-FR", file: "docs/a.md", key: "k1", source: "Hello" },
    { locale: "fr-FR", file: "docs/a.md", key: "k2", source: "World" },
  ],
};

const convergedPlan: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: null,
    totals: { missingTarget: 0, tmExact: 0, aiRemaining: 0, tokenEstimate: 0 },
    note: "TM leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls).",
  },
  changedFiles: 0,
  removedFiles: 0,
  storeMissing: false,
  versionStale: false,
};

const convergedReport: ConvergenceReport = {
  locales: [
    {
      locale: "fr-FR",
      collection: "docs",
      total: 40,
      pct: { translated: 100, reviewed: 100 },
      gated: true,
      shippable: true,
    },
  ],
  review: [],
};

describe("ConvergenceHero", () => {
  it("renders the drift summary and an enabled Bring up to date", () => {
    const onBringUpToDate = vi.fn();
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={onBringUpToDate}
        convergence={driftedReport}
        plan={driftedPlan}
      />,
    );
    // "N source files changed · M units missing targets · K parked for review"
    expect(screen.getByText(/3 source file\(s\) changed/)).toBeInTheDocument();
    expect(screen.getByText(/21 unit\(s\) missing targets/)).toBeInTheDocument();
    expect(screen.getByText(/2 parked for review/)).toBeInTheDocument();

    const btn = screen.getByRole("button", { name: "Bring the project up to date" });
    expect(btn).toBeEnabled();
  });

  it("launches the run from the primary button", async () => {
    const onBringUpToDate = vi.fn();
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={onBringUpToDate}
        convergence={driftedReport}
        plan={driftedPlan}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Bring the project up to date" }));
    expect(onBringUpToDate).toHaveBeenCalledTimes(1);
  });

  it("renders the calm state with the button disabled when converged", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={convergedReport}
        plan={convergedPlan}
      />,
    );
    expect(screen.getByText("Up to date · all gates green")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bring the project up to date" })).toBeDisabled();
  });

  it("opens the pre-flight plan dialog from Plan… and confirms into a run", async () => {
    const onBringUpToDate = vi.fn();
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={onBringUpToDate}
        convergence={driftedReport}
        plan={driftedPlan}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Preview the convergence plan" }));

    // Per-(collection, locale) rows: missing / TM-exact / AI-remainder.
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("docs");
    expect(dialog).toHaveTextContent("12");
    expect(dialog).toHaveTextContent("TM exact");
    expect(dialog).toHaveTextContent("AI work");
    // Total token estimate + the disclosed heuristic note.
    expect(dialog).toHaveTextContent("1360");
    expect(dialog).toHaveTextContent(/chars \/ 4/);

    // Confirm launches the run and closes the dialog.
    const confirm = screen.getAllByRole("button", { name: "Bring up to date" });
    await userEvent.click(confirm[confirm.length - 1]);
    expect(onBringUpToDate).toHaveBeenCalledTimes(1);
  });
});

describe("ConvergePlanDialog", () => {
  it("renders the empty state when nothing is pending", () => {
    render(
      <ConvergePlanDialog open onOpenChange={vi.fn()} plan={convergedPlan} onConfirm={vi.fn()} />,
    );
    expect(
      screen.getByText("Nothing to do: every unit has a committed target."),
    ).toBeInTheDocument();
  });

  it("cancel closes without launching", async () => {
    const onOpenChange = vi.fn();
    const onConfirm = vi.fn();
    render(
      <ConvergePlanDialog
        open
        onOpenChange={onOpenChange}
        plan={driftedPlan}
        onConfirm={onConfirm}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
