// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StepRow } from "../components/flow-editor";
import type { FlowTool } from "../components/flow-editor";
import type { ComponentSchema } from "../components/schema-form";

const TOOL: FlowTool = {
  name: "translate",
  display_name: "Translate",
  description: "Translate with AI.",
  has_schema: true,
};

const SCHEMA: ComponentSchema = {
  title: "Translate options",
  type: "object",
  properties: {
    provider: { type: "string", title: "Provider" },
  },
};

afterEach(cleanup);

describe("StepRow", () => {
  it("shows the tool's name and description", () => {
    render(
      <ul>
        <StepRow step={{ tool: "translate" }} tool={TOOL} index={0} count={2} />
      </ul>,
    );
    expect(screen.getByText("Translate")).toBeTruthy();
    expect(screen.getByText("Translate with AI.")).toBeTruthy();
  });

  it("renders the options form when opened by default and a schema is present", () => {
    render(
      <ul>
        <StepRow
          step={{ tool: "translate" }}
          tool={TOOL}
          index={0}
          count={1}
          schema={SCHEMA}
          defaultOpen
        />
      </ul>,
    );
    expect(screen.getByTestId("step-options")).toBeTruthy();
  });

  it("shows no options control without a schema", () => {
    render(
      <ul>
        <StepRow step={{ tool: "qa" }} index={0} count={1} />
      </ul>,
    );
    expect(screen.queryByLabelText("Options")).toBeNull();
  });

  it("wires the move and remove controls", async () => {
    const onRemove = vi.fn();
    const onMoveDown = vi.fn();
    render(
      <ul>
        <StepRow
          step={{ tool: "translate" }}
          tool={TOOL}
          index={0}
          count={2}
          onRemove={onRemove}
          onMoveDown={onMoveDown}
        />
      </ul>,
    );
    expect((screen.getByLabelText("Move up") as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(screen.getByLabelText("Move down"));
    await userEvent.click(screen.getByLabelText("Remove Translate"));
    expect(onMoveDown).toHaveBeenCalledOnce();
    expect(onRemove).toHaveBeenCalledOnce();
  });

  it("labels a parallel group when the step carries no tool", () => {
    render(
      <ul>
        <StepRow
          step={{ tool: "", parallel: [{ tool: "qa" }, { tool: "voice-check" }] }}
          index={0}
          count={1}
        />
      </ul>,
    );
    expect(screen.getByText("Parallel group")).toBeTruthy();
  });
});
