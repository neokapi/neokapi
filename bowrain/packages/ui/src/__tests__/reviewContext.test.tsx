/**
 * The context both review surfaces render for the unit under decision.
 *
 * Two things are asserted here rather than eyeballed. First, a finding that
 * carries a run anchor lands on the words it names: the queue projects it
 * through the same declared projection and the same overlay resolver the
 * document uses, so a reviewer sees the span rather than a sentence about it.
 * Second, every layer draws its own empty state — an ungoverned point, an
 * unmatched block and an undecided unit each say which of those they are, so a
 * surface with nothing resolved cannot be mistaken for one that failed.
 */
import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";

import { FocusedReviewer } from "../components/review/FocusedReviewer";
import { ReviewInspector } from "../components/review/ReviewInspector";
import type { ReviewEntry } from "../components/review/reviewQueue";
import type { BlockInfo, ReviewContext, QAIssue } from "../types/api";

function block(over: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
    source: "Reset your password",
    source_coded: "Reset your password",
    source_spans: [],
    targets: { "fr-FR": { text: "Réinitialisez votre mot de passe", status: "translated" } },
    targets_coded: { "fr-FR": "Réinitialisez votre mot de passe" },
    translatable: true,
    has_spans: false,
    properties: {},
    ...over,
  };
}

function entry(over: Partial<ReviewEntry> = {}): ReviewEntry {
  return {
    id: "itm-1::b1::fr-FR",
    itemId: "itm-1",
    itemName: "auth.json",
    collectionId: "",
    termCompliance: "",
    locale: "fr-FR",
    block: block(),
    issues: [],
    ...over,
  };
}

/** The shape the endpoint returns for a unit nothing has resolved anything for. */
function emptyContext(over: Partial<ReviewContext> = {}): ReviewContext {
  return {
    block_id: "b1",
    item_name: "auth.json",
    locale: "fr-FR",
    terms: [],
    collection_id: "",
    notes: [],
    voice_findings: [],
    ...over,
  };
}

function reviewer(over: Partial<ReviewEntry> = {}, context: ReviewContext | null = null) {
  return render(
    <FocusedReviewer
      entry={entry(over)}
      sourceLocale="en"
      position={{ index: 1, total: 3 }}
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
}

describe("the queue anchors what the checks found", () => {
  it("marks the span a voice finding names, inside the target text", () => {
    reviewer(
      {},
      emptyContext({
        voice_findings: [
          {
            category: "compliance",
            severity: "major",
            message: "Uses a term the profile forbids.",
            original_text: "Réinitialisez",
            suggestion: "Changez",
            position: { kind: "range", start: { run: 0, offset: 0 }, end: { run: 0, offset: 13 } },
          },
        ],
      }),
    );

    const anchored = screen.getByTestId("anchored-target");
    const marks = anchored.querySelectorAll("mark");
    expect(marks).toHaveLength(1);
    expect(marks[0].textContent).toBe("Réinitialisez");
    // The rest of the target is still there, unmarked.
    expect(anchored.textContent).toBe("Réinitialisez votre mot de passe");
  });

  it("marks a positioned check finding on the same text", () => {
    const issue: QAIssue = {
      type: "terminology",
      severity: "error",
      message: "Uses a forbidden rendering.",
      original_text: "mot de passe",
      suggestion: "code secret",
      position: { kind: "range", start: { run: 0, offset: 19 }, end: { run: 0, offset: 31 } },
    };
    reviewer({ issues: [issue] }, emptyContext());

    const marks = screen.getByTestId("anchored-target").querySelectorAll("mark");
    expect([...marks].map((m) => m.textContent)).toEqual(["mot de passe"]);
  });

  it("lists what each finding was raised against and what to say instead", () => {
    reviewer(
      {
        issues: [
          {
            type: "terminology",
            severity: "error",
            message: "Uses a forbidden rendering.",
            original_text: "mot de passe",
            suggestion: "code secret",
          },
        ],
      },
      emptyContext({
        voice_findings: [
          {
            category: "compliance",
            severity: "major",
            message: "Uses a term the profile forbids.",
            original_text: "Réinitialisez",
            suggestion: "Changez",
          },
        ],
      }),
    );

    expect(screen.getByTestId("finding-issue-0-original").textContent).toBe("mot de passe");
    expect(screen.getByTestId("finding-issue-0-suggestion").textContent).toBe("code secret");
    expect(screen.getByTestId("finding-voice-0-original").textContent).toBe("Réinitialisez");
    expect(screen.getByTestId("finding-voice-0-suggestion").textContent).toBe("Changez");
  });

  it("draws no anchored text when nothing was flagged", () => {
    reviewer({}, emptyContext());
    expect(screen.queryByTestId("anchored-target")).toBeNull();
    expect(screen.getByTestId("findings-none")).toBeTruthy();
  });
});

describe("the queue's neighbourhood", () => {
  it("renders a neighbour's placeholder as a chip rather than dropping it", () => {
    reviewer(
      {},
      emptyContext({
        previous: {
          block_id: "b0",
          source_runs: [
            { text: "We sent a link to " },
            { ph: { id: "1", type: "code:variable", data: "{{.Email}}", equiv: "{{.Email}}" } },
            { text: "." },
          ],
        },
      }),
    );

    const previous = screen.getByTestId("neighbour-previous");
    expect(previous.textContent).toContain("We sent a link to ");
    // The projection answers for the placeholder kind; a text-only loop would
    // have rendered "We sent a link to ." and lost the variable.
    expect(previous.textContent).toContain("{{.Email}}");
  });

  it("says where the item ends rather than showing an empty box", () => {
    reviewer({}, emptyContext());
    expect(screen.getByTestId("neighbour-previous").textContent).toContain("Start of the item.");
    expect(screen.getByTestId("neighbour-next").textContent).toContain("End of the item.");
  });
});

describe("the queue's empty states", () => {
  it("names an ungoverned point, an unmatched block and an undecided unit", () => {
    reviewer({}, emptyContext());

    expect(screen.getByTestId("review-point").textContent).toContain(
      "No voice profile is bound at this point.",
    );
    expect(screen.getByTestId("review-point").textContent).toContain(
      "No terms matched this block.",
    );
    expect(screen.getByTestId("reviewer-memory").textContent).toContain(
      "No content-memory match for this block.",
    );
    expect(screen.getByTestId("reviewer-provenance").textContent).toContain(
      "No decision recorded, and no provenance stamped.",
    );
  });

  it("shows the profile by name with its guidance and the rules in force", () => {
    reviewer(
      {},
      emptyContext({
        voice_profile: {
          id: "vp-1",
          name: "Bowrain Voice",
          guidance: "Say what the product does for the reader.",
          compliance_bar: 90,
          term_rules: [{ term: "leverage", replacement: "use", severity: "major" }],
        },
      }),
    );

    expect(screen.getByTestId("point-profile-name").textContent).toBe("Bowrain Voice");
    expect(screen.getByTestId("point-guidance").textContent).toContain(
      "Say what the product does for the reader.",
    );
    const rules = screen.getByTestId("point-term-rules");
    expect(rules.textContent).toContain("leverage");
    expect(rules.textContent).toContain("use");
  });

  it("shows the memory match on both sides, with who decided the unit and when", () => {
    reviewer(
      {},
      emptyContext({
        memory_match: {
          source: "Reset your password",
          target: "Changez votre mot de passe",
          score: 1,
          match_type: "exact",
        },
        decision: {
          state: "rejected",
          by: "maria@bowrain.test",
          at: "2026-08-30T09:12:00Z",
          note: "Reads as machine output.",
        },
        origin: { kind: "ai", engine: "claude-sonnet" },
      }),
    );

    expect(screen.getByTestId("memory-match-source").textContent).toBe("Reset your password");
    expect(screen.getByTestId("memory-match-target").textContent).toBe(
      "Changez votre mot de passe",
    );
    const provenance = screen.getByTestId("review-provenance").textContent ?? "";
    expect(provenance).toContain("AI translation");
    expect(provenance).toContain("Rejected");
    expect(provenance).toContain("maria@bowrain.test");
    expect(provenance).toContain("Reads as machine output.");
  });
});

function inspector(context: ReviewContext | null) {
  return render(
    <ReviewInspector
      block={block()}
      node={null}
      itemName="auth.json"
      locale="fr-FR"
      localeLabel="French (France) (fr-FR)"
      issues={[]}
      terms={[]}
      context={context}
      editing={false}
      marked={false}
      onClose={() => {}}
      onApprove={() => {}}
      onReject={() => {}}
      onEditToggle={() => {}}
      onSaveEdit={() => {}}
      onCancelEdit={() => {}}
      onToggleMark={() => {}}
    />,
  );
}

describe("the document's inspector", () => {
  it("names an unmatched block and an undecided unit", () => {
    inspector(emptyContext());

    expect(screen.getByTestId("inspector-memory").textContent).toContain(
      "No content-memory match for this block.",
    );
    expect(screen.getByTestId("inspector-provenance").textContent).toContain(
      "No decision recorded, and no provenance stamped.",
    );
  });

  it("shows the memory wording, the origin, the decision and the note", () => {
    inspector(
      emptyContext({
        memory_match: {
          source: "Reset your password",
          target: "Changez votre mot de passe",
          score: 0.92,
          match_type: "fuzzy",
        },
        decision: { state: "approved", by: "sam@bowrain.test", at: "2026-08-31T08:00:00Z" },
        notes: [
          {
            id: "n1",
            blockId: "b1",
            author: "sam@bowrain.test",
            text: "Legal signed off on this wording.",
            createdAt: "2026-08-31T08:01:00Z",
          },
        ],
        origin: { kind: "memory", reference: "entry-42" },
      }),
    );

    expect(screen.getByTestId("memory-match-target").textContent).toBe(
      "Changez votre mot de passe",
    );
    const provenance = screen.getByTestId("review-provenance").textContent ?? "";
    expect(provenance).toContain("Recycled from content memory");
    expect(provenance).toContain("Approved");
    expect(provenance).toContain("sam@bowrain.test");
    expect(provenance).toContain("Legal signed off on this wording.");
  });

  it("shows the voice findings the score was made of", () => {
    inspector(
      emptyContext({
        voice_findings: [
          {
            category: "compliance",
            severity: "major",
            message: "Uses a term the profile forbids.",
            original_text: "Réinitialisez",
            suggestion: "Changez",
          },
        ],
      }),
    );

    expect(screen.getByTestId("finding-voice-0").textContent).toContain(
      "Uses a term the profile forbids.",
    );
    expect(screen.getByTestId("finding-voice-0-suggestion").textContent).toBe("Changez");
  });
});
