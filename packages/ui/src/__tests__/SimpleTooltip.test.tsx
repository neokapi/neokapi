// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { SimpleTooltip, TooltipProvider } from "../components/ui/tooltip";

// Polyfill ResizeObserver for Radix popper in jsdom.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

const roots: Root[] = [];

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  roots.push(root);
  act(() => {
    root.render(el);
  });
  return container;
}

afterEach(() => {
  for (const root of roots.splice(0)) {
    act(() => root.unmount());
  }
  document.body.innerHTML = "";
});

function focusTrigger(trigger: HTMLElement) {
  act(() => {
    trigger.focus();
    // jsdom's focus() does not reliably fire the focusin React listens to.
    trigger.dispatchEvent(new FocusEvent("focusin", { bubbles: true }));
  });
}

function tooltipContent(): HTMLElement | null {
  return document.body.querySelector('[data-slot="tooltip-content"]');
}

describe("SimpleTooltip", () => {
  it("renders the trigger and shows the content on focus without an explicit provider", () => {
    const c = render(
      <SimpleTooltip content="Delete preset">
        <button type="button">trash</button>
      </SimpleTooltip>,
    );
    const trigger = c.querySelector("button")!;
    // asChild: the child element itself is the trigger — no extra wrapper button.
    expect(c.querySelectorAll("button")).toHaveLength(1);
    expect(trigger.dataset.slot).toBe("tooltip-trigger");
    expect(tooltipContent()).toBeNull();

    focusTrigger(trigger);
    expect(tooltipContent()?.textContent).toContain("Delete preset");
  });

  it("works under a single app-wide TooltipProvider", () => {
    const c = render(
      <TooltipProvider>
        <SimpleTooltip content="Undo (⌘Z)">
          <button type="button">undo</button>
        </SimpleTooltip>
      </TooltipProvider>,
    );
    focusTrigger(c.querySelector("button")!);
    expect(tooltipContent()?.textContent).toContain("Undo (⌘Z)");
  });

  it("renders the trigger unchanged when content is falsy", () => {
    for (const content of [undefined, null, "", false] as const) {
      const c = render(
        <SimpleTooltip content={content}>
          <button type="button">plain</button>
        </SimpleTooltip>,
      );
      const trigger = c.querySelector("button")!;
      expect(trigger.dataset.slot).toBeUndefined();
      focusTrigger(trigger);
      expect(tooltipContent()).toBeNull();
    }
  });

  it("keeps the child's own props working through asChild", () => {
    let clicks = 0;
    const c = render(
      <SimpleTooltip content="Clear completed">
        <button type="button" aria-label="Clear completed" onClick={() => clicks++}>
          ×
        </button>
      </SimpleTooltip>,
    );
    const trigger = c.querySelector("button")!;
    expect(trigger.getAttribute("aria-label")).toBe("Clear completed");
    act(() => {
      trigger.click();
    });
    expect(clicks).toBe(1);
  });
});
