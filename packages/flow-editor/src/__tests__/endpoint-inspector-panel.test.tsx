// @vitest-environment jsdom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { EndpointInspectorPanel } from "../EndpointInspectorPanel";

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

describe("EndpointInspectorPanel", () => {
  it("renders the source chrome around host content", () => {
    act(() => {
      root.render(
        createElement(
          EndpointInspectorPanel,
          { role: "source", onClose: vi.fn() },
          createElement("div", null, "host body"),
        ),
      );
    });
    expect(container.textContent).toContain("Source");
    expect(container.textContent).toContain("What enters the flow");
    expect(container.textContent).toContain("host body");
  });

  it("renders the sink chrome", () => {
    act(() => {
      root.render(createElement(EndpointInspectorPanel, { role: "sink", onClose: vi.fn() }, "out"));
    });
    expect(container.textContent).toContain("Sink");
    expect(container.textContent).toContain("What the flow wrote");
  });

  it("fires onClose from the Close button", () => {
    const onClose = vi.fn();
    act(() => {
      root.render(createElement(EndpointInspectorPanel, { role: "source", onClose }, "x"));
    });
    const close = Array.from(container.querySelectorAll("button")).find(
      (b) => b.textContent === "Close",
    );
    expect(close).toBeDefined();
    act(() => close!.click());
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
