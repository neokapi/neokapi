import { describe, it, expect, beforeAll } from "vitest";
import { render, screen } from "@testing-library/react";
import { TrendAreaChart } from "../../components/pulse";

beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

describe("TrendAreaChart", () => {
  it("renders chart container when data is provided", () => {
    const data = [
      { date: "Mar 1", value: 12 },
      { date: "Mar 5", value: 25 },
      { date: "Mar 10", value: 18 },
    ];
    const { container } = render(<TrendAreaChart data={data} />);
    expect(container.firstChild).toBeTruthy();
  });

  it("shows empty state when no data provided", () => {
    render(<TrendAreaChart data={[]} />);
    expect(screen.getByText("No activity data yet.")).toBeTruthy();
  });
});
