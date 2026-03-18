import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent } from "@testing-library/react";
import { BravoUsageDashboard } from "../components/bravo/BravoUsageDashboard";
import type { BravoUsageSummary } from "../types/api";

const usage: BravoUsageSummary = {
  workspace_id: "ws-1",
  total_input_tokens: 45200,
  total_output_tokens: 12800,
  total_container_sec: 3720,
  message_count: 38,
};

describe("BravoUsageDashboard", () => {
  it("renders all stat cards", () => {
    render(<BravoUsageDashboard usage={usage} />);

    // Total tokens: (45200+12800)/1000 = 58.0k
    expect(screen.getByText("58.0k")).toBeDefined();
    expect(screen.getByText("Total tokens")).toBeDefined();

    // Messages
    expect(screen.getByText("38")).toBeDefined();
    expect(screen.getByText("Messages")).toBeDefined();

    // Container time: ceil(3720/60) = 62m
    expect(screen.getByText("62m")).toBeDefined();
    expect(screen.getByText("Container time")).toBeDefined();

    // Avg tokens/msg: round(58000/38) = 1526
    expect(screen.getByText("1,526")).toBeDefined();
    expect(screen.getByText("Avg tokens/msg")).toBeDefined();
  });

  it("renders token breakdown bars", () => {
    render(<BravoUsageDashboard usage={usage} />);

    expect(screen.getByText("Input tokens")).toBeDefined();
    expect(screen.getByText("Output tokens")).toBeDefined();
    // Input: 45200/58000 = 78%
    expect(screen.getByText("45.2k (78%)")).toBeDefined();
    // Output: 12800/58000 = 22%
    expect(screen.getByText("12.8k (22%)")).toBeDefined();
  });

  it("renders date range buttons when handler provided", () => {
    const onDateRangeChange = vi.fn();
    render(<BravoUsageDashboard usage={usage} onDateRangeChange={onDateRangeChange} />);

    expect(screen.getByText("7d")).toBeDefined();
    expect(screen.getByText("30d")).toBeDefined();
    expect(screen.getByText("90d")).toBeDefined();
  });

  it("calls onDateRangeChange when date range button clicked", () => {
    const onDateRangeChange = vi.fn();
    render(<BravoUsageDashboard usage={usage} onDateRangeChange={onDateRangeChange} />);

    fireEvent.click(screen.getByText("7d"));
    expect(onDateRangeChange).toHaveBeenCalledTimes(1);
    const [from, to] = onDateRangeChange.mock.calls[0];
    expect(typeof from).toBe("string");
    expect(typeof to).toBe("string");
  });

  it("does not show date range buttons without handler", () => {
    render(<BravoUsageDashboard usage={usage} />);

    // Buttons should not be rendered
    expect(screen.queryByText("7d")).toBeNull();
    expect(screen.queryByText("90d")).toBeNull();
  });

  it("handles zero messages gracefully", () => {
    const emptyUsage: BravoUsageSummary = {
      ...usage,
      message_count: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_container_sec: 0,
    };
    render(<BravoUsageDashboard usage={emptyUsage} />);

    expect(screen.getByText("0.0k")).toBeDefined();
  });
});
