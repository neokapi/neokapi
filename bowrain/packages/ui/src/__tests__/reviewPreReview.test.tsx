/**
 * An agent's pre-review (a decide with outcome advise) reaches the reviewer in
 * two places: the queue row carries its score, and the unit's checks card
 * carries the score, who gave it and the reasons. Neither decides anything.
 */
import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";

import { FocusedReviewer } from "../components/review/FocusedReviewer";
import { ReviewQueueList } from "../components/review/ReviewQueueList";
import type { ReviewEntry } from "../components/review/reviewQueue";
import type { BlockInfo, ReviewContext } from "../types/api";

function block(): BlockInfo {
  return {
    id: "b1",
    source: "Reset your password",
    source_coded: "Reset your password",
    source_spans: [],
    targets: { fr: { text: "Réinitialisez votre mot de passe", status: "translated" } },
    targets_coded: { fr: "Réinitialisez votre mot de passe" },
    translatable: true,
    has_spans: false,
    properties: {},
  };
}

function entry(over: Partial<ReviewEntry> = {}): ReviewEntry {
  return {
    id: "itm-1::b1::fr",
    itemId: "itm-1",
    itemName: "auth.json",
    collectionId: "",
    termCompliance: "",
    locale: "fr",
    block: block(),
    issues: [],
    ...over,
  };
}

describe("an agent's pre-review", () => {
  it("shows its score on the queue row, and nothing on a row nobody advised on", () => {
    render(
      <ReviewQueueList
        entries={[
          entry({ preReview: { score: 72, reviewer: "agent/claude-code", reasons: ["stiff"] } }),
          entry({ id: "itm-1::b2::fr", block: { ...block(), id: "b2" } }),
        ]}
        sourceLocale="en"
        groupBy="item"
        onGroupByChange={() => {}}
        currentId={null}
        onSelect={() => {}}
      />,
    );

    const chip = screen.getByTestId("queue-row-prereview-itm-1::b1::fr");
    expect(chip.textContent).toBe("AI 72");
    expect(chip.getAttribute("title")).toBe("agent/claude-code");
    expect(screen.queryByTestId("queue-row-prereview-itm-1::b2::fr")).toBeNull();
  });

  it("shows the score, the reviewer and the reasons in the unit's checks", () => {
    const context: ReviewContext = {
      block_id: "b1",
      item_name: "auth.json",
      locale: "fr",
      collection_id: "",
      terms: [],
      notes: [],
      point: { default: false, terms_total: 0 },
      neighbourhood: { key: "b1", window: 2 },
      history: {},
      judgement: {
        ai_score: 72,
        ai_model: "agent/claude-code",
        ai_findings: [{ message: "reads as machine output" }],
      },
      provenance: {},
    };
    render(
      <FocusedReviewer
        entry={entry()}
        sourceLocale="en"
        position={{ index: 1, total: 1 }}
        editing={false}
        context={context}
        onApprove={() => {}}
        onReject={() => {}}
        onEditToggle={() => {}}
        onSaveEdit={() => {}}
        onCancelEdit={() => {}}
        onReCheck={() => {}}
        onMarkTerm={() => {}}
        onSuggestVoiceRule={() => {}}
        onMakeRule={() => {}}
      />,
    );

    const checks = screen.getByTestId("reviewer-checks");
    const score = checks.querySelector("[data-slot='review-ai-score']");
    expect(score?.textContent).toBe("72agent/claude-code");
    expect(checks.querySelector("[data-slot='review-ai-finding']")?.textContent).toContain(
      "reads as machine output",
    );
  });
});
