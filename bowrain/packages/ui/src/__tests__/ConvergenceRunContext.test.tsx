import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConvergenceRunContext } from "../components/ConvergenceRunContext";
import type { ConvergenceRun } from "../types/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeRun(overrides: Partial<ConvergenceRun> = {}): ConvergenceRun {
  return {
    id: "run-1",
    project_id: "proj-1",
    trigger: "push",
    state: "converged",
    passes: 1,
    locales: [
      { locale: "fr-FR", state: "shippable", units: 12, produced: 12, viaTM: 4, viaAI: 8 },
      { locale: "de-DE", state: "shippable", units: 8, produced: 8, viaTM: 0, viaAI: 8 },
    ],
    created_at: "2026-07-18T10:00:00Z",
    finished_at: "2026-07-18T10:01:00Z",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Awaiting-review completion element (governed review default-on)
// ---------------------------------------------------------------------------

describe("ConvergenceRunContext awaiting review", () => {
  it("shows the awaiting-review count with per-locale breakdown on a converged run", () => {
    render(<ConvergenceRunContext run={makeRun()} />);

    expect(screen.getByText("20 blocks awaiting review")).toBeInTheDocument();
    expect(screen.getByText(/fr-FR 12 · de-DE 8/)).toBeInTheDocument();
  });

  it("deep-links into the review surface via onOpenReviewSurface", async () => {
    const onOpenReviewSurface = vi.fn();
    const onOpenReview = vi.fn();
    render(
      <ConvergenceRunContext
        run={makeRun()}
        onOpenReview={onOpenReview}
        onOpenReviewSurface={onOpenReviewSurface}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Open review" }));
    expect(onOpenReviewSurface).toHaveBeenCalledTimes(1);
    expect(onOpenReview).not.toHaveBeenCalled();
  });

  it("falls back to onOpenReview when no surface callback is wired", async () => {
    const onOpenReview = vi.fn();
    render(<ConvergenceRunContext run={makeRun()} onOpenReview={onOpenReview} />);

    await userEvent.click(screen.getByRole("button", { name: "Open review" }));
    expect(onOpenReview).toHaveBeenCalledTimes(1);
  });

  it("uses the singular form for a single block", () => {
    const run = makeRun({
      locales: [{ locale: "fr-FR", state: "shippable", units: 1, produced: 1, viaAI: 1 }],
    });
    render(<ConvergenceRunContext run={run} />);

    expect(screen.getByText("1 block awaiting review")).toBeInTheDocument();
  });

  it("renders nothing extra when the run produced no translations", () => {
    const run = makeRun({
      locales: [{ locale: "fr-FR", state: "shippable", units: 0, produced: 0 }],
    });
    render(<ConvergenceRunContext run={run} />);

    expect(screen.queryByText(/awaiting review/)).not.toBeInTheDocument();
  });

  it("suppresses the element when the project opted out of review", () => {
    render(<ConvergenceRunContext run={makeRun()} reviewEnabled={false} />);

    expect(screen.queryByText(/awaiting review/)).not.toBeInTheDocument();
  });

  it("does not render the element while the run is still running", () => {
    render(<ConvergenceRunContext run={makeRun({ state: "running" })} />);

    expect(screen.queryByText(/awaiting review/)).not.toBeInTheDocument();
  });

  it("folds the count into the parked banner instead of a second element", () => {
    render(<ConvergenceRunContext run={makeRun({ state: "parked" })} />);

    expect(screen.getByText("Parked, pending review")).toBeInTheDocument();
    expect(
      screen.getByText(/20 blocks awaiting review \(fr-FR 12 · de-DE 8\)/),
    ).toBeInTheDocument();
    // No standalone converged-style element alongside the parked banner.
    expect(screen.queryByText("20 blocks awaiting review")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// No-target-locales configuration hold (never a silent "Up to date")
// ---------------------------------------------------------------------------

describe("ConvergenceRunContext no target languages", () => {
  it("labels a no_target_locales park with the configuration banner", () => {
    const run = makeRun({
      state: "parked",
      stall_reason: "no_target_locales",
      passes: 0,
      locales: [],
    });
    render(<ConvergenceRunContext run={run} />);

    expect(screen.getByText("No target languages configured")).toBeInTheDocument();
    expect(screen.getByText(/nothing to converge toward/)).toBeInTheDocument();
    // Not the generic pending-review park: the next action is configuration.
    expect(screen.queryByText("Parked, pending review")).not.toBeInTheDocument();
  });
});
