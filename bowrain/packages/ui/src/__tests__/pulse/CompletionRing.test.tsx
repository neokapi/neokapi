import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { CompletionRing } from "../../components/pulse";

describe("CompletionRing", () => {
  it("renders percentage text", () => {
    render(<CompletionRing percentage={75} />);
    expect(screen.getByText("75%")).toBeTruthy();
  });

  it("renders 0% for zero progress", () => {
    render(<CompletionRing percentage={0} />);
    expect(screen.getByText("0%")).toBeTruthy();
  });

  it("renders 100% for complete progress", () => {
    render(<CompletionRing percentage={100} />);
    expect(screen.getByText("100%")).toBeTruthy();
  });

  it("clamps values above 100", () => {
    render(<CompletionRing percentage={150} />);
    expect(screen.getByText("150%")).toBeTruthy();
  });

  it("renders SVG with correct size", () => {
    const { container } = render(<CompletionRing percentage={50} size={80} />);
    const svg = container.querySelector("svg");
    expect(svg).toBeTruthy();
    expect(svg?.getAttribute("width")).toBe("80");
    expect(svg?.getAttribute("height")).toBe("80");
  });
});
