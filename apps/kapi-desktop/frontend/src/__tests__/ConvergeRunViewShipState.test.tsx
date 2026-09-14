import { render, screen } from "./testUtils";
import { describe, it, expect } from "vitest";
import { ConvergeRunView } from "../components/ConvergeRunView";
import type { RunEvent } from "../context/JobFeedContext";

// The desktop adapter hands the run's per-locale ship states to the shared view,
// so a converged run over a project with no ship gates claims none.
describe("ConvergeRunView: a project with no ship gates", () => {
  it("reports up to date without claiming a gate", () => {
    const events: RunEvent[] = [
      {
        type: "converge_event",
        flow_id: "up",
        converge_event: { type: "pass_start", pass: 1, maxPasses: 3, pending: ["nb"] },
      },
      {
        type: "converge_event",
        flow_id: "up",
        converge_event: {
          type: "pass_done",
          pass: 1,
          produced: 1,
          producedDelta: 1,
          failingChecks: 0,
          pending: [],
        },
      },
      {
        type: "complete",
        flow_id: "up",
        converge_result: {
          flow: "up",
          passes: 1,
          converged: true,
          locales: [{ locale: "nb", shippable: true, shipState: "not_gated", gated: false }],
        },
      },
    ];
    render(<ConvergeRunView events={events} />);
    expect(
      screen.getByText("Up to date in 1 pass(es). No ship gates are declared."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Every gated scope is shippable/)).not.toBeInTheDocument();
  });
});
