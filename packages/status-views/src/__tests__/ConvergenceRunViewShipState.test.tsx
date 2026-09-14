// @vitest-environment jsdom
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ConvergenceRunView } from "../ConvergenceRunView";
import type { ConvergenceRunModel } from "../convergence-model";

// A converged run claims the gates only where a gate matches. Given the run's
// per-locale ship states, a project with no ship gates reads as such, and the
// languages no gate matches are named beside the gated verdict.

const settledModel: ConvergenceRunModel = {
  live: false,
  passes: [
    {
      pass: 1,
      maxPasses: 3,
      settled: true,
      produced: 20,
      producedDelta: 20,
      failingChecks: 0,
      pending: [],
      rows: [{ locale: "nb", units: 20, done: 20, viaMemory: 20, viaAI: 0, state: "done" }],
    },
  ],
};

describe("ConvergenceRunView: converged outcome and ship states", () => {
  it("says no ship gates are declared when no locale is gated", () => {
    render(
      <ConvergenceRunView
        model={settledModel}
        result={{
          converged: true,
          passes: 1,
          locales: [{ locale: "nb", shipState: "not_gated", gated: false }],
        }}
      />,
    );
    expect(
      screen.getByText("Up to date in 1 pass(es). No ship gates are declared."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Every gated scope is shippable/)).not.toBeInTheDocument();
  });

  it("names the languages no gate matches beside the gated verdict", () => {
    render(
      <ConvergenceRunView
        model={settledModel}
        result={{
          converged: true,
          passes: 1,
          locales: [
            { locale: "fr", shipState: "shippable", gated: true },
            { locale: "nb", shipState: "not_gated", gated: false },
          ],
        }}
      />,
    );
    expect(
      screen.getByText("Up to date in 1 pass(es). Every gated scope is shippable. Not gated: nb."),
    ).toBeInTheDocument();
  });
});
