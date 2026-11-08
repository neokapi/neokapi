// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vite-plus/test";
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
  it("offers the command that runs its tool on files", () => {
    render(
      <ul>
        <StepRow step={{ tool: "translate" }} tool={TOOL} index={0} readOnly />
      </ul>,
    );
    const copy = screen.getByRole("button", {
      name: "Copy kapi exec translate <files...> for the terminal",
    });
    expect(copy.getAttribute("data-command")).toBe("kapi exec translate <files...>");
  });

  it("shows the tool's name and description", () => {
    render(
      <ul>
        <StepRow step={{ tool: "translate" }} tool={TOOL} index={0} />
      </ul>,
    );
    expect(screen.getByText("Translate")).toBeTruthy();
    expect(screen.getByText("Translate with AI.")).toBeTruthy();
  });

  it("renders the options form when opened by default and a schema is present", () => {
    render(
      <ul>
        <StepRow step={{ tool: "translate" }} tool={TOOL} index={0} schema={SCHEMA} defaultOpen />
      </ul>,
    );
    expect(screen.getByTestId("step-options")).toBeTruthy();
  });

  it("shows no options control without a schema", () => {
    render(
      <ul>
        <StepRow step={{ tool: "qa" }} index={0} />
      </ul>,
    );
    expect(screen.queryByLabelText("Options")).toBeNull();
  });

  it("wires the remove control", async () => {
    const onRemove = vi.fn();
    render(
      <ul>
        <StepRow step={{ tool: "translate" }} tool={TOOL} index={0} onRemove={onRemove} />
      </ul>,
    );
    await userEvent.click(screen.getByLabelText("Remove Translate"));
    expect(onRemove).toHaveBeenCalledOnce();
  });

  it("shows a grip only when the step can be reordered", () => {
    render(
      <ul>
        <StepRow step={{ tool: "translate" }} tool={TOOL} index={0} />
      </ul>,
    );
    expect(screen.queryByLabelText("Reorder Translate")).toBeNull();
  });

  it("labels a parallel group when the step carries no tool", () => {
    render(
      <ul>
        <StepRow
          step={{ tool: "", parallel: [{ tool: "qa" }, { tool: "voice-check" }] }}
          index={0}
        />
      </ul>,
    );
    expect(screen.getByText("Parallel group")).toBeTruthy();
  });
});
