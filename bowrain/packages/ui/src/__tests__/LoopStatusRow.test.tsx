import { describe, it, expect, vi } from "vite-plus/test";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { LoopStatusRow } from "../components/LoopStatusRow";

describe("LoopStatusRow", () => {
  it("renders activity, review count, and brand health from props", () => {
    render(
      <LoopStatusRow
        status={{
          latestActivity: {
            summary: "Translate flow completed for Website (fr)",
            created_at: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
          },
          openReviewTasks: 7,
          brand: { averageScore: 86, scoredProjects: 3, driftingProjects: 1 },
        }}
      />,
    );

    const activity = screen.getByTestId("loop-card-activity");
    expect(
      within(activity).getByText("Translate flow completed for Website (fr)"),
    ).toBeInTheDocument();
    expect(within(activity).getByText("5m ago")).toBeInTheDocument();

    const review = screen.getByTestId("loop-card-review");
    expect(within(review).getByText("7")).toBeInTheDocument();
    expect(within(review).getByText(/open review tasks for you/)).toBeInTheDocument();

    const brand = screen.getByTestId("loop-card-brand");
    expect(within(brand).getByText("86")).toBeInTheDocument();
    expect(within(brand).getByText(/average across 3 scored projects/)).toBeInTheDocument();
    expect(within(brand).getByText(/1 drifting/)).toBeInTheDocument();
  });

  it("shows the zero states", () => {
    render(
      <LoopStatusRow
        status={{
          openReviewTasks: 0,
          brand: { averageScore: null, scoredProjects: 0, driftingProjects: 0 },
        }}
      />,
    );
    expect(screen.getByText(/No loop activity yet/)).toBeInTheDocument();
    expect(screen.getByText(/Nothing is waiting on you/)).toBeInTheDocument();
    expect(screen.getByText(/No brand scores yet/)).toBeInTheDocument();
  });

  it("shows a placeholder while the review count loads", () => {
    render(<LoopStatusRow status={{}} />);
    expect(within(screen.getByTestId("loop-card-review")).getByText("—")).toBeInTheDocument();
  });

  it("hides the brand card when the rollup is unavailable", () => {
    render(<LoopStatusRow status={{ openReviewTasks: 2 }} />);
    expect(screen.queryByTestId("loop-card-brand")).not.toBeInTheDocument();
  });

  it("hides the drifting badge when nothing drifts", () => {
    render(
      <LoopStatusRow
        status={{ brand: { averageScore: 91, scoredProjects: 2, driftingProjects: 0 } }}
      />,
    );
    expect(screen.queryByText(/drifting/)).not.toBeInTheDocument();
  });

  it("fires the per-card open handlers", () => {
    const onOpenActivities = vi.fn();
    const onOpenTasks = vi.fn();
    const onOpenBrandDashboard = vi.fn();
    render(
      <LoopStatusRow
        status={{
          latestActivity: { summary: "x", created_at: new Date().toISOString() },
          openReviewTasks: 1,
          brand: { averageScore: 80, scoredProjects: 1, driftingProjects: 0 },
        }}
        onOpenActivities={onOpenActivities}
        onOpenTasks={onOpenTasks}
        onOpenBrandDashboard={onOpenBrandDashboard}
      />,
    );
    fireEvent.click(screen.getByTestId("loop-card-activity"));
    fireEvent.click(screen.getByTestId("loop-card-review"));
    fireEvent.click(screen.getByTestId("loop-card-brand"));
    expect(onOpenActivities).toHaveBeenCalledTimes(1);
    expect(onOpenTasks).toHaveBeenCalledTimes(1);
    expect(onOpenBrandDashboard).toHaveBeenCalledTimes(1);
  });
});
