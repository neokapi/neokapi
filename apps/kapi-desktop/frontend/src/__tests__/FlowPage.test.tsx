import { render, screen } from "./testUtils";
import { describe, it, expect, vi } from "vitest";
import { FlowPage } from "../components/FlowPage";

describe("FlowPage", () => {
  it("renders the visual flow editor", () => {
    render(
      <FlowPage
        flowName="translate"
        flow={{ steps: [{ tool: "translate" }] }}
        onChange={vi.fn()}
        onRun={vi.fn()}
      />,
    );
    // The linear flow editor renders a Run button in its header.
    expect(screen.getByLabelText("Run flow")).toBeInTheDocument();
  });
});
