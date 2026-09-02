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
    expect(screen.getByText(/No voice scores yet/)).toBeInTheDocument();
  });

  it("shows a placeholder while the review count loads", () => {
    render(<LoopStatusRow status={{}} />);
    expect(within(screen.getByTestId("loop-card-review")).getByText("·")).toBeInTheDocument();
  });

  it("hides the brand card when the rollup is unavailable", () => {
    render(<LoopStatusRow status={{ openReviewTasks: 2 }} />);
    expect(screen.queryByTestId("loop-card-brand")).not.toBeInTheDocument();
  });

  it("renders the latest-run card from the loop rollup", () => {
    render(
      <LoopStatusRow
        status={{
          latestRun: {
            state: "parked",
            stallReason: "needs_credits",
            projectName: "Website",
            updatedAt: new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString(),
          },
        }}
      />,
    );
    const run = screen.getByTestId("loop-card-run");
    expect(within(run).getByText("Parked")).toBeInTheDocument();
    expect(within(run).getByText(/needs credits/)).toBeInTheDocument();
    expect(within(run).getByText(/Website · 3h ago/)).toBeInTheDocument();
  });

  it("renders a converged run without a stall badge", () => {
    render(
      <LoopStatusRow
        status={{
          latestRun: {
            state: "converged",
            projectName: "Docs",
            updatedAt: new Date().toISOString(),
          },
        }}
      />,
    );
    const run = screen.getByTestId("loop-card-run");
    expect(within(run).getByText("Converged")).toBeInTheDocument();
    expect(within(run).queryByText(/needs/)).not.toBeInTheDocument();
  });

  it("hides the latest-run card when the workspace has never run", () => {
    render(<LoopStatusRow status={{ openReviewTasks: 0 }} />);
    expect(screen.queryByTestId("loop-card-run")).not.toBeInTheDocument();
  });

  it("renders the ship-readiness card with per-state counts", () => {
    render(
      <LoopStatusRow
        status={{
          ship: {
            governed: 3,
            aiShippable: 2,
            pending: 5,
            countedProjects: 4,
            totalProjects: 4,
          },
        }}
      />,
    );
    const ship = screen.getByTestId("loop-card-ship");
    expect(within(ship).getByText("5")).toBeInTheDocument();
    expect(within(ship).getByText(/of 10 locales shippable/)).toBeInTheDocument();
    expect(within(ship).getByText(/3 governed · 2 AI-shippable · 5 pending/)).toBeInTheDocument();
    // Full coverage: no partial-coverage qualifier.
    expect(within(ship).queryByText(/across/)).not.toBeInTheDocument();
  });

  it("labels a partial ship rollup with its project coverage", () => {
    render(
      <LoopStatusRow
        status={{
          ship: {
            governed: 1,
            aiShippable: 0,
            pending: 1,
            countedProjects: 2,
            totalProjects: 6,
          },
        }}
      />,
    );
    expect(
      within(screen.getByTestId("loop-card-ship")).getByText(/across 2 of 6 projects/),
    ).toBeInTheDocument();
  });

  it("hides the ship card when no rollup data exists", () => {
    render(<LoopStatusRow status={{ openReviewTasks: 0 }} />);
    expect(screen.queryByTestId("loop-card-ship")).not.toBeInTheDocument();
  });

  it("fires the run and delivery open handlers", () => {
    const onOpenRuns = vi.fn();
    const onOpenDelivery = vi.fn();
    render(
      <LoopStatusRow
        status={{
          latestRun: { state: "running", projectName: "Website" },
          ship: {
            governed: 1,
            aiShippable: 1,
            pending: 0,
            countedProjects: 1,
            totalProjects: 1,
          },
        }}
        onOpenRuns={onOpenRuns}
        onOpenDelivery={onOpenDelivery}
      />,
    );
    fireEvent.click(screen.getByTestId("loop-card-run"));
    fireEvent.click(screen.getByTestId("loop-card-ship"));
    expect(onOpenRuns).toHaveBeenCalledTimes(1);
    expect(onOpenDelivery).toHaveBeenCalledTimes(1);
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
    const onOpenVoiceDashboard = vi.fn();
    render(
      <LoopStatusRow
        status={{
          latestActivity: { summary: "x", created_at: new Date().toISOString() },
          openReviewTasks: 1,
          brand: { averageScore: 80, scoredProjects: 1, driftingProjects: 0 },
        }}
        onOpenActivities={onOpenActivities}
        onOpenTasks={onOpenTasks}
        onOpenVoiceDashboard={onOpenVoiceDashboard}
      />,
    );
    fireEvent.click(screen.getByTestId("loop-card-activity"));
    fireEvent.click(screen.getByTestId("loop-card-review"));
    fireEvent.click(screen.getByTestId("loop-card-brand"));
    expect(onOpenActivities).toHaveBeenCalledTimes(1);
    expect(onOpenTasks).toHaveBeenCalledTimes(1);
    expect(onOpenVoiceDashboard).toHaveBeenCalledTimes(1);
  });
});
