// @vitest-environment jsdom
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { ReaderView, type ReaderViewProps } from "../ReaderView";
import type { ScreenTarget } from "../port";
import { READER_CLEAN, READER_REFUSED, READER_WITH_FIXES } from "../stories/fixtures";
import type { ReaderFile } from "../types";

function renderView(reader: ReaderFile, over: Partial<ReaderViewProps> = {}) {
  const props = {
    reader,
    onFix: vi.fn(),
    onFixAll: vi.fn(),
    onBack: vi.fn(),
    navigate: vi.fn<(target: ScreenTarget) => void>(),
    ...over,
  };
  render(<ReaderView {...props} />);
  return props;
}

function block(id: string): HTMLElement {
  const el = document.querySelector<HTMLElement>(
    `[data-slot="reader-block"][data-block-id="${id}"]`,
  );
  if (!el) throw new Error(`no block ${id}`);
  return el;
}

describe("the Reader", () => {
  it("draws the file's blocks, each finding on its words, and the rules in the margin", () => {
    renderView(READER_WITH_FIXES);
    expect(screen.getByRole("heading", { name: "docs/billing.md" })).toBeInTheDocument();
    expect(screen.getByText("3 blocks · 3 findings")).toBeInTheDocument();
    const marks = block("p2").querySelectorAll('mark[data-overlay-type="finding"]');
    expect([...marks].map((m) => m.textContent)).toEqual(["Login", "utilize"]);
    expect(document.querySelectorAll('[data-slot="reader-rule"]')).toHaveLength(6);
  });

  it("applies one fix and every fix through the host", async () => {
    const { onFix, onFixAll } = renderView(READER_WITH_FIXES);
    await userEvent.click(within(block("p2")).getAllByRole("button", { name: "Fix" })[0]);
    expect(onFix).toHaveBeenCalledWith("a1f0-1");
    await userEvent.click(screen.getByRole("button", { name: "Fix 2 in this file" }));
    expect(onFixAll).toHaveBeenCalled();
  });

  it("draws no Fix when the host applies no fix, and still says which findings are edited by hand", () => {
    renderView(READER_WITH_FIXES, { onFix: undefined, onFixAll: undefined });
    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
    expect(within(block("p3")).getByText("Edit by hand")).toBeInTheDocument();
    expect(document.querySelectorAll('[data-slot="reader-finding"]').length).toBeGreaterThan(0);
  });

  it("says why a refused fix did not apply, under its finding", () => {
    renderView(READER_REFUSED);
    const row = block("p2").querySelector<HTMLElement>(
      '[data-slot="reader-finding"][data-id="b2c3-1"]',
    )!;
    expect(within(row).getByRole("alert").textContent).toMatch(/^Not fixed: the edit introduces/);
  });

  it("leads a file with no rules to the rules, when the host opens them", async () => {
    const empty: ReaderFile = { ...READER_CLEAN, rules: [], rules_total: 0 };
    const { navigate } = renderView(empty);
    await userEvent.click(screen.getByRole("button", { name: "Open Context" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "rules" });
  });

  it("hides Open Context and the back button when the host has neither", () => {
    const empty: ReaderFile = { ...READER_CLEAN, rules: [], rules_total: 0 };
    renderView(empty, { canNavigate: () => false, onBack: undefined });
    expect(screen.queryByRole("button", { name: "Open Context" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
    expect(screen.getByText("No rules hold here yet.")).toBeInTheDocument();
  });
});
