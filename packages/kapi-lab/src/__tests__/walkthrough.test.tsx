// @vitest-environment jsdom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LAB_SCENARIOS } from "../labScenarios";
import WalkthroughCard from "../WalkthroughCard";

describe("scenario walkthroughs", () => {
  const withWalkthrough = LAB_SCENARIOS.filter((s) => s.walkthrough);

  it("exist for every teaching scenario (free play excluded)", () => {
    const ids = withWalkthrough.map((s) => s.id);
    expect(ids).toEqual(
      expect.arrayContaining([
        "redaction",
        "redaction-ner",
        "segmentation",
        "annotations",
        "pseudo",
      ]),
    );
    expect(LAB_SCENARIOS.find((s) => s.id === "build-your-own")?.walkthrough).toBeUndefined();
  });

  it("only reference nodes the scenario's flow actually has", () => {
    for (const s of withWalkthrough) {
      for (const step of s.walkthrough!) {
        if (!step.select) continue;
        if (step.select.startsWith("tool-")) {
          const idx = Number(step.select.slice("tool-".length));
          expect(
            idx,
            `${s.id}: ${step.select} out of range (${s.steps.length} steps)`,
          ).toBeLessThan(s.steps.length);
          expect(idx).toBeGreaterThanOrEqual(0);
        } else {
          expect(["endpoint-source", "endpoint-sink"]).toContain(step.select);
        }
      }
    }
  });

  it("run before any step that inspects run results", () => {
    // An "inspect" focus on a tool node reads the trace; the walkthrough must
    // have offered Run on an earlier step.
    for (const s of withWalkthrough) {
      const runAt = s.walkthrough!.findIndex((st) => st.run);
      s.walkthrough!.forEach((st, i) => {
        if (st.select?.startsWith("tool-") && (st.mode ?? "inspect") === "inspect") {
          expect(runAt, `${s.id}: step ${i} inspects before any run step`).toBeGreaterThanOrEqual(
            0,
          );
          expect(runAt).toBeLessThan(i);
        }
      });
    }
  });
});

describe("WalkthroughCard", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    act(() => {
      root = createRoot(container);
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  const STEPS = [
    { prose: "first step", select: "endpoint-source" },
    { prose: "now run", run: true, select: null },
    { prose: "look at the result", select: "tool-0" },
  ];

  const buttons = () => Array.from(container.querySelectorAll("button"));

  it("shows the active step's prose and position", () => {
    act(() => {
      root.render(
        createElement(WalkthroughCard, {
          steps: STEPS,
          index: 0,
          onIndexChange: vi.fn(),
          onRun: vi.fn(),
        }),
      );
    });
    expect(container.textContent).toContain("first step");
    expect(container.textContent).toContain("1 / 3");
  });

  it("Next advances, Back is disabled on the first step", () => {
    const onIndexChange = vi.fn();
    act(() => {
      root.render(
        createElement(WalkthroughCard, { steps: STEPS, index: 0, onIndexChange, onRun: vi.fn() }),
      );
    });
    const back = buttons().find((b) => b.textContent?.includes("Back"))!;
    expect(back.disabled).toBe(true);
    const next = buttons().find((b) => b.textContent?.includes("Next"))!;
    act(() => next.click());
    expect(onIndexChange).toHaveBeenCalledWith(1);
  });

  it("offers Run (not Next) on a run step", () => {
    const onRun = vi.fn();
    act(() => {
      root.render(
        createElement(WalkthroughCard, { steps: STEPS, index: 1, onIndexChange: vi.fn(), onRun }),
      );
    });
    expect(buttons().some((b) => b.textContent?.includes("Next"))).toBe(false);
    const run = buttons().find((b) => b.textContent?.includes("Run the flow"))!;
    act(() => run.click());
    expect(onRun).toHaveBeenCalledTimes(1);
  });

  it("marks the end of the walkthrough", () => {
    act(() => {
      root.render(
        createElement(WalkthroughCard, {
          steps: STEPS,
          index: 2,
          onIndexChange: vi.fn(),
          onRun: vi.fn(),
        }),
      );
    });
    expect(container.textContent).toContain("End of the walkthrough");
    expect(buttons().some((b) => b.textContent?.includes("Next"))).toBe(false);
  });
});

describe("specFromTrace", () => {
  it("reconstructs the flow from a recorded trace's tool nodes, in order", async () => {
    const { specFromTrace } = await import("../traceImport");
    const trace = {
      nodes: [
        { id: "reader", type: "reader", name: "json", label: "JSON Reader" },
        { id: "pseudo", type: "tool", name: "pseudo-translate", label: "Pseudo-Translate" },
        { id: "qa", type: "bridge-tool", name: "qa" },
        { id: "writer", type: "writer", name: "json", label: "JSON Writer" },
      ],
      events: [],
      parts: {},
      durationUs: 0,
    };
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const spec = specFromTrace(trace as any);
    expect(spec.steps).toEqual([
      { tool: "pseudo-translate", label: "Pseudo-Translate" },
      { tool: "qa" },
    ]);
  });
});
