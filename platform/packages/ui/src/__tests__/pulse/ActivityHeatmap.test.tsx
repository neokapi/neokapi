import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ActivityHeatmap } from "../../components/pulse";

describe("ActivityHeatmap", () => {
  it("renders with data", () => {
    const days = [
      { date: "2026-03-01", count: 5 },
      { date: "2026-03-15", count: 10 },
    ];
    render(<ActivityHeatmap days={days} />);
    expect(screen.getByRole("img")).toBeTruthy();
    expect(screen.getByText(/activities in the last year/)).toBeTruthy();
  });

  it("renders empty state", () => {
    render(<ActivityHeatmap days={[]} />);
    expect(screen.getByText("0 activities in the last year")).toBeTruthy();
  });

  it("renders cells with tooltips", () => {
    const days = [{ date: "2026-03-01", count: 3 }];
    render(<ActivityHeatmap days={days} />);
    expect(screen.getByText("2026-03-01: 3 activities")).toBeTruthy();
  });
});
