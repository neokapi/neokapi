import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { FlowPage } from "../components/FlowPage";

function renderPage() {
  return render(
    <FlowPage
      flowName="translate"
      flow={{
        steps: [
          { tool: "translate" },
          { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
        ],
      }}
      onChange={vi.fn()}
      onRun={vi.fn()}
    />,
  );
}

describe("FlowPage", () => {
  it("opens on the step editor with a Steps / Diagram switch", () => {
    renderPage();
    // The linear flow editor renders a Run button in its header.
    expect(screen.getByLabelText("Run flow")).toBeInTheDocument();
    expect(screen.getByTestId("flow-view-steps")).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("flow-view-diagram")).toBeInTheDocument();
    // The desktop records no run trace, so there is no Run view.
    expect(screen.queryByTestId("flow-view-run")).not.toBeInTheDocument();
  });

  it("shows the flow as a read-only diagram, parallel branches included", async () => {
    renderPage();
    await userEvent.click(screen.getByTestId("flow-view-diagram"));
    expect(screen.getByTestId("flow-diagram-view")).toBeInTheDocument();
    expect(screen.getByText("translate")).toBeInTheDocument();
    expect(screen.getByText("qa")).toBeInTheDocument();
    expect(screen.getByText("word-count")).toBeInTheDocument();
    expect(screen.queryByLabelText("Add tool")).not.toBeInTheDocument();
    expect(screen.queryByText("Add branch")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Remove tool")).not.toBeInTheDocument();
  });
});
