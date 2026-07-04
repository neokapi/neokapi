import { describe, it, expect } from "vitest";
import type { ConvergenceEvent } from "../types/api";
import { reduceRun, applyEvent, emptyRunModel } from "./runReducer";

/** A representative two-locale, single-pass converging run. */
function convergingRun(): ConvergenceEvent[] {
  return [
    { type: "pass_start", pass: 1, maxPasses: 3, pending: ["fr-FR", "de-DE"] },
    { type: "locale_start", pass: 1, locale: "fr-FR", units: 10 },
    { type: "unit_progress", pass: 1, locale: "fr-FR", done: 4, viaTM: 3, viaAI: 1 },
    { type: "locale_start", pass: 1, locale: "de-DE", units: 6 },
    { type: "unit_progress", pass: 1, locale: "fr-FR", done: 10, viaTM: 6, viaAI: 4 },
    {
      type: "locale_done",
      pass: 1,
      locale: "fr-FR",
      units: 10,
      done: 10,
      viaTM: 6,
      viaAI: 4,
      state: "shippable",
    },
    { type: "unit_progress", pass: 1, locale: "de-DE", done: 6, viaTM: 2, viaAI: 4 },
    {
      type: "locale_done",
      pass: 1,
      locale: "de-DE",
      units: 6,
      done: 6,
      viaTM: 2,
      viaAI: 4,
      state: "shippable",
    },
    { type: "pass_done", pass: 1, produced: 16, producedDelta: 16, failingChecks: 0, pending: [] },
    { type: "materialized", files: 2 },
    { type: "done", state: "converged" },
  ];
}

describe("reduceRun", () => {
  it("folds a converging run into one settled pass with per-locale rows", () => {
    const m = reduceRun(convergingRun());

    expect(m.done).toBe(true);
    expect(m.finalState).toBe("converged");
    expect(m.materializedFiles).toBe(2);
    expect(m.passes).toHaveLength(1);

    const pass = m.passes[0];
    expect(pass.pass).toBe(1);
    expect(pass.maxPasses).toBe(3);
    expect(pass.settled).toBe(true);
    expect(pass.produced).toBe(16);
    expect(pass.failingChecks).toBe(0);

    const fr = pass.rows.find((r) => r.locale === "fr-FR")!;
    expect(fr.state).toBe("done");
    expect(fr.localeState).toBe("shippable");
    expect(fr.units).toBe(10);
    expect(fr.done).toBe(10);
    expect(fr.viaTM).toBe(6);
    expect(fr.viaAI).toBe(4);
  });

  it("tracks the queued -> running -> done lifecycle from pass_start", () => {
    const events = convergingRun();
    // After only pass_start, both locales are queued at zero.
    const queued = reduceRun(events.slice(0, 1));
    expect(queued.passes[0].rows.map((r) => r.state)).toEqual(["queued", "queued"]);

    // After the first locale_start, fr-FR is running with its denominator set.
    const started = reduceRun(events.slice(0, 2));
    const fr = started.passes[0].rows.find((r) => r.locale === "fr-FR")!;
    expect(fr.state).toBe("running");
    expect(fr.units).toBe(10);
    expect(fr.done).toBe(0);
  });

  it("marks a run parked and carries failing-check counts", () => {
    const m = reduceRun([
      { type: "pass_start", pass: 1, maxPasses: 2, pending: ["ja-JP"] },
      { type: "locale_start", pass: 1, locale: "ja-JP", units: 8 },
      {
        type: "locale_done",
        pass: 1,
        locale: "ja-JP",
        units: 8,
        done: 5,
        viaAI: 5,
        state: "parked",
      },
      {
        type: "pass_done",
        pass: 1,
        produced: 5,
        producedDelta: 5,
        failingChecks: 3,
        pending: ["ja-JP"],
      },
      { type: "done", state: "parked" },
    ]);

    expect(m.finalState).toBe("parked");
    expect(m.passes[0].failingChecks).toBe(3);
    expect(m.passes[0].pending).toEqual(["ja-JP"]);
    expect(m.passes[0].rows[0].localeState).toBe("parked");
    expect(m.passes[0].rows[0].done).toBe(5);
  });

  it("accumulates multiple passes and resets rows per pass", () => {
    const m = reduceRun([
      { type: "pass_start", pass: 1, maxPasses: 3, pending: ["fr-FR"] },
      { type: "locale_done", pass: 1, locale: "fr-FR", units: 10, done: 7, state: "pending" },
      {
        type: "pass_done",
        pass: 1,
        produced: 7,
        producedDelta: 7,
        failingChecks: 1,
        pending: ["fr-FR"],
      },
      { type: "pass_start", pass: 2, maxPasses: 3, pending: ["fr-FR"] },
      { type: "locale_done", pass: 2, locale: "fr-FR", units: 10, done: 10, state: "shippable" },
      { type: "pass_done", pass: 2, produced: 10, producedDelta: 3, failingChecks: 0, pending: [] },
      { type: "done", state: "converged" },
    ]);

    expect(m.passes).toHaveLength(2);
    // Each pass carries its own row for the locale — pass 1 short, pass 2 full.
    expect(m.passes[0].rows[0].done).toBe(7);
    expect(m.passes[0].rows[0].localeState).toBe("pending");
    expect(m.passes[1].rows[0].done).toBe(10);
    expect(m.passes[1].rows[0].localeState).toBe("shippable");
  });

  it("routes a locale event to the pass named by its pass stamp, not just the latest", () => {
    // A late unit_progress stamped pass 1 must land in pass 1 even after pass 2
    // has opened (out-of-order delivery on reconnect).
    const m = reduceRun([
      { type: "pass_start", pass: 1, maxPasses: 3, pending: ["fr-FR"] },
      { type: "pass_start", pass: 2, maxPasses: 3, pending: ["fr-FR"] },
      { type: "unit_progress", pass: 1, locale: "fr-FR", done: 3, viaTM: 3 },
    ]);
    expect(m.passes[0].rows[0].done).toBe(3);
    expect(m.passes[1].rows[0].done).toBe(0);
  });

  it("collects log lines and pre-pass extraction notes", () => {
    const m = reduceRun([
      {
        type: "pass_start",
        pass: 1,
        maxPasses: 1,
        pending: ["fr-FR"],
        extractedFiles: 2,
        extractedBlocks: 40,
      },
      { type: "log", message: "connector transport: pushed 2 files" },
      { type: "done", state: "converged" },
    ]);
    expect(m.logs).toContain("Extracted 40 blocks from 2 files");
    expect(m.logs).toContain("connector transport: pushed 2 files");
  });

  it("applyEvent folds incrementally the same way reduceRun folds in bulk", () => {
    const events = convergingRun();
    const incremental = events.reduce(applyEvent, emptyRunModel());
    expect(incremental).toEqual(reduceRun(events));
  });
});
