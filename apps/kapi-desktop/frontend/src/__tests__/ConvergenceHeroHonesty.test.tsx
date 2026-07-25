import { render, screen, waitFor } from "./testUtils";
import { describe, it, expect, vi, afterEach } from "vitest";
import { ConvergenceHero } from "../components/ConvergenceHero";
import { api } from "../hooks/useApi";
import type { ConvergePlan, ConvergenceReport, RunError } from "../types/api";

// The hero must never claim convergence it cannot substantiate. Three ways it
// used to:
//
//   1. `gated.every(...)` over an empty array is vacuously true, so a recipe
//      with no ship gate — or a report that never arrived — asserted "all gates
//      green" without a gate having been evaluated.
//   2. A rejected derivation was folded to `null`, and `loaded` came from
//      `isSuccess` on a query fn that never throws — so a backend error like
//      "compute coverage: …" rendered as "Up to date · all gates green".
//   3. A failed catch-up run left no trace in the file-derived numbers, so a run
//      that died on its first locale looked identical to a converged project.

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

const gatedGreenReport: ConvergenceReport = {
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

/** The same converged state, but the recipe declares no ship gate. */
const ungatedReport: ConvergenceReport = {
  locales: [
    {
      locale: "fr-FR",
      collection: "docs",
      total: 40,
      pct: { translated: 100 },
      gated: false,
      shippable: true,
    },
  ],
  review: [],
};

const ollamaDown: RunError = {
  kind: "ollama-unreachable",
  headline: "Ollama isn't running",
  remediation: "Start Ollama, then run again. Kapi expects it at http://localhost:11434.",
  actions: [
    { kind: "command", label: "Start Ollama", command: "ollama serve" },
    { kind: "open-url", label: "Install Ollama", url: "https://ollama.com" },
  ],
  locale: "nb",
  file: "notes/release-notes.md",
  provider: "ollama",
  raw: "converge nb: flow execution error: notes/release-notes.md: execute flow: tool translate: translate: ollama: cannot reach Ollama at http://localhost:11434 — is it running?",
};

describe("ConvergenceHero honesty", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("claims green gates only when gates exist", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={gatedGreenReport}
        plan={convergedPlan}
        lastRunError={null}
      />,
    );
    expect(screen.getByText("Up to date · all gates green")).toBeInTheDocument();
  });

  it("says only 'Up to date' when the recipe declares no ship gates", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={ungatedReport}
        plan={convergedPlan}
        lastRunError={null}
      />,
    );
    // Still converged — but it must not assert a gate verdict it never made.
    expect(screen.getByText("Up to date")).toBeInTheDocument();
    expect(screen.queryByText("Up to date · all gates green")).not.toBeInTheDocument();
  });

  it("does not report convergence when the last run failed", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={gatedGreenReport}
        plan={convergedPlan}
        lastRunError={ollamaDown}
      />,
    );
    // The file-derived numbers say "done"; the failed run says otherwise, and
    // the failure wins.
    expect(screen.queryByText("Up to date · all gates green")).not.toBeInTheDocument();
    expect(screen.getByText(/last run failed/)).toBeInTheDocument();
    // Re-running is possible again (the button must not be disabled by a stale
    // "converged" reading).
    expect(screen.getByRole("button", { name: "Bring the project up to date" })).toBeEnabled();
  });

  it("shows the failure as a headline + remediation action, not a wrapped chain", async () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={gatedGreenReport}
        plan={convergedPlan}
        lastRunError={ollamaDown}
      />,
    );
    await waitFor(() =>
      expect(document.querySelector('[data-slot="run-error-notice"]')).not.toBeNull(),
    );
    // Human headline…
    expect(screen.getByText("Ollama isn't running")).toBeInTheDocument();
    // …the remedy as an action carrying the concrete command…
    expect(screen.getByText("ollama serve")).toBeInTheDocument();
    expect(screen.getByText("Install Ollama")).toBeInTheDocument();
    // …the affected scope as metadata…
    expect(screen.getByText("nb")).toBeInTheDocument();
    expect(screen.getByText("notes/release-notes.md")).toBeInTheDocument();
    // …and the raw chain only behind Details.
    expect(screen.queryByText(/flow execution error/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Details/ })).toBeInTheDocument();
  });

  it("treats a cancelled run as not-a-failure", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={gatedGreenReport}
        plan={convergedPlan}
        lastRunError={{ kind: "canceled", headline: "Run canceled", raw: "context canceled" }}
      />,
    );
    // Cancelling is a user decision, not a broken state: the derived numbers
    // stand on their own.
    expect(screen.getByText("Up to date · all gates green")).toBeInTheDocument();
  });

  it("reports an unknown state when a derivation fails, never convergence", async () => {
    vi.spyOn(api, "getConvergence").mockRejectedValue(
      new Error("compute coverage: open project block store: permission denied"),
    );
    vi.spyOn(api, "getConvergePlan").mockRejectedValue(
      new Error("compute coverage: open project block store: permission denied"),
    );
    vi.spyOn(api, "getProjectServer").mockResolvedValue(null as never);
    vi.spyOn(api, "getLastRunError").mockResolvedValue(null);

    render(<ConvergenceHero tabID="t1" onBringUpToDate={vi.fn()} />);

    // The summary line reports the unknown, and the panel carries the cause.
    await waitFor(() =>
      expect(document.querySelector('[data-slot="hero-drift-summary"]')?.textContent).toBe(
        "Could not determine project state",
      ),
    );
    expect(document.querySelector('[data-slot="hero-derivation-error"]')).not.toBeNull();
    expect(document.querySelector('[data-slot="hero-up-to-date"]')).toBeNull();
    expect(screen.queryByText("Up to date · all gates green")).not.toBeInTheDocument();
  });
});
