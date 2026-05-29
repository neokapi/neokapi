import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CandidateRulesList } from "../brand/CandidateRulesList";
import { BlastRadiusSummary } from "../brand/BlastRadiusSummary";
import { DriftAlert } from "../brand/DriftAlert";
import type { CandidateRule, BlastRadius, DriftResult } from "../brand/types";

const pending: CandidateRule = {
  term: "utilize",
  replacement: "use",
  correction_count: 4,
  dimension: "vocabulary",
  status: "pending",
};
const promoted: CandidateRule = {
  term: "leverage",
  replacement: "use",
  correction_count: 6,
  dimension: "vocabulary",
  status: "promoted",
  promoted_version: 3,
  auto: true,
};

describe("CandidateRulesList", () => {
  it("renders the term, replacement, and correction count", () => {
    render(<CandidateRulesList candidates={[pending]} />);
    expect(screen.getByText("utilize")).toBeInTheDocument();
    expect(screen.getByText("use")).toBeInTheDocument();
    expect(screen.getByText("4 corrections")).toBeInTheDocument();
  });

  it("fires onPromote and onReject for an actionable candidate", async () => {
    const onPromote = vi.fn();
    const onReject = vi.fn();
    render(<CandidateRulesList candidates={[pending]} onPromote={onPromote} onReject={onReject} />);
    await userEvent.click(screen.getByRole("button", { name: "Promote" }));
    await userEvent.click(screen.getByRole("button", { name: "Reject" }));
    expect(onPromote).toHaveBeenCalledWith(pending);
    expect(onReject).toHaveBeenCalledWith(pending);
  });

  it("does not offer actions for a promoted candidate and shows its version", () => {
    const onPromote = vi.fn();
    render(<CandidateRulesList candidates={[promoted]} onPromote={onPromote} />);
    expect(screen.queryByRole("button", { name: "Promote" })).not.toBeInTheDocument();
    expect(screen.getByText(/auto-promoted/)).toBeInTheDocument();
    expect(screen.getByText(/v3/)).toBeInTheDocument();
  });

  it("disables the row being acted on", () => {
    render(<CandidateRulesList candidates={[pending]} onPromote={vi.fn()} busyTerm="utilize" />);
    expect(screen.getByRole("button", { name: "Promote" })).toBeDisabled();
  });

  it("renders an empty state", () => {
    render(<CandidateRulesList candidates={[]} />);
    expect(screen.getByText(/No candidate rules yet/)).toBeInTheDocument();
  });
});

describe("BlastRadiusSummary", () => {
  const radius: BlastRadius = {
    total_blocks: 120,
    affected_blocks: 14,
    improved_blocks: 0,
    degraded_blocks: 14,
    new_violations: 18,
    resolved_violations: 0,
    critical_count: 2,
    collections: [
      {
        collection_id: "c1",
        collection_name: "Marketing",
        affected_blocks: 10,
        avg_score_delta: -6.5,
      },
    ],
  };

  it("renders the headline counts and per-collection breakdown", () => {
    render(<BlastRadiusSummary radius={radius} />);
    expect(screen.getByText("120")).toBeInTheDocument();
    expect(screen.getByText("18")).toBeInTheDocument();
    expect(screen.getByText("Marketing")).toBeInTheDocument();
    expect(screen.getByText("10 affected")).toBeInTheDocument();
  });
});

describe("DriftAlert", () => {
  it("renders a warning when drifted", () => {
    const drift: DriftResult = {
      drifted: true,
      recent_avg: 71.2,
      baseline_avg: 95,
      drop: 23.8,
      recent_days: 7,
      recent_count: 30,
      reason: "recent average dropped from baseline",
    };
    render(<DriftAlert drift={drift} />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByText(/drifting/i)).toBeInTheDocument();
  });

  it("renders nothing when stable by default", () => {
    const drift: DriftResult = {
      drifted: false,
      recent_avg: 92,
      baseline_avg: 91,
      drop: -1,
      recent_days: 7,
      recent_count: 30,
    };
    const { container } = render(<DriftAlert drift={drift} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("can show a stable confirmation", () => {
    const drift: DriftResult = {
      drifted: false,
      recent_avg: 92,
      baseline_avg: 91,
      drop: -1,
      recent_days: 7,
      recent_count: 30,
    };
    render(<DriftAlert drift={drift} showStable />);
    expect(screen.getByText(/stable/i)).toBeInTheDocument();
  });
});
