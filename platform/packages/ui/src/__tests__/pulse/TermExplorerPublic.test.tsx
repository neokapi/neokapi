import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TermExplorerPublic } from "../../components/pulse";
import { mockTerms } from "../../stories/pulse/pulse-fixtures";

describe("TermExplorerPublic", () => {
  it("renders term names", () => {
    render(<TermExplorerPublic terms={mockTerms} />);
    expect(screen.getByText("workspace")).toBeTruthy();
    expect(screen.getByText("stream")).toBeTruthy();
    expect(screen.getAllByText("collection").length).toBeGreaterThan(0);
  });

  it("shows empty state when no terms provided", () => {
    render(<TermExplorerPublic terms={[]} />);
    expect(screen.getByText("No terminology published yet.")).toBeTruthy();
  });

  it("filters terms by search input", () => {
    render(<TermExplorerPublic terms={mockTerms} />);
    const input = screen.getByPlaceholderText("Search terminology...");
    fireEvent.change(input, { target: { value: "workspace" } });
    expect(screen.getByText("workspace")).toBeTruthy();
    expect(screen.queryByText("stream")).toBeNull();
    expect(screen.queryByText("collection")).toBeNull();
  });
});
