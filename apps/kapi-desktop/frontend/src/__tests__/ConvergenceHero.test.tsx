import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, afterEach } from "vitest";
import { ConvergenceHero, ConvergePlanDialog } from "../components/ConvergenceHero";
import { api } from "../hooks/useApi";
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
    note: "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls).",
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
    note: "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining units (no tokenizer, no provider calls).",
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
  afterEach(() => {
    vi.restoreAllMocks();
  });

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

  it("launches the run from the primary button, then navigates to the runner", async () => {
    const onBringUpToDate = vi.fn();
    const bringUpToDate = vi.spyOn(api, "bringUpToDate").mockResolvedValue(null);
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={onBringUpToDate}
        convergence={driftedReport}
        plan={driftedPlan}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Bring the project up to date" }));
    // The hero itself owns the launch; navigation happens only after success.
    await waitFor(() => expect(onBringUpToDate).toHaveBeenCalledTimes(1));
    expect(bringUpToDate).toHaveBeenCalledWith("t1");
  });

  it("surfaces a synchronous launch error inline and stays home", async () => {
    const onBringUpToDate = vi.fn();
    vi.spyOn(api, "bringUpToDate").mockRejectedValue(
      new Error("no target languages configured (defaults.target_languages)"),
    );
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={onBringUpToDate}
        convergence={driftedReport}
        plan={driftedPlan}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Bring the project up to date" }));

    // The error renders inline on the hero…
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("no target languages configured");
    // …and the user never navigates to a dead runner view.
    expect(onBringUpToDate).not.toHaveBeenCalled();
  });

  it("renders the quiet reopen state when the project files are missing", async () => {
    vi.spyOn(api, "getConvergence").mockRejectedValue(
      new Error("project files are missing or moved — reopen the project"),
    );
    vi.spyOn(api, "getConvergePlan").mockRejectedValue(
      new Error("project files are missing or moved — reopen the project"),
    );
    render(<ConvergenceHero tabID="t1" onBringUpToDate={vi.fn()} />);

    expect(await screen.findByText("Project files are missing or moved")).toBeInTheDocument();
    // Quiet terminal state: no run controls, no error banner.
    expect(
      screen.queryByRole("button", { name: "Bring the project up to date" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
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
    await userEvent.click(screen.getByRole("button", { name: "Preview the catch-up plan" }));

    // Per-(collection, locale) rows: missing / content memory-exact / AI-remainder.
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("docs");
    expect(dialog).toHaveTextContent("12");
    expect(dialog).toHaveTextContent("Memory exact");
    expect(dialog).toHaveTextContent("AI work");
    // Total token estimate + the disclosed heuristic note.
    expect(dialog).toHaveTextContent("1360");
    expect(dialog).toHaveTextContent(/chars \/ 4/);

    // Confirm launches the run and closes the dialog.
    const confirm = screen.getAllByRole("button", { name: "Bring up to date" });
    await userEvent.click(confirm[confirm.length - 1]);
    await waitFor(() => expect(onBringUpToDate).toHaveBeenCalledTimes(1));
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

describe("ConvergePlanDialog subscription wording", () => {
  const subscriptionPlan: ConvergePlan = {
    ...driftedPlan,
    plan: {
      ...driftedPlan.plan,
      provider: "claude-code",
      subscription: true,
      note: "AI work runs on your Claude subscription — the token estimate is scale, not a metered cost. Content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 (no tokenizer, no provider calls).",
    },
  };

  it("shows the subscription line when the provider is claude-code", () => {
    render(
      <ConvergePlanDialog
        open
        onOpenChange={() => {}}
        plan={subscriptionPlan}
        onConfirm={() => {}}
      />,
    );
    expect(
      screen.getByText("AI work runs on your Claude subscription, with no per-token API cost."),
    ).toBeInTheDocument();
  });

  it("keeps the metered framing for API-key providers", () => {
    render(
      <ConvergePlanDialog open onOpenChange={() => {}} plan={driftedPlan} onConfirm={() => {}} />,
    );
    expect(screen.queryByText(/runs on your Claude subscription/)).not.toBeInTheDocument();
  });

  it("the hero notes the subscription while drifted", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        convergence={driftedReport}
        plan={subscriptionPlan}
        onBringUpToDate={() => {}}
      />,
    );
    expect(screen.getByText(/AI runs on your Claude subscription\./)).toBeInTheDocument();
  });
});
