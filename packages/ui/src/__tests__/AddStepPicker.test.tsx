// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { AddStepPicker } from "../components/flow-editor";
import type { FlowTool } from "../components/flow-editor";

const TOOLS: FlowTool[] = [
  { name: "recycle", display_name: "Recycle", description: "Reuse approved wording." },
  { name: "translate", display_name: "Translate", description: "Translate with AI." },
  { name: "qa", display_name: "Quality Check", description: "Check the result." },
];

beforeAll(() => {
  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  }
  if (typeof Element !== "undefined") {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  }
});

afterEach(cleanup);

describe("AddStepPicker", () => {
  it("lists every tool when open", () => {
    render(<AddStepPicker tools={TOOLS} onAdd={vi.fn()} defaultOpen />);
    expect(screen.getAllByTestId("add-step-tool")).toHaveLength(3);
  });

  it("filters the list by the search box", async () => {
    render(<AddStepPicker tools={TOOLS} onAdd={vi.fn()} defaultOpen />);
    await userEvent.type(screen.getByLabelText("Search tools"), "qual");
    const remaining = screen.getAllByTestId("add-step-tool");
    expect(remaining).toHaveLength(1);
    expect(remaining[0].textContent).toContain("Quality Check");
  });

  it("shows an empty message when nothing matches", async () => {
    render(<AddStepPicker tools={TOOLS} onAdd={vi.fn()} defaultOpen />);
    await userEvent.type(screen.getByLabelText("Search tools"), "zzz");
    expect(screen.queryAllByTestId("add-step-tool")).toHaveLength(0);
    expect(screen.getByText("No tools match your search.")).toBeTruthy();
  });

  it("calls onAdd with the tool name when a tool is picked", async () => {
    const onAdd = vi.fn();
    render(<AddStepPicker tools={TOOLS} onAdd={onAdd} defaultOpen />);
    const translate = screen
      .getAllByTestId("add-step-tool")
      .find((b) => b.textContent?.includes("Translate"));
    await userEvent.click(translate!);
    expect(onAdd).toHaveBeenCalledWith("translate");
  });
});
