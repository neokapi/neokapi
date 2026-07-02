import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ConvergeRunView } from "../components/ConvergeRunView";
import type { RunEvent } from "../context/JobFeedContext";

const passEvents: RunEvent[] = [
  { type: "state", flow_id: "translate", message: "running" },
  {
    type: "converge_pass",
    flow_id: "translate",
    converge: {
      pass: 1,
      extractedFiles: 2,
      extractedBlocks: 128,
      produced: 40,
      producedDelta: 40,
      failingChecks: 3,
      pendingLocales: ["de-DE"],
    },
  },
  {
    type: "converge_pass",
    flow_id: "translate",
    converge: {
      pass: 2,
      produced: 43,
      producedDelta: 3,
      failingChecks: 0,
      pendingLocales: ["de-DE"],
    },
  },
];

const completeEvent: RunEvent = {
  type: "complete",
  flow_id: "translate",
  converge_result: {
    flow: "translate",
    passes: 2,
    converged: false,
    locales: [
      { locale: "fr-FR", shippable: true },
      { locale: "de-DE", shippable: false, parked: true },
    ],
    parkedScopes: [{ locale: "de-DE", collection: "docs" }],
  },
};

describe("ConvergeRunView", () => {
  it("renders one row per pass with extracted/produced/failing counts", () => {
    render(<ConvergeRunView events={passEvents} running />);
    expect(screen.getByText("Pass 1")).toBeInTheDocument();
    expect(screen.getByText("extracted 128")).toBeInTheDocument();
    expect(screen.getByText(/produced 40/)).toBeInTheDocument();
    expect(screen.getByText("checks failing 3")).toBeInTheDocument();
    expect(screen.getByText("Pass 2")).toBeInTheDocument();
    expect(screen.getByText(/produced 43/)).toBeInTheDocument();
    // Raw flow logs are not rendered — the view speaks passes.
    expect(screen.queryByText("running")).not.toBeInTheDocument();
  });

  it("deep-links a parked scope to the Review page filtered to it", async () => {
    const onOpenReview = vi.fn();
    render(<ConvergeRunView events={[...passEvents, completeEvent]} onOpenReview={onOpenReview} />);
    expect(screen.getByText(/1 scope\(s\) parked/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Review de-DE · docs" }));
    expect(onOpenReview).toHaveBeenCalledWith({ collection: "docs", locale: "de-DE" });
  });

  it("renders the converged outcome", () => {
    const converged: RunEvent = {
      type: "complete",
      flow_id: "translate",
      converge_result: {
        flow: "translate",
        passes: 1,
        converged: true,
        locales: [{ locale: "fr-FR", shippable: true }],
        materializedFiles: 4,
      },
    };
    render(<ConvergeRunView events={[passEvents[1], converged]} />);
    expect(
      screen.getByText("Converged in 1 pass(es) — every gated scope is shippable."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Materialized 4 localized file(s) from the project store."),
    ).toBeInTheDocument();
  });
});
