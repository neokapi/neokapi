// @vitest-environment jsdom
import { type ComponentProps } from "react";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { LinearFlowEditor } from "../components/flow-editor";
import type { FlowSpec, FlowTool } from "../components/flow-editor";
import type { ComponentSchema } from "../components/schema-form";

const TOOLS: FlowTool[] = [
  {
    name: "recycle",
    display_name: "Recycle",
    description: "Reuse approved wording.",
    has_schema: false,
  },
  {
    name: "translate",
    display_name: "Translate",
    description: "Translate with AI.",
    has_schema: true,
  },
  {
    name: "qa",
    display_name: "Quality Check",
    description: "Check the result.",
    has_schema: false,
  },
];

const TRANSLATE_SCHEMA: ComponentSchema = {
  title: "Translate options",
  type: "object",
  properties: {
    provider: { type: "string", title: "Provider", description: "Which provider to use." },
  },
};

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

function renderEditor(overrides: Partial<ComponentProps<typeof LinearFlowEditor>> = {}) {
  const onChange = vi.fn();
  const flow: FlowSpec = { steps: [{ tool: "recycle" }, { tool: "translate" }] };
  const utils = render(
    <LinearFlowEditor
      flowName="translate-and-qa"
      flow={flow}
      tools={TOOLS}
      onChange={onChange}
      {...overrides}
    />,
  );
  return { onChange, ...utils };
}

describe("LinearFlowEditor", () => {
  it("renders one row per step plus the chip strip", () => {
    renderEditor();
    expect(screen.getAllByTestId("step-row")).toHaveLength(2);
    const chips = screen.getByTestId("step-chips");
    expect(within(chips).getByText("Recycle")).toBeTruthy();
    expect(within(chips).getByText("Translate")).toBeTruthy();
  });

  it("appends a step picked from the tool list", async () => {
    const { onChange } = renderEditor();
    await userEvent.click(screen.getByTestId("add-step"));
    const toolButtons = await screen.findAllByTestId("add-step-tool");
    const checkTool = toolButtons.find((b) => b.textContent?.includes("Quality Check"));
    await userEvent.click(checkTool!);
    expect(onChange).toHaveBeenCalledWith({
      steps: [{ tool: "recycle" }, { tool: "translate" }, { tool: "qa" }],
    });
  });

  it("removes a step", async () => {
    const { onChange } = renderEditor();
    const rows = screen.getAllByTestId("step-row");
    await userEvent.click(within(rows[0]).getByLabelText(/^Remove /));
    expect(onChange).toHaveBeenCalledWith({ steps: [{ tool: "translate" }] });
  });

  it("reorders steps with move down", async () => {
    const { onChange } = renderEditor();
    const rows = screen.getAllByTestId("step-row");
    await userEvent.click(within(rows[0]).getByLabelText("Move down"));
    expect(onChange).toHaveBeenCalledWith({
      steps: [{ tool: "translate" }, { tool: "recycle" }],
    });
  });

  it("disables move up on the first step and move down on the last", () => {
    renderEditor();
    const rows = screen.getAllByTestId("step-row");
    expect((within(rows[0]).getByLabelText("Move up") as HTMLButtonElement).disabled).toBe(true);
    expect((within(rows[1]).getByLabelText("Move down") as HTMLButtonElement).disabled).toBe(true);
  });

  it("expands a step's options when the tool has a schema", async () => {
    renderEditor({ onGetSchema: (name) => (name === "translate" ? TRANSLATE_SCHEMA : null) });
    const rows = screen.getAllByTestId("step-row");
    expect(within(rows[0]).queryByLabelText("Options")).toBeNull();
    await userEvent.click(within(rows[1]).getByLabelText("Options"));
    expect(within(rows[1]).getByTestId("step-options")).toBeTruthy();
  });

  it("toggles the project default", async () => {
    const onToggleDefault = vi.fn();
    renderEditor({ isDefault: false, onToggleDefault });
    await userEvent.click(screen.getByLabelText("Set as the project's default flow"));
    expect(onToggleDefault).toHaveBeenCalledWith(true);
  });

  it("renames the flow", async () => {
    const onRename = vi.fn();
    renderEditor({ onRename });
    await userEvent.click(screen.getByLabelText("Rename flow"));
    const input = screen.getByLabelText("Flow name");
    await userEvent.clear(input);
    await userEvent.type(input, "my-flow");
    await userEvent.click(screen.getByLabelText("Save"));
    expect(onRename).toHaveBeenCalledWith("my-flow");
  });

  it("shows the empty state with the template slot when a flow has no steps", () => {
    renderEditor({
      flow: { steps: [] },
      templateLibrary: <div data-testid="tpl">templates</div>,
    });
    expect(screen.getByTestId("tpl")).toBeTruthy();
    expect(screen.getByTestId("add-step")).toBeTruthy();
    expect(screen.queryByTestId("step-row")).toBeNull();
  });

  it("hides editing controls when read-only", () => {
    renderEditor({ readOnly: true, onRun: vi.fn() });
    expect(screen.queryByTestId("add-step")).toBeNull();
    expect(screen.queryByLabelText("Move up")).toBeNull();
    expect(screen.queryByLabelText(/^Remove /)).toBeNull();
  });
});

describe("LinearFlowEditor parallel groups", () => {
  it("appends a parallel group seeded with the picked tool", async () => {
    const { onChange } = renderEditor();
    await userEvent.click(screen.getByTestId("add-parallel-group"));
    const toolButtons = await screen.findAllByTestId("add-step-tool");
    await userEvent.click(toolButtons.find((b) => b.textContent?.includes("Quality Check"))!);
    expect(onChange).toHaveBeenCalledWith({
      steps: [{ tool: "recycle" }, { tool: "translate" }, { tool: "", parallel: [{ tool: "qa" }] }],
    });
  });

  it("renders a parallel group with a branch row per branch", () => {
    renderEditor({
      flow: { steps: [{ tool: "", parallel: [{ tool: "qa" }, { tool: "translate" }] }] },
    });
    expect(screen.getByTestId("parallel-group")).toBeTruthy();
    const branches = screen.getByTestId("parallel-branches");
    expect(within(branches).getAllByTestId("step-row")).toHaveLength(2);
    // A branch has no reorder controls (parallel branches are unordered).
    expect(within(branches).queryByLabelText("Move up")).toBeNull();
  });

  it("adds a branch to a group", async () => {
    const onChange = vi.fn();
    render(
      <LinearFlowEditor
        flowName="f"
        flow={{ steps: [{ tool: "", parallel: [{ tool: "qa" }] }] }}
        tools={TOOLS}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByTestId("add-branch"));
    const toolButtons = await screen.findAllByTestId("add-step-tool");
    await userEvent.click(toolButtons.find((b) => b.textContent?.includes("Translate"))!);
    expect(onChange).toHaveBeenCalledWith({
      steps: [{ tool: "", parallel: [{ tool: "qa" }, { tool: "translate" }] }],
    });
  });

  it("removes a branch from a group", async () => {
    const onChange = vi.fn();
    render(
      <LinearFlowEditor
        flowName="f"
        flow={{ steps: [{ tool: "", parallel: [{ tool: "qa" }, { tool: "translate" }] }] }}
        tools={TOOLS}
        onChange={onChange}
      />,
    );
    const branches = screen.getByTestId("parallel-branches");
    const rows = within(branches).getAllByTestId("step-row");
    await userEvent.click(within(rows[0]).getByLabelText(/^Remove /));
    expect(onChange).toHaveBeenCalledWith({
      steps: [{ tool: "", parallel: [{ tool: "translate" }] }],
    });
  });

  it("removes the whole group", async () => {
    const onChange = vi.fn();
    render(
      <LinearFlowEditor
        flowName="f"
        flow={{ steps: [{ tool: "recycle" }, { tool: "", parallel: [{ tool: "qa" }] }] }}
        tools={TOOLS}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByLabelText("Remove parallel group"));
    expect(onChange).toHaveBeenCalledWith({ steps: [{ tool: "recycle" }] });
  });
});
