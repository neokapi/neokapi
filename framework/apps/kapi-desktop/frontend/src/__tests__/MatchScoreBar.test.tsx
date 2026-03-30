import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { MatchScoreBar } from "@neokapi/ui-primitives";

describe("MatchScoreBar", () => {
  it("renders the percentage", () => {
    render(<MatchScoreBar score={0.85} matchType="fuzzy" />);
    expect(screen.getByText("85%")).toBeInTheDocument();
  });

  it("renders the match type label", () => {
    render(<MatchScoreBar score={0.9} matchType="fuzzy" />);
    expect(screen.getByText("fuzzy")).toBeInTheDocument();
  });

  it("renders exact match type label", () => {
    render(<MatchScoreBar score={1.0} matchType="exact" />);
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(screen.getByText("exact")).toBeInTheDocument();
  });

  it("renders generalized-exact as gen-exact", () => {
    render(<MatchScoreBar score={1.0} matchType="generalized-exact" />);
    expect(screen.getByText("gen-exact")).toBeInTheDocument();
  });

  it("renders structural-exact as struct-exact", () => {
    render(<MatchScoreBar score={1.0} matchType="structural-exact" />);
    expect(screen.getByText("struct-exact")).toBeInTheDocument();
  });

  it("renders generalized-fuzzy as gen-fuzzy", () => {
    render(<MatchScoreBar score={0.8} matchType="generalized-fuzzy" />);
    expect(screen.getByText("gen-fuzzy")).toBeInTheDocument();
  });

  it("renders structural-fuzzy as struct-fuzzy", () => {
    render(<MatchScoreBar score={0.8} matchType="structural-fuzzy" />);
    expect(screen.getByText("struct-fuzzy")).toBeInTheDocument();
  });

  it("renders unknown match type as-is", () => {
    render(<MatchScoreBar score={0.5} matchType="custom-type" />);
    expect(screen.getByText("custom-type")).toBeInTheDocument();
  });

  it("uses blue color for exact match (score = 1.0)", () => {
    render(<MatchScoreBar score={1.0} matchType="exact" />);
    const pctEl = screen.getByText("100%");
    expect(pctEl.style.color).toContain("252"); // blue hue
  });

  it("uses green color for high score (0.85-0.99)", () => {
    render(<MatchScoreBar score={0.9} matchType="fuzzy" />);
    const pctEl = screen.getByText("90%");
    expect(pctEl.style.color).toContain("155"); // green hue
  });

  it("uses amber color for medium score (0.7-0.84)", () => {
    render(<MatchScoreBar score={0.75} matchType="fuzzy" />);
    const pctEl = screen.getByText("75%");
    expect(pctEl.style.color).toContain("85"); // amber hue
  });

  it("uses red color for low score (< 0.7)", () => {
    render(<MatchScoreBar score={0.5} matchType="fuzzy" />);
    const pctEl = screen.getByText("50%");
    expect(pctEl.style.color).toContain("27"); // red hue
  });

  it("rounds percentage correctly", () => {
    render(<MatchScoreBar score={0.777} matchType="fuzzy" />);
    expect(screen.getByText("78%")).toBeInTheDocument();
  });

  it("renders 0% for zero score", () => {
    render(<MatchScoreBar score={0} matchType="fuzzy" />);
    expect(screen.getByText("0%")).toBeInTheDocument();
  });

  it("applies custom className", () => {
    const { container } = render(
      <MatchScoreBar score={0.5} matchType="fuzzy" className="my-class" />,
    );
    const wrapper = container.firstElementChild;
    expect(wrapper?.className).toContain("my-class");
  });

  it("renders the progress bar with correct width", () => {
    const { container } = render(<MatchScoreBar score={0.65} matchType="fuzzy" />);
    const bar = container.querySelector("[style*='width']");
    expect(bar).toBeInTheDocument();
    expect(bar?.getAttribute("style")).toContain("width: 65%");
  });
});
