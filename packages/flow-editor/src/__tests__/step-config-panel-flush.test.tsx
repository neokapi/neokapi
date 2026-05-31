// @vitest-environment jsdom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { StepConfigPanel } from "../FlowEditor";
import type { ComponentSchema } from "../types";

/**
 * Component-level proof of finding #51: the StepConfigPanel debounces config
 * edits (300ms) and must FLUSH the last pending edit when it unmounts (panel
 * closed or selection switched) instead of dropping it.
 *
 * The repo's UI tests drive React via react-dom/client + act (no RTL), so we
 * follow that convention here.
 */

const SCHEMA: ComponentSchema = {
  title: "AI Translate",
  type: "object",
  properties: {
    targetLang: { type: "string", default: "" },
  },
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.useFakeTimers();
  container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    root = createRoot(container);
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.useRealTimers();
});

function renderPanel(onConfigChange: (c: Record<string, unknown>) => void) {
  act(() => {
    root.render(
      createElement(StepConfigPanel, {
        step: { tool: "ai-translate" },
        toolInfo: { name: "ai-translate", description: "", category: "translate" },
        schema: SCHEMA,
        doc: null,
        config: {},
        isSourceTransformStage: false,
        onConfigChange,
        onStageToggle: () => {},
        onClose: () => {},
      }),
    );
  });
}

// Write through React's native value setter so the controlled input fires a
// change event the SchemaForm responds to.
function typeInput(el: HTMLInputElement, value: string): void {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  act(() => {
    setter?.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

describe("StepConfigPanel debounce flush on unmount (#51)", () => {
  it("flushes the last sub-300ms edit when the panel unmounts", () => {
    const onConfigChange = vi.fn<(c: Record<string, unknown>) => void>();
    renderPanel(onConfigChange);

    const input = container.querySelector("input") as HTMLInputElement;
    expect(input).toBeTruthy();

    typeInput(input, "fr");
    // The edit is still within the debounce window — not yet committed.
    act(() => vi.advanceTimersByTime(100));
    expect(onConfigChange).not.toHaveBeenCalled();

    // Unmount before the 300ms timer fires; the pending edit must flush.
    act(() => root.unmount());
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    expect(onConfigChange.mock.calls[0][0]).toMatchObject({ targetLang: "fr" });

    // Re-create a root so afterEach's unmount is a harmless no-op.
    act(() => {
      root = createRoot(container);
    });
  });

  it("commits normally after the debounce delay (no double emit on later unmount)", () => {
    const onConfigChange = vi.fn<(c: Record<string, unknown>) => void>();
    renderPanel(onConfigChange);

    const input = container.querySelector("input") as HTMLInputElement;
    typeInput(input, "de");
    act(() => vi.advanceTimersByTime(300));
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    expect(onConfigChange.mock.calls[0][0]).toMatchObject({ targetLang: "de" });

    // Unmounting afterwards must not re-emit (nothing pending).
    act(() => root.unmount());
    expect(onConfigChange).toHaveBeenCalledTimes(1);

    act(() => {
      root = createRoot(container);
    });
  });
});
