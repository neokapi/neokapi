import { describe, it, expect, vi } from "vite-plus/test";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CandidateReview } from "../voice/CandidateReview";
import { BlastRadiusSummary } from "../voice/BlastRadiusSummary";
import { DriftAlert } from "../voice/DriftAlert";
import type { CandidateRule, BlastRadius, DriftResult } from "../voice/types";

const pending: CandidateRule = {
  term: "utilize",
  replacement: "use",
  correction_count: 4,
  dimension: "vocabulary",
  status: "pending",
};
const second: CandidateRule = {
  term: "synergy",
  replacement: "teamwork",
  correction_count: 3,
  dimension: "tone",
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
  decided_at: "2026-10-01T09:00:00Z",
};
const rejected: CandidateRule = {
  term: "disrupt",
  replacement: "improve",
  correction_count: 3,
  dimension: "vocabulary",
  status: "rejected",
  decided_by: "u-maria",
  decided_at: "2026-10-02T09:00:00Z",
};

const names = (id: string | null | undefined) => (id === "u-maria" ? "Maria Lund" : (id ?? ""));

function review(
  candidates: CandidateRule[],
  props: Partial<React.ComponentProps<typeof CandidateReview>> = {},
) {
  const onPromote = vi.fn();
  const onReject = vi.fn();
  render(
    <CandidateReview
      candidates={candidates}
      onPromote={onPromote}
      onReject={onReject}
      nameOf={names}
      {...props}
    />,
  );
  return { onPromote, onReject };
}

const detail = () => document.querySelector<HTMLElement>('[data-slot="candidate-detail"]')!;
const selectedRow = () =>
  document.querySelector<HTMLElement>('[data-slot="candidate-item"][aria-selected="true"]');

describe("CandidateReview", () => {
  it("opens on the first suggestion, with the rule as the detail's title", () => {
    review([pending, second]);
    expect(selectedRow()?.dataset.term).toBe("utilize");
    const toolbar = within(detail()).getByRole("heading");
    expect(toolbar).toHaveTextContent("Write use, not utilize");
    expect(within(detail()).getByText("4")).toBeInTheDocument();
  });

  it("promotes from the toolbar and moves on to the next suggestion", async () => {
    const { onPromote } = review([pending, second]);
    await userEvent.click(within(detail()).getByRole("button", { name: "Promote" }));
    expect(onPromote).toHaveBeenCalledWith(pending, undefined);
    expect(selectedRow()?.dataset.term).toBe("synergy");
    const decided = document.querySelector<HTMLElement>('[data-term="utilize"]');
    expect(decided?.dataset.decided).toBe("true");
  });

  it("answers the desktop's keys: a promotes, d rejects, j and k move", () => {
    const { onPromote, onReject } = review([pending, second]);
    fireEvent.keyDown(window, { key: "j" });
    expect(selectedRow()?.dataset.term).toBe("synergy");
    fireEvent.keyDown(window, { key: "k" });
    expect(selectedRow()?.dataset.term).toBe("utilize");
    fireEvent.keyDown(window, { key: "d" });
    expect(onReject).toHaveBeenCalledWith(pending);
    fireEvent.keyDown(window, { key: "a" });
    expect(onPromote).toHaveBeenCalledWith(second, undefined);
  });

  it("changes the form before promoting it", async () => {
    const { onPromote } = review([pending]);
    fireEvent.keyDown(window, { key: "c" });
    const field = screen.getByRole("textbox", { name: "What to write instead" });
    await userEvent.clear(field);
    await userEvent.type(field, "apply{Enter}");
    expect(onPromote).toHaveBeenCalledWith(pending, "apply");
  });

  it("puts a phrasing corrected two ways under Needs you, each form with its own choice", async () => {
    const employ: CandidateRule = { ...pending, replacement: "employ", correction_count: 2 };
    const { onPromote, onReject } = review([pending, employ]);
    const group = document.querySelector<HTMLElement>('[data-slot="review-conflicts"]')!;
    expect(group).toHaveTextContent("Needs you");
    expect(within(detail()).getByRole("heading")).toHaveTextContent("use or employ for utilize?");
    const choices = within(detail()).getAllByRole("button", { name: "Choose this form" });
    expect(choices).toHaveLength(2);
    await userEvent.click(choices[1]);
    expect(onPromote).toHaveBeenCalledWith(employ, undefined);
    expect(onReject).not.toHaveBeenCalled();
  });

  it("keeps decided rules in closed groups, with who decided and when", async () => {
    review([pending, promoted, rejected]);
    expect(document.querySelector('[data-term="leverage"]')).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Established" }));
    await userEvent.click(document.querySelector<HTMLElement>('[data-term="leverage"]')!);
    expect(within(detail()).queryByRole("button", { name: "Promote" })).toBeNull();
    expect(detail()).toHaveTextContent("Automatically, at the profile's threshold");
    expect(detail()).toHaveTextContent("v3");
    await userEvent.click(screen.getByRole("button", { name: "Rejected" }));
    await userEvent.click(document.querySelector<HTMLElement>('[data-term="disrupt"]')!);
    expect(detail()).toHaveTextContent("Maria Lund");
  });

  it("names the reader's own decision as theirs", async () => {
    review([rejected], { currentUserId: "u-maria" });
    await userEvent.click(screen.getByRole("button", { name: "Rejected" }));
    await userEvent.click(document.querySelector<HTMLElement>('[data-term="disrupt"]')!);
    expect(within(detail()).getByText("You")).toBeInTheDocument();
  });

  it("offers Preview impact only once a project is chosen to evaluate against", async () => {
    const onEvaluate = vi.fn();
    const { rerender } = render(
      <CandidateReview candidates={[pending]} onPromote={vi.fn()} onReject={vi.fn()} />,
    );
    expect(within(detail()).queryByRole("button", { name: "Preview impact" })).toBeNull();
    rerender(
      <CandidateReview
        candidates={[pending]}
        onPromote={vi.fn()}
        onReject={vi.fn()}
        onEvaluate={onEvaluate}
      />,
    );
    await userEvent.click(within(detail()).getByRole("button", { name: "Preview impact" }));
    expect(onEvaluate).toHaveBeenCalledWith(pending);
  });

  it("disables the acts on the rule being decided", () => {
    review([pending], { busyTerm: "utilize" });
    expect(within(detail()).getByRole("button", { name: "Promote" })).toBeDisabled();
  });

  it("puts a failed decision back, selected, with the reason", () => {
    const { rerender } = render(
      <CandidateReview candidates={[pending, second]} onPromote={vi.fn()} onReject={vi.fn()} />,
    );
    fireEvent.keyDown(window, { key: "a" });
    expect(selectedRow()?.dataset.term).toBe("synergy");
    rerender(
      <CandidateReview
        candidates={[pending, second]}
        onPromote={vi.fn()}
        onReject={vi.fn()}
        failed={{ term: "utilize", error: new Error("forbidden") }}
      />,
    );
    expect(selectedRow()?.dataset.term).toBe("utilize");
    expect(detail()).toHaveTextContent("The decision was not recorded");
  });

  it("says what would appear when nothing is suggested", () => {
    review([]);
    expect(screen.getByText("No suggested rules yet")).toBeInTheDocument();
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
