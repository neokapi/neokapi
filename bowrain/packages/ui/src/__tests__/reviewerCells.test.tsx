/**
 * The focused reviewer's two cells.
 *
 * Source and target are read against each other, so they are rendered by one
 * primitive in one frame: a reviewer comparing them must be seeing a difference
 * in the content, never in the rendering. The source used to go through the
 * document-preview kit, which paints its own paper and drops inline codes — so
 * a block whose target showed a `var` chip showed a *gap* on the source side,
 * in a different type, on a differently-tinted card.
 */
import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "@testing-library/react";

import { FocusedReviewer } from "../components/review/FocusedReviewer";
import type { ReviewEntry } from "../components/review/reviewQueue";
import type { BlockInfo, SpanInfo } from "../types/api";

/** The coded-text marker standing for a placeholder span (model.MarkerPlaceholder). */
const PH = "\uE003";

const resetDate: SpanInfo = {
  span_type: "placeholder",
  type: "code:variable",
  id: "1",
  data: "{{.ResetDate}}",
  equiv_text: "{{.ResetDate}}",
};

function block(over: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
    // The plain projection carries no marker; the coded text is where the
    // placeholder is located.
    source: "Your credits reset on .",
    source_coded: `Your credits reset on ${PH}.`,
    source_spans: [resetDate],
    targets: { nb: { text: "Kredittene tilbakestilles .", status: "translated" } },
    targets_coded: { nb: `Kredittene tilbakestilles ${PH}.` },
    translatable: true,
    has_spans: true,
    properties: {},
    ...over,
  };
}

function entry(over: Partial<ReviewEntry> = {}): ReviewEntry {
  return {
    id: "itm-1::b1::nb",
    itemId: "itm-1",
    itemName: "credits-exhausted.kbf.json",
    locale: "nb",
    block: block(),
    issues: [],
    ...over,
  };
}

function reviewer(over: Partial<ReviewEntry> = {}) {
  return render(
    <FocusedReviewer
      entry={entry(over)}
      sourceLocale="en"
      position={{ index: 1, total: 3 }}
      editing={false}
      onApprove={() => {}}
      onSignOff={() => {}}
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

function chips(el: HTMLElement): string[] {
  return [...el.querySelectorAll("[data-inline-code]")].map((c) => c.textContent ?? "");
}

describe("FocusedReviewer — source and target cells", () => {
  it("shows the source's placeholder as the same chip the target shows", () => {
    reviewer();

    expect(chips(screen.getByTestId("reviewer-source"))).toEqual(["var"]);
    expect(chips(screen.getByTestId("reviewer-target"))).toEqual(["var"]);
  });

  it("frames both sides identically", () => {
    reviewer();

    expect(screen.getByTestId("reviewer-source").className).toBe(
      screen.getByTestId("reviewer-target").className,
    );
  });

  it("keeps the literal text around the placeholder intact", () => {
    reviewer();

    const source = screen.getByTestId("reviewer-source");
    expect(source.textContent).toContain("Your credits reset on ");
    // The marker character itself never reaches the reading.
    expect(source.textContent).not.toContain(PH);
  });

  it("writes each side in its own direction", () => {
    reviewer({
      id: "itm-1::b1::ar",
      locale: "ar",
      block: block({
        targets: { ar: { text: "أعد التعيين", status: "translated" } },
        targets_coded: { ar: "أعد التعيين" },
      }),
    });

    expect(screen.getByTestId("reviewer-source").getAttribute("dir")).toBe("ltr");
    expect(screen.getByTestId("reviewer-target").getAttribute("dir")).toBe("rtl");
  });

  it("marks an entity the server positioned by byte offset", () => {
    // "Café résumé " is 12 characters but 15 bytes, so a string-index reading
    // of the offsets would underline three characters short of the entity.
    reviewer({
      block: block({
        source: "Café résumé password",
        source_coded: "Café résumé password",
        source_spans: [],
        has_spans: false,
        entities: [
          {
            key: "entity:0",
            text: "password",
            type: "entity:product",
            start: 15,
            end: 23,
            dnt: false,
          },
        ],
      }),
    });

    const marked = screen
      .getByTestId("reviewer-source")
      .querySelector("[data-entity-key='entity:0']");
    expect(marked?.textContent).toBe("password");
  });
});
