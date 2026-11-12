import { describe, it, expect, vi } from "vite-plus/test";
import { render, waitFor } from "./testUtils";

import { ReviewPage } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { ReviewContext, ReviewItem, ReviewUnitDetail } from "../types/api";
import { queueChanges } from "../stories/memoryChanges";

/**
 * An i18next interpolation as the json reader marks it: a placeholder run of
 * the code finder's generic type, whose only text is its native form.
 */
const NAME_PH = { id: "c1", type: "code", data: "{{name}}", equiv: "" };

const ITEM: ReviewItem = {
  locale: "nb",
  file: "locales/nb.json",
  key: "/cordonFailed",
  collection: "Glossary",
  source: "Failed to cordon node {{name}}",
  target: "Kunne ikke sperre node {{name}} for planlegging.",
  source_locale: "en-US",
  has_findings: false,
};

const UNIT: ReviewUnitDetail = {
  locale: "nb",
  file: ITEM.file,
  key: ITEM.key,
  collection: ITEM.collection,
  source: ITEM.source,
  target: ITEM.target ?? "",
  source_runs: [{ text: "Failed to cordon node " }, { ph: NAME_PH }],
  target_runs: [
    { text: "Kunne ikke sperre node " },
    { ph: NAME_PH },
    { text: " for planlegging." },
  ],
  source_locale: "en-US",
  status: "translated",
  findings: [],
  context: {
    point: { collection: "Glossary", path: "locales/en.json" },
    neighbourhood: {
      key: "cordonFailed",
      before: [
        {
          key: "/cordoned",
          source: [{ text: "Cordoned node " }, { ph: NAME_PH }, { text: "." }],
          target: [{ text: "Sperret node " }, { ph: NAME_PH }, { text: " for planlegging." }],
        },
      ],
      window: 2,
    },
    history: {},
    judgement: {},
    provenance: {},
  } as ReviewContext,
};

function renderPage() {
  const changes = queueChanges([ITEM], {
    "nb:/cordonFailed": {
      text: 'Kunne ikke sperre node <x id="c1/"/> for planlegging.',
      codes: { "c1/": { kind: "placeholder", type: "code" } },
      editions: { "en-US": { text: 'Failed to cordon node <x id="c1/"/>' } },
    },
  });
  const loadUnit = vi.fn(async () => UNIT);
  return render(
    <ErrorProvider>
      <ReviewPage tabID="t1" items={[ITEM]} loadUnit={loadUnit} changes={changes} />
    </ErrorProvider>,
  );
}

const text = (sel: string) => document.querySelector(sel)?.textContent ?? "";

describe("ReviewPage: a unit with an i18next placeholder", () => {
  it("draws the placeholder in the queue row, the header and its own row in the document", async () => {
    renderPage();
    await waitFor(() =>
      expect(text("[data-slot='review-source']")).toBe("Failed to cordon node {{name}}"),
    );
    expect(text("[data-slot='review-queue-item']")).toContain("Failed to cordon node {{name}}");

    const unitRow = text("[data-slot='review-neighbour-unit']");
    expect(unitRow).toContain("Failed to cordon node {{name}}");
    expect(unitRow).toContain("Kunne ikke sperre node {{name}} for planlegging.");
    // The neighbour already read through the projection; the unit now matches it.
    expect(text("[data-slot='review-neighbour']")).toContain("Cordoned node {{name}}.");
  });

  it("labels the editor's chip with the placeholder's own text", async () => {
    renderPage();
    const editor = await waitFor(() => {
      const el = document.querySelector<HTMLElement>(
        "[data-slot='review-target-editor'] [contenteditable='true']",
      );
      expect(el).not.toBeNull();
      return el!;
    });
    await waitFor(() => expect(editor.textContent).toContain("{{name}}"));
    expect(editor.textContent).not.toMatch(/\bcode\b/);
  });
});
