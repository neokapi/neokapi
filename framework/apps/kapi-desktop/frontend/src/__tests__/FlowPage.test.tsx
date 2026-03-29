import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { FlowPage } from "../components/FlowPage";

describe("FlowPage", () => {
  it("shows empty state when no flows", () => {
    render(
      <FlowPage
        project={{ version: "v1", name: "Test", flows: {} }}
        onUpdate={vi.fn()}
      />,
    );
    expect(screen.getByText(/No flows yet/)).toBeInTheDocument();
  });

  it("lists flows in sidebar", () => {
    render(
      <FlowPage
        project={{
          version: "v1",
          name: "Test",
          flows: {
            translate: { steps: [{ tool: "ai-translate" }] },
            pseudo: { steps: [{ tool: "pseudo-translate" }] },
          },
        }}
        onUpdate={vi.fn()}
      />,
    );
    expect(screen.getByText("translate")).toBeInTheDocument();
    expect(screen.getByText("pseudo")).toBeInTheDocument();
  });

  it("adds a new flow when clicking +", async () => {
    const onUpdate = vi.fn();
    render(
      <FlowPage
        project={{ version: "v1", name: "Test", flows: {} }}
        onUpdate={onUpdate}
      />,
    );

    await userEvent.click(screen.getByLabelText("New flow"));
    expect(onUpdate).toHaveBeenCalledOnce();
    const updated = onUpdate.mock.calls[0][0];
    expect(Object.keys(updated.flows)).toHaveLength(1);
  });

  it("shows visual flow editor when flow is selected", async () => {
    render(
      <FlowPage
        project={{
          version: "v1",
          name: "Test",
          flows: {
            translate: { steps: [{ tool: "ai-translate" }] },
          },
        }}
        onUpdate={vi.fn()}
        onRunFlow={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByText("translate"));
    // The FlowEditor renders with a Run button and tool palette.
    expect(screen.getByLabelText("Run flow")).toBeInTheDocument();
  });
});
