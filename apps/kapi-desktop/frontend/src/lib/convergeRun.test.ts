import { describe, it, expect } from "vitest";
import { reduceConvergeRun } from "./convergeRun";
import type { RunEvent } from "../context/JobFeedContext";
import type { ConvergeEvent } from "../types/api";

/** Wrap a typed convergence event as the RunEvent the job feed stores. */
function ce(event: ConvergeEvent): RunEvent {
  return { type: "converge_event", flow_id: "up", converge_event: event };
}

describe("reduceConvergeRun (typed converge_event stream)", () => {
  it("builds a pass with one queued row per pending locale on pass_start", () => {
    const model = reduceConvergeRun([
      ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["fr-FR", "de-DE"] }),
    ]);
    expect(model.live).toBe(true);
    expect(model.passes).toHaveLength(1);
    expect(model.passes[0]).toMatchObject({ pass: 1, maxPasses: 6 });
    expect(model.passes[0].rows.map((r) => [r.locale, r.state])).toEqual([
      ["fr-FR", "queued"],
      ["de-DE", "queued"],
    ]);
  });

  it("advances a locale row through running → done with TM/AI split", () => {
    const model = reduceConvergeRun([
      ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["fr-FR"] }),
      ce({ type: "locale_start", pass: 1, locale: "fr-FR", units: 20 }),
      ce({ type: "unit_progress", pass: 1, locale: "fr-FR", done: 8, viaTM: 5, viaAI: 3 }),
      ce({
        type: "locale_done",
        pass: 1,
        locale: "fr-FR",
        units: 20,
        done: 20,
        viaTM: 12,
        viaAI: 8,
      }),
    ]);
    const row = model.passes[0].rows[0];
    expect(row).toMatchObject({
      locale: "fr-FR",
      state: "done",
      units: 20,
      done: 20,
      viaTM: 12,
      viaAI: 8,
    });
  });

  it("mid-flight unit_progress leaves the row running with partial counts", () => {
    const model = reduceConvergeRun([
      ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["fr-FR"] }),
      ce({ type: "locale_start", pass: 1, locale: "fr-FR", units: 20 }),
      ce({ type: "unit_progress", pass: 1, locale: "fr-FR", done: 8, viaTM: 5, viaAI: 3 }),
    ]);
    expect(model.passes[0].rows[0]).toMatchObject({ state: "running", done: 8, units: 20 });
  });

  it("records the pass summary on pass_done", () => {
    const model = reduceConvergeRun([
      ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["fr-FR", "de-DE"] }),
      ce({ type: "locale_done", pass: 1, locale: "fr-FR", units: 20, done: 20 }),
      ce({
        type: "pass_done",
        pass: 1,
        produced: 40,
        producedDelta: 40,
        failingChecks: 3,
        pending: ["de-DE"],
      }),
    ]);
    expect(model.passes[0]).toMatchObject({
      settled: true,
      produced: 40,
      producedDelta: 40,
      failingChecks: 3,
      pending: ["de-DE"],
    });
  });

  it("accumulates multiple passes and the run outcome", () => {
    const model = reduceConvergeRun([
      ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["de-DE"] }),
      ce({ type: "pass_done", pass: 1, produced: 40, producedDelta: 40, pending: ["de-DE"] }),
      ce({ type: "pass_start", pass: 2, maxPasses: 6, pending: ["de-DE"] }),
      ce({ type: "pass_done", pass: 2, produced: 43, producedDelta: 3, pending: [] }),
      ce({ type: "materialized", files: 12 }),
      ce({ type: "done", state: "converged" }),
    ]);
    expect(model.passes.map((p) => p.pass)).toEqual([1, 2]);
    expect(model.materializedFiles).toBe(12);
    expect(model.finalState).toBe("converged");
  });

  it("degrades to the converge_pass summary path when no typed events exist", () => {
    const events: RunEvent[] = [
      {
        type: "converge_pass",
        flow_id: "up",
        converge: {
          pass: 1,
          extractedBlocks: 128,
          produced: 40,
          producedDelta: 40,
          failingChecks: 2,
          pendingLocales: ["de-DE"],
        },
      },
    ];
    const model = reduceConvergeRun(events);
    expect(model.live).toBe(false);
    expect(model.passes[0]).toMatchObject({
      pass: 1,
      settled: true,
      produced: 40,
      rows: [],
      pending: ["de-DE"],
    });
  });

  it("prefers the typed stream and ignores converge_pass when both are present", () => {
    const events: RunEvent[] = [
      ce({ type: "pass_start", pass: 1, maxPasses: 6, pending: ["fr-FR"] }),
      ce({ type: "pass_done", pass: 1, produced: 10, producedDelta: 10, pending: [] }),
      {
        type: "converge_pass",
        flow_id: "up",
        converge: { pass: 1, produced: 999, producedDelta: 999 },
      },
    ];
    const model = reduceConvergeRun(events);
    expect(model.live).toBe(true);
    expect(model.passes).toHaveLength(1);
    expect(model.passes[0].produced).toBe(10);
  });
});
