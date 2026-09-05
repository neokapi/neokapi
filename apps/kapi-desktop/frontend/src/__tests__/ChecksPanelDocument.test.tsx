import { render, screen, within, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
import { ChecksPanel } from "../components/ChecksPanel";
import { ErrorProvider } from "../components/ErrorBanner";
import { findingHighlights, findingSide } from "../lib/findingHighlights";
import type { CheckRunResult, DesktopFinding } from "../types/api";

// The file the findings were raised on, as InspectFileAnnotated returns it. The
// blocks are named by the reader (the key path), which is how the preview
// addresses a unit; the findings name them by id.
const tree: ContentTree = {
  format: "json",
  root: [
    {
      kind: "block",
      id: "b1",
      name: "greeting",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Please utilize the dashboard" }],
      targets: { de: [{ text: "Bitte nutzen Sie das Dashboard" }] },
    },
    {
      kind: "block",
      id: "b2",
      name: "welcome",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [
        { text: "Hello " },
        { ph: { id: "name", type: "var", data: "{name}", equiv: "name" } },
      ],
      targets: { de: [{ text: "Hallo" }] },
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 2, data: 0, media: 0, runs: 3 },
};

const vocab: DesktopFinding = {
  category: "vocabulary",
  severity: "major",
  message: 'Forbidden term "utilize" found',
  suggestion: 'Use "use" instead',
  original_text: "utilize",
  replacement: "use",
  block_id: "b1",
  field: "source",
  locale: "en",
  fixable: true,
  position: { kind: "range", start: { run: 0, offset: 7 }, end: { run: 0, offset: 14 } },
  source_runs: [{ text: "Please utilize the dashboard" }],
};

const dnt: DesktopFinding = {
  category: "do-not-translate",
  severity: "critical",
  message: 'Do-not-translate term "dashboard" is missing from the de target',
  original_text: "dashboard",
  block_id: "b1",
  field: "target",
  locale: "de",
  fixable: false,
  position: { kind: "range", start: { run: 0, offset: 19 }, end: { run: 1 } },
  source_runs: [{ text: "Please utilize the dashboard" }],
  target_runs: [{ text: "Bitte nutzen Sie das Dashboard" }],
};

const placeholder: DesktopFinding = {
  category: "placeholder",
  severity: "critical",
  message: "Placeholder {name} is missing from the de target",
  original_text: "{name}",
  block_id: "b2",
  field: "target",
  locale: "de",
  fixable: false,
  source_runs: [
    { text: "Hello " },
    { ph: { id: "name", type: "var", data: "{name}", equiv: "name" } },
  ],
  target_runs: [{ text: "Hallo" }],
};

const quotedOnly: DesktopFinding = {
  category: "register",
  severity: "neutral",
  message: "Tone reads more formal than the brand's register",
  original_text: "Please",
  block_id: "b1",
  field: "source",
  fixable: false,
};

const result: CheckRunResult = {
  pass: false,
  score: 40,
  files: [{ path: "/p/locales/en.json", findings: [dnt, placeholder, vocab, quotedOnly] }],
};

function renderPanel() {
  return render(
    <ErrorProvider>
      <ChecksPanel tabID="t1" result={result} previewTree={tree} />
    </ErrorProvider>,
  );
}

const marks = (root: ParentNode) =>
  Array.from(root.querySelectorAll<HTMLElement>('mark[data-overlay-type="finding"]'));

describe("ChecksPanel: the finding in context", () => {
  it("reads each finding in the text it was raised on, with its span marked", () => {
    renderPanel();
    const cards = screen.getAllByTestId("finding-card");
    // The backend's order is kept: the DNT finding first.
    const dntCard = cards[0];
    const dntSnippet = within(dntCard).getByTestId("finding-snippet");
    expect(dntSnippet).toHaveTextContent("Bitte nutzen Sie das Dashboard");
    expect(marks(dntSnippet)).toHaveLength(0);
    // Its position is anchored to the source, so the source follows with the
    // words underlined.
    const dntSource = within(dntCard).getByTestId("finding-snippet-source");
    expect(marks(dntSource)[0]).toHaveTextContent("dashboard");

    const placeholderCard = cards[1];
    expect(within(placeholderCard).getByTestId("finding-snippet")).toHaveTextContent("Hallo");
    expect(within(placeholderCard).queryByTestId("finding-snippet-source")).toBeNull();

    const vocabCard = cards[2];
    const vocabSnippet = within(vocabCard).getByTestId("finding-snippet");
    expect(vocabSnippet).toHaveTextContent("Please utilize the dashboard");
    expect(marks(vocabSnippet)[0]).toHaveTextContent("utilize");
    expect(marks(vocabSnippet)[0].className).toContain("decoration-destructive");
  });

  it("falls back to the quoted text when the block's runs did not travel", () => {
    renderPanel();
    const card = screen.getAllByTestId("finding-card")[3];
    const snippet = within(card).getByTestId("finding-snippet");
    expect(snippet.getAttribute("data-snippet")).toBe("fallback");
    expect(snippet).toHaveTextContent("Please");
  });

  it("opens the finding's document at its block, on its side, with the span in focus", async () => {
    renderPanel();
    const vocabCard = screen.getAllByTestId("finding-card")[2];
    await userEvent.click(within(vocabCard).getByRole("button", { name: /Open in document/ }));

    const sheet = await screen.findByRole("dialog");
    const focusRow = sheet.querySelector('[data-slot="file-preview-focus"]') as HTMLElement;
    // The block is addressed by its unit key, mapped from the finding's block id.
    expect(within(focusRow).getByText("greeting")).toBeInTheDocument();
    expect(within(focusRow).getByText('Forbidden term "utilize" found')).toBeInTheDocument();
    expect(within(focusRow).getByRole("button", { name: /Back to checks/ })).toBeInTheDocument();

    const focus = marks(sheet).filter((m) => m.getAttribute("data-emphasis") === "focus");
    expect(focus.map((m) => m.textContent)).toEqual(["utilize"]);
    // The file's other findings are drawn dimmer alongside it.
    const dim = marks(sheet).filter((m) => m.getAttribute("data-emphasis") === "dim");
    expect(dim.length).toBeGreaterThan(0);
    expect(dim.some((m) => m.textContent === "dashboard")).toBe(true);

    // Back returns to the list, which stayed mounted behind the sheet.
    await userEvent.click(within(focusRow).getByRole("button", { name: /Back to checks/ }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getAllByTestId("finding-card")).toHaveLength(4);
  });

  it("opens a finding about the translation on its locale, with the block marked whole", async () => {
    renderPanel();
    const dntCard = screen.getAllByTestId("finding-card")[0];
    await userEvent.click(within(dntCard).getByRole("button", { name: /Open in document/ }));
    const sheet = await screen.findByRole("dialog");
    const focus = marks(sheet).filter((m) => m.getAttribute("data-emphasis") === "focus");
    // The target block is marked whole on the de side, and the source span is
    // marked where the checker anchored it.
    expect(focus.map((m) => m.textContent).sort()).toEqual([
      "Bitte nutzen Sie das Dashboard",
      "dashboard",
    ]);
  });

  it("opens the whole file from its heading with every finding drawn plainly", async () => {
    renderPanel();
    await userEvent.click(screen.getByTestId("file-open-document"));
    const sheet = await screen.findByRole("dialog");
    const focusRow = sheet.querySelector('[data-slot="file-preview-focus"]') as HTMLElement;
    expect(focusRow).toHaveTextContent("4 findings");
    expect(within(focusRow).getByRole("button", { name: /Back to checks/ })).toBeInTheDocument();
    const all = marks(sheet);
    expect(all.length).toBeGreaterThan(0);
    expect(all.every((m) => !m.hasAttribute("data-emphasis"))).toBe(true);
  });
});

describe("findingHighlights", () => {
  it("draws a source finding's span on the source, in focus, and the rest dimmed", () => {
    const out = findingHighlights([vocab, dnt, placeholder], vocab);
    expect(out.b1).toEqual([
      {
        side: "source",
        anchor: vocab.position,
        tone: "destructive",
        label: vocab.message,
        emphasis: "focus",
      },
      {
        side: "de",
        anchor: { kind: "block" },
        tone: "destructive",
        label: dnt.message,
        emphasis: "dim",
      },
      {
        side: "source",
        anchor: dnt.position,
        tone: "destructive",
        label: dnt.message,
        emphasis: "dim",
      },
    ]);
    // A target finding with no position marks its block whole on its locale.
    expect(out.b2).toEqual([
      {
        side: "de",
        anchor: { kind: "block" },
        tone: "destructive",
        label: placeholder.message,
        emphasis: "dim",
      },
    ]);
  });

  it("marks a source finding with no position on its block, and skips a finding with no block", () => {
    const out = findingHighlights(
      [quotedOnly, { ...quotedOnly, block_id: undefined, message: "unreadable" }],
      null,
    );
    expect(Object.keys(out)).toEqual(["b1"]);
    expect(out.b1).toEqual([
      { side: "source", anchor: { kind: "block" }, tone: "muted", label: quotedOnly.message },
    ]);
  });

  it("names the side a finding opens on", () => {
    expect(findingSide(vocab)).toBe("source");
    expect(findingSide(dnt)).toBe("de");
    expect(findingSide({ ...dnt, locale: undefined })).toBe("source");
  });
});
