// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import { StatRow } from "../components/ui/stat-row";
import type { StatItem } from "../components/ui/stat-row";

afterEach(cleanup);

const OVERVIEW: StatItem[] = [
  { value: 4, label: "projects" },
  { value: 61, label: "rules" },
  { value: 11, label: "suggestions" },
  { value: 1, label: "needs you", tone: "warning" },
];

/** The text assistive technology reads: everything outside an aria-hidden subtree. */
function spokenText(el: Element): string {
  let out = "";
  for (const node of Array.from(el.childNodes)) {
    if (node.nodeType === Node.TEXT_NODE) out += node.textContent ?? "";
    else if (node instanceof Element && node.getAttribute("aria-hidden") !== "true") {
      out += spokenText(node);
    }
  }
  return out.replace(/\s+/g, " ").trim();
}

describe("StatRow", () => {
  it("is a list with one item per count, each read as the count and its label", () => {
    render(<StatRow items={OVERVIEW} aria-label="Your context" />);
    const list = screen.getByRole("list", { name: "Your context" });
    const items = within(list).getAllByRole("listitem");
    expect(items.map(spokenText)).toEqual([
      "4 projects",
      "61 rules",
      "11 suggestions",
      "1 needs you",
    ]);
  });

  it("draws the value and the label, hidden from assistive technology so they are read once", () => {
    render(<StatRow items={[{ value: 61, label: "rules" }]} />);
    const item = screen.getByRole("listitem");
    const drawn = item.querySelector('[aria-hidden="true"]');
    expect(drawn?.textContent).toBe("61rules");
    expect(drawn?.querySelector(".tabular-nums")?.textContent).toBe("61");
  });

  it("takes the warning tone on the count that needs a person, and only there", () => {
    render(<StatRow items={OVERVIEW} />);
    const items = screen.getAllByRole("listitem");
    expect(items.map((li) => li.getAttribute("data-tone"))).toEqual([
      "default",
      "default",
      "default",
      "warning",
    ]);
    const warningValue = items[3].querySelector(".tabular-nums");
    expect(warningValue?.className).toContain("text-warning-ink");
    expect(items[0].querySelector(".tabular-nums")?.className).toContain("text-foreground");
  });

  it("draws the muted tone in the muted ink", () => {
    render(<StatRow items={[{ value: 3, label: "earlier", tone: "muted" }]} />);
    expect(screen.getByRole("listitem").querySelector(".tabular-nums")?.className).toContain(
      "text-muted-foreground",
    );
  });

  it("shows a zero as 0 rather than dropping the item", () => {
    render(
      <StatRow
        items={[
          { value: 4, label: "projects" },
          { value: 0, label: "needs you", tone: "warning" },
        ]}
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(spokenText(items[1])).toBe("0 needs you");
    expect(items[1].querySelector(".tabular-nums")?.textContent).toBe("0");
  });

  it("formats a large count for the UI language", () => {
    render(<StatRow items={[{ value: 12045, label: "units" }]} />);
    expect(spokenText(screen.getByRole("listitem"))).toBe("12,045 units");
  });

  it("renders a count with onClick as a button named by the count", () => {
    const onClick = vi.fn();
    render(
      <StatRow
        items={[
          { value: 4, label: "projects" },
          { value: 1, label: "needs you", tone: "warning", onClick },
        ]}
      />,
    );
    expect(screen.getAllByRole("button")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "1 needs you" }));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("makes a count with a hint focusable, so the hint is reachable from the keyboard", () => {
    render(
      <StatRow items={[{ value: 11, label: "suggestions", hint: "Advising, not yet kept" }]} />,
    );
    const item = screen.getByRole("listitem");
    const focusable = item.querySelector('[tabindex="0"]');
    expect(focusable).not.toBeNull();
    expect(spokenText(item)).toBe("11 suggestions");
  });

  it("keys items by id, so two counts may share a label", () => {
    render(
      <StatRow
        items={[
          { id: "a", value: 2, label: "rules" },
          { id: "b", value: 3, label: "rules" },
        ]}
      />,
    );
    expect(screen.getAllByRole("listitem").map(spokenText)).toEqual(["2 rules", "3 rules"]);
  });
});
