// @vitest-environment jsdom
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ConvergenceRunView } from "../ConvergenceRunView";
import type { ConvergenceRunModel } from "../convergence-model";

/** A live model: fr-FR done, de-DE mid-flight, pass settled. */
const liveModel: ConvergenceRunModel = {
  live: true,
  passes: [
    {
      pass: 1,
      maxPasses: 6,
      settled: true,
      produced: 29,
      producedDelta: 29,
      failingChecks: 2,
      pending: ["de-DE"],
      rows: [
        { locale: "fr-FR", units: 20, done: 20, viaMemory: 12, viaAI: 8, state: "done" },
        { locale: "de-DE", units: 20, done: 9, viaMemory: 4, viaAI: 5, state: "running" },
      ],
    },
  ],
};

describe("ConvergenceRunView — live locale rows", () => {
  it("renders one row per locale with state, progress, and content memory/AI split", () => {
    const { container } = render(<ConvergenceRunView model={liveModel} running />);

    expect(screen.getByText("Pass 1")).toBeInTheDocument();
    expect(screen.getByText("of 6")).toBeInTheDocument();

    const rows = container.querySelectorAll('[data-slot="converge-locale-row"]');
    expect(rows).toHaveLength(2);

    const fr = container.querySelector('[data-locale="fr-FR"]')!;
    expect(fr.getAttribute("data-state")).toBe("done");
    expect(within(fr as HTMLElement).getByText("20/20")).toBeInTheDocument();
    expect(within(fr as HTMLElement).getByText("Memory 12 · AI 8")).toBeInTheDocument();

    const de = container.querySelector('[data-locale="de-DE"]')!;
    expect(de.getAttribute("data-state")).toBe("running");
    expect(within(de as HTMLElement).getByText("9/20")).toBeInTheDocument();
  });

  it("renders the settled pass summary (produced, failing checks, pending)", () => {
    render(<ConvergenceRunView model={liveModel} running />);
    expect(screen.getByText(/produced 29/)).toBeInTheDocument();
    expect(screen.getByText("checks failing 2")).toBeInTheDocument();
    expect(screen.getByText("still pending")).toBeInTheDocument();
  });

  it("shows the deriving affordance before the first pass arrives", () => {
    render(<ConvergenceRunView model={{ passes: [] }} running />);
    expect(screen.getByText("Deriving state and running the first pass…")).toBeInTheDocument();
  });
});

describe("ConvergenceRunView — structured outcome (kapi-desktop)", () => {
  it("deep-links a parked scope to review filtered to it", async () => {
    const onOpenReview = vi.fn();
    render(
      <ConvergenceRunView
        model={liveModel}
        result={{
          converged: false,
          passes: 2,
          parkedScopes: [{ locale: "de-DE", collection: "docs" }],
        }}
        onOpenReview={onOpenReview}
      />,
    );
    expect(screen.getByText(/1 scope\(s\) parked/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Review de-DE · docs" }));
    expect(onOpenReview).toHaveBeenCalledWith({ collection: "docs", locale: "de-DE" });
  });

  it("renders the converged outcome with materialized count", () => {
    render(
      <ConvergenceRunView
        model={liveModel}
        result={{ converged: true, passes: 1, materializedFiles: 4 }}
      />,
    );
    expect(
      screen.getByText("Up to date in 1 pass(es). Every gated scope is shippable."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Materialized 4 localized file(s) from the project store."),
    ).toBeInTheDocument();
  });

  it("renders the cancelled affordance", () => {
    render(<ConvergenceRunView model={liveModel} canceled />);
    expect(screen.getByText(/Cancelled\. The run stopped/)).toBeInTheDocument();
  });
});

describe("ConvergenceRunView — header + logs + done footer (bowrain)", () => {
  it("renders the run header, per-locale ship badge, logs, and done footer", () => {
    const model: ConvergenceRunModel = {
      done: true,
      finalState: "parked",
      materializedFiles: 3,
      logs: ["Extracted 128 blocks from 4 files"],
      passes: [
        {
          pass: 1,
          maxPasses: 3,
          settled: true,
          failingChecks: 1,
          rows: [
            {
              locale: "fr-FR",
              units: 12,
              done: 12,
              viaMemory: 8,
              viaAI: 4,
              state: "done",
              localeState: "shippable",
            },
          ],
        },
      ],
    };
    render(<ConvergenceRunView model={model} run={{ id: "run-8f2a1c0e", trigger: "manual" }} />);
    expect(screen.getByText("Manual run")).toBeInTheDocument();
    expect(screen.getByText("Run run-8f2a")).toBeInTheDocument();
    expect(screen.getByText("shippable")).toBeInTheDocument();
    expect(screen.getByText("Log (1)")).toBeInTheDocument();
    expect(screen.getByText("3 files written")).toBeInTheDocument();
  });
});

// A run held on source produced nothing, in any language, and said so nowhere:
// the event carried blockedOnSource and the desktop dropped it, so the one
// number explaining a stalled run was in memory and off the screen.
describe("ConvergenceRunView — source hold", () => {
  const model: ConvergenceRunModel = { live: true, passes: [] };

  it("names the count and points at the source lane", () => {
    render(
      <ConvergenceRunView
        model={model}
        result={{
          converged: false,
          passes: 1,
          blockedOnSource: 12,
          sourceGate: "approved",
          stallReason: "source_not_ready",
        }}
      />,
    );
    const hold = document.querySelector('[data-slot="convergence-source-hold"]');
    expect(hold).not.toBeNull();
    expect(hold!.textContent).toContain("12 segments need source review");
    expect(hold!.textContent).toContain("approved");
    expect(hold!.textContent).toContain("source lane");
  });

  // A run that produced work but held some of it is not a stalled run, and must
  // not read like one.
  it("does not claim nothing was produced when the run only held part", () => {
    render(
      <ConvergenceRunView
        model={model}
        result={{ converged: false, passes: 1, blockedOnSource: 3 }}
      />,
    );
    const hold = document.querySelector('[data-slot="convergence-source-hold"]');
    expect(hold!.textContent).toContain("3 segments need source review");
    expect(hold!.textContent).not.toContain("Nothing was produced");
  });

  it("says nothing when the source is settled", () => {
    render(<ConvergenceRunView model={model} result={{ converged: true, passes: 1 }} />);
    expect(document.querySelector('[data-slot="convergence-source-hold"]')).toBeNull();
  });
});
