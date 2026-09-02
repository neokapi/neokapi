import { render, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ConvergeRunView } from "../components/ConvergeRunView";
import type { RunEvent } from "../context/JobFeedContext";
import type { ConvergeEvent } from "../types/api";

/** Wrap a typed convergence event as the RunEvent the job feed stores. */
function ce(event: ConvergeEvent): RunEvent {
  return { type: "converge_event", flow_id: "up", converge_event: event };
}

/** A live typed stream: two locales, one done, one mid-flight, pass settled. */
const liveEvents: RunEvent[] = [
  { type: "state", flow_id: "up", message: "running" },
  ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["fr-FR", "de-DE"] }),
  ce({ type: "locale_start", pass: 1, locale: "fr-FR", units: 20 }),
  ce({ type: "locale_done", pass: 1, locale: "fr-FR", units: 20, done: 20, viaTM: 12, viaAI: 8 }),
  ce({ type: "locale_start", pass: 1, locale: "de-DE", units: 20 }),
  ce({ type: "unit_progress", pass: 1, locale: "de-DE", done: 9, viaTM: 4, viaAI: 5 }),
  ce({
    type: "pass_done",
    pass: 1,
    produced: 29,
    producedDelta: 29,
    failingChecks: 2,
    pending: ["de-DE"],
  }),
];

const completeEvent: RunEvent = {
  type: "complete",
  flow_id: "up",
  converge_result: {
    flow: "up",
    passes: 2,
    converged: false,
    locales: [
      { locale: "fr-FR", shippable: true },
      { locale: "de-DE", shippable: false, parked: true },
    ],
    parkedScopes: [{ locale: "de-DE", collection: "docs" }],
  },
};

describe("ConvergeRunView — live locale rows", () => {
  it("renders one row per locale with state, progress, and content memory/AI split", () => {
    const { container } = render(<ConvergeRunView events={liveEvents} running />);

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
    render(<ConvergeRunView events={liveEvents} running />);
    expect(screen.getByText(/produced 29/)).toBeInTheDocument();
    expect(screen.getByText("checks failing 2")).toBeInTheDocument();
    expect(screen.getByText("still pending")).toBeInTheDocument();
    // Raw flow logs are not rendered — the view speaks passes and locales.
    expect(screen.queryByText("running")).not.toBeInTheDocument();
  });

  it("shows the deriving affordance before the first pass arrives", () => {
    render(
      <ConvergeRunView events={[{ type: "state", flow_id: "up", message: "running" }]} running />,
    );
    expect(screen.getByText("Deriving state and running the first pass…")).toBeInTheDocument();
  });

  it("deep-links a parked scope to the Review page filtered to it", async () => {
    const onOpenReview = vi.fn();
    render(<ConvergeRunView events={[...liveEvents, completeEvent]} onOpenReview={onOpenReview} />);
    expect(screen.getByText(/1 scope\(s\) parked/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Review de-DE · docs" }));
    expect(onOpenReview).toHaveBeenCalledWith({ collection: "docs", locale: "de-DE" });
  });

  it("renders the converged outcome with materialized count", () => {
    const converged: RunEvent = {
      type: "complete",
      flow_id: "up",
      converge_result: {
        flow: "up",
        passes: 1,
        converged: true,
        locales: [{ locale: "fr-FR", shippable: true }],
        materializedFiles: 4,
      },
    };
    render(<ConvergeRunView events={[...liveEvents, converged]} />);
    expect(
      screen.getByText("Up to date in 1 pass(es). Every gated scope is shippable."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Materialized 4 localized file(s) from the project store."),
    ).toBeInTheDocument();
  });

  it("renders the cancelled affordance", () => {
    render(<ConvergeRunView events={liveEvents} canceled />);
    expect(screen.getByText(/Cancelled\. The run stopped/)).toBeInTheDocument();
  });
});
