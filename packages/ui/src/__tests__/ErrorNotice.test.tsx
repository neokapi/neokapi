// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { createElement, act } from "react";
import { createRoot } from "react-dom/client";
import { ErrorNotice } from "../components/ui/error-notice";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

const envelope =
  '{"message":"no default flow configured — add a flow or pick one to run","cause":{},"kind":"RuntimeError"}';

describe("ErrorNotice", () => {
  it("renders the friendly line from a Wails envelope, not the raw JSON", () => {
    const c = render(createElement(ErrorNotice, { error: envelope }));
    const title = c.querySelector('[data-slot="error-notice-title"]');
    expect(title?.textContent).toBe(
      "no default flow configured — add a flow or pick one to run",
    );
    // The raw JSON must not be visible before disclosure.
    expect(c.textContent).not.toContain('"kind"');
  });

  it("reveals syntax-highlighted raw JSON behind the Details disclosure", () => {
    const c = render(createElement(ErrorNotice, { error: envelope }));
    const toggle = c.querySelector<HTMLButtonElement>('[data-slot="error-notice-toggle"]');
    expect(toggle).not.toBeNull();
    expect(toggle!.getAttribute("aria-expanded")).toBe("false");

    act(() => {
      toggle!.click();
    });

    expect(toggle!.getAttribute("aria-expanded")).toBe("true");
    const raw = c.querySelector('[data-slot="error-notice-raw"]');
    expect(raw).not.toBeNull();
    expect(raw!.textContent).toContain('"kind"');
    expect(raw!.textContent).toContain("RuntimeError");
    // Tokenised output: the key tokens are wrapped in styled spans.
    expect(raw!.querySelectorAll("span").length).toBeGreaterThan(0);

    // Collapses again.
    act(() => {
      toggle!.click();
    });
    expect(c.querySelector('[data-slot="error-notice-raw"]')).toBeNull();
  });

  it("offers no disclosure for a plain string error", () => {
    const c = render(createElement(ErrorNotice, { error: "plain failure" }));
    expect(c.textContent).toContain("plain failure");
    expect(c.querySelector('[data-slot="error-notice-toggle"]')).toBeNull();
  });

  it("shows a contextual title with the parsed message as secondary line", () => {
    const c = render(
      createElement(ErrorNotice, { error: envelope, title: "Failed to bring up to date" }),
    );
    expect(c.querySelector('[data-slot="error-notice-title"]')?.textContent).toBe(
      "Failed to bring up to date",
    );
    expect(c.querySelector('[data-slot="error-notice-secondary"]')?.textContent).toBe(
      "no default flow configured — add a flow or pick one to run",
    );
  });

  it("renders the hint and variant attribute", () => {
    const c = render(
      createElement(ErrorNotice, { error: "x", hint: "Try again later", variant: "panel" }),
    );
    expect(c.querySelector('[data-slot="error-notice-hint"]')?.textContent).toBe(
      "Try again later",
    );
    expect(
      c.querySelector('[data-slot="error-notice"]')?.getAttribute("data-variant"),
    ).toBe("panel");
  });
});
