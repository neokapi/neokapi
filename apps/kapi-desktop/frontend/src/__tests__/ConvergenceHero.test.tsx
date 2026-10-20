import { render, screen, waitFor, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import { ConvergenceHero, ConvergePlanDialog } from "../components/ConvergenceHero";
import { api } from "../hooks/useApi";
import { useProjectStoreSync } from "../hooks/useProjectStoreSync";
import type { ConvergePlan, ConvergenceReport } from "../types/api";

const driftedPlan: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: [
      {
        locale: "fr-FR",
        collection: "docs",
        missingTarget: 12,
        memoryExact: 4,
        aiRemaining: 8,
        tokenEstimate: 640,
      },
      {
        locale: "de-DE",
        collection: "docs",
        missingTarget: 9,
        memoryExact: 0,
        drafts: 5,
        aiRemaining: 4,
        tokenEstimate: 320,
      },
    ],
    totals: { missingTarget: 21, memoryExact: 4, drafts: 5, aiRemaining: 12, tokenEstimate: 960 },
    note: "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining blocks (no tokenizer, no provider calls).",
  },
  changedFiles: 0,
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
      pending: [{ state: "translated", actual: 70, required: 100 }],
      failingChecks: 2,
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
    totals: { missingTarget: 0, memoryExact: 0, aiRemaining: 0, tokenEstimate: 0 },
    note: "content-memory leverage counts exact-hash hits only; token estimate is source chars / 4 for the remaining blocks (no tokenizer, no provider calls).",
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
      pct: { translated: 100, established: 100 },
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

  it("counts the work by who does it, and leads each count to its place", async () => {
    const onOpenReview = vi.fn();
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        onOpenReview={onOpenReview}
        convergence={driftedReport}
        plan={driftedPlan}
        lastRunError={null}
      />,
    );
    expect(screen.getByText("Translations are missing")).toBeInTheDocument();
    const counts = document.querySelector('[data-slot="hero-counts"]') as HTMLElement;
    expect(counts).toHaveTextContent("21blocks to translate");
    expect(counts).toHaveTextContent("2waiting for review");
    expect(counts).toHaveTextContent("1language held back");

    await userEvent.click(within(counts).getByRole("button", { name: /waiting for review/ }));
    expect(onOpenReview).toHaveBeenCalledTimes(1);

    const btn = screen.getByRole("button", { name: "Bring the project up to date" });
    expect(btn).toBeEnabled();
  });

  it("lists what holds each language back, and leads to Checks", async () => {
    const onOpenChecks = vi.fn();
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        onOpenReview={vi.fn()}
        onOpenChecks={onOpenChecks}
        convergence={driftedReport}
        plan={driftedPlan}
        lastRunError={null}
      />,
    );
    const counts = document.querySelector('[data-slot="hero-counts"]') as HTMLElement;
    await userEvent.click(within(counts).getByRole("button", { name: /language held back/ }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("70% translated; the ship gate needs 100%");
    expect(dialog).toHaveTextContent("2 failing a check");
    await userEvent.click(within(dialog).getByRole("button", { name: "Open Checks" }));
    expect(onOpenChecks).toHaveBeenCalledTimes(1);
  });

  it("reads the files itself instead of asking for an extract", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={driftedReport}
        plan={{ ...driftedPlan, storeMissing: true, changedFiles: 3 }}
        lastRunError={null}
      />,
    );
    const hero = document.querySelector('[data-slot="convergence-hero"]') as HTMLElement;
    expect(screen.getByText("Reading the project's files…")).toBeInTheDocument();
    expect(hero.textContent).not.toMatch(/extract/i);
    expect(document.querySelector('[data-slot="hero-counts"]')).toBeNull();
  });

  it("names a failed read of the files", async () => {
    vi.spyOn(api, "syncProjectStore").mockRejectedValue(
      new Error("read the project's files: open web/en/a.md: permission denied"),
    );
    function Synced() {
      useProjectStoreSync("t1");
      return (
        <ConvergenceHero
          tabID="t1"
          onBringUpToDate={vi.fn()}
          convergence={driftedReport}
          plan={{ ...driftedPlan, storeMissing: true }}
          lastRunError={null}
        />
      );
    }
    render(<Synced />);
    await waitFor(() =>
      expect(document.querySelector('[data-slot="hero-sync-error"]')).not.toBeNull(),
    );
    expect(screen.getByText("Could not read the project's files")).toBeInTheDocument();
    expect(screen.queryByText("Reading the project's files…")).not.toBeInTheDocument();
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

    // Per-(collection, locale) rows: missing / content memory-exact / stored
    // drafts / AI-remainder.
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("docs");
    expect(dialog).toHaveTextContent("12");
    expect(dialog).toHaveTextContent("Memory exact");
    expect(dialog).toHaveTextContent("Stored drafts");
    expect(dialog).toHaveTextContent("AI work");
    // Total token estimate + the disclosed heuristic note.
    expect(dialog).toHaveTextContent("960");
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
    expect(screen.getByText("Every block is translated.")).toBeInTheDocument();
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
