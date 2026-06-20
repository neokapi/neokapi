import { render, screen } from "@testing-library/react";
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
    // FlowEditor renders a Run button in its toolbar.
    expect(screen.getByLabelText("Run flow")).toBeInTheDocument();
  });
});
