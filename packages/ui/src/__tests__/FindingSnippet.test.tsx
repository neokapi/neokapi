// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { FindingSnippet } from "../components/review/FindingSnippet";
import type { Run } from "../components/preview/types";

afterEach(cleanup);

const plain: Run[] = [{ text: "Please utilize the dashboard" }];

const withPlaceholder: Run[] = [
  { text: "Hello " },
  { ph: { id: "name", type: "var", data: "{name}", equiv: "name" } },
  { text: ", welcome" },
];

const mark = (root: HTMLElement) =>
  root.querySelector<HTMLElement>('mark[data-overlay-type="finding"]');

describe("FindingSnippet", () => {
  it("reads the block's text with the finding's span marked in its tone", () => {
    const { container } = render(
      <FindingSnippet
        runs={plain}
        locale="en"
        anchor={{ kind: "range", start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } }}
        tone="destructive"
        label='Forbidden term "utilize" found'
        data-testid="snippet"
      />,
    );
    const root = container.querySelector<HTMLElement>('[data-snippet="runs"]')!;
    expect(root.textContent).toBe("Please utilize the dashboard");
    expect(root.getAttribute("lang")).toBe("en");
    const m = mark(root)!;
    expect(m.textContent).toBe("utilize");
    expect(m.className).toContain("decoration-destructive");
    expect(m.getAttribute("title")).toBe('Forbidden term "utilize" found');
  });

  it("marks the whole text for a block anchor", () => {
    const { container } = render(
      <FindingSnippet runs={plain} anchor={{ kind: "block" }} tone="warning" />,
    );
    const m = mark(container)!;
    expect(m.textContent).toBe("Please utilize the dashboard");
    expect(m.className).toContain("decoration-warning");
  });

  it("keeps a placeholder as a chip rather than dropping it, and marks it for a run anchor", () => {
    const { container } = render(
      <FindingSnippet
        runs={withPlaceholder}
        anchor={{ kind: "run", runId: "name" }}
        tone="destructive"
        label="The target drops {name}."
      />,
    );
    const root = container.querySelector<HTMLElement>('[data-snippet="runs"]')!;
    expect(root.textContent).toContain("Hello ");
    expect(root.textContent).toContain(", welcome");
    const chip = root.querySelector<HTMLElement>("[data-inline-code]")!;
    expect(chip).toBeTruthy();
    expect(chip.closest('mark[data-overlay-type="finding"]')).toBeTruthy();
  });

  it("shows the text unmarked when the finding has no position", () => {
    const { container } = render(<FindingSnippet runs={plain} tone="muted" />);
    expect(container.textContent).toBe("Please utilize the dashboard");
    expect(mark(container)).toBeNull();
  });

  it("falls back to the quoted text when the block's runs are unavailable", () => {
    const { container } = render(
      <FindingSnippet
        fallbackText="Acme Cloud"
        locale="de-DE"
        tone="destructive"
        label="Missing"
      />,
    );
    const root = container.querySelector<HTMLElement>('mark[data-snippet="fallback"]')!;
    expect(root.getAttribute("lang")).toBe("de-DE");
    expect(root.getAttribute("data-overlay-type")).toBe("finding");
    expect(root.textContent).toBe("Acme Cloud");
    expect(root.className).toContain("decoration-destructive");
  });

  it("renders nothing with neither runs nor a quote", () => {
    const { container } = render(<FindingSnippet tone="muted" />);
    expect(container.innerHTML).toBe("");
  });
});
