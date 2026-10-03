import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReviewPage } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { ReviewContext, ReviewItem, ReviewUnitDetail } from "../types/api";
import { waitFor } from "storybook/test";
import { queueChanges } from "./memoryChanges";

/** A date placeholder, the kind a concatenating run walk deletes silently. */
const DATE_PH = { id: "1", type: "var", data: "{date}", equiv: "date" };

const meta: Meta<typeof ReviewPage> = {
  title: "Project/ReviewPage",
  component: ReviewPage,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <ErrorProvider>
        <div className="h-screen">
          <Story />
        </div>
      </ErrorProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ReviewPage>;

const QUEUE: ReviewItem[] = [
  {
    locale: "nb",
    file: "locales/nb.json",
    key: "hero.title",
    collection: "Marketing",
    source: "Ship every language without the toil",
    target: "Send lokalisert innhold uten slitet",
    hasFindings: false,
  },
  {
    locale: "nb",
    file: "locales/nb.json",
    key: "cta.primary",
    collection: "Marketing",
    source: "Get started with {product}",
    target: "Kom i gang",
    hasFindings: true,
  },
  {
    locale: "nb",
    file: "docs/guide/nb/intro.md",
    key: "intro.p1",
    collection: "Docs",
    source: "KapiMart is a sample storefront used across the documentation.",
    target: "KapiMart er en eksempelbutikk brukt i dokumentasjonen.",
    hasFindings: false,
  },
  {
    locale: "de-DE",
    file: "locales/de-DE.json",
    key: "hero.title",
    collection: "Marketing",
    source: "Ship every language without the toil",
    target: "Lokalisierte Inhalte ohne Mühsal ausliefern",
    hasFindings: false,
  },
  {
    locale: "de-DE",
    file: "locales/de-DE.json",
    key: "cta.primary",
    collection: "Marketing",
    source: "Get started with {product}",
    target: "Loslegen mit {product}",
    hasFindings: false,
  },
];

const DETAILS: Record<string, Partial<ReviewUnitDetail>> = {
  "nb:cta.primary": {
    status: "translated",
    origin: { kind: "mt", engine: "deepl" },
    memory_score: 78,
    findings: [
      {
        category: "placeholder",
        severity: "major",
        message: "placeholder {product} missing from target",
        suggestion: "Carry {product} into the translation verbatim",
      },
    ],
  },
  "nb:hero.title": {
    status: "translated",
    origin: { kind: "ai", engine: "claude" },
    memory_score: 92,
  },
  "de-DE:hero.title": {
    status: "established",
    review_state: "approved",
    origin: { kind: "human" },
  },
  "nb:intro.p1": {
    status: "draft",
    review_state: "rejected",
    note: "Too literal — KapiMart is a brand name, keep the English framing.",
  },
};

async function loadUnit(item: ReviewItem): Promise<ReviewUnitDetail> {
  const extra = DETAILS[`${item.locale}:${item.key}`] ?? {};
  return {
    locale: item.locale,
    file: item.file,
    key: item.key,
    collection: item.collection,
    source: item.source,
    target: item.target ?? "",
    status: "translated",
    findings: [],
    ...extra,
  };
}

export const Queue: Story = {
  args: {
    tabID: "storybook",
    items: QUEUE,
    loadUnit,
    changes: queueChanges(QUEUE),
  },
};

export const ScopedToCollection: Story = {
  args: {
    tabID: "storybook",
    items: QUEUE,
    scope: { collection: "Marketing", locale: "nb" },
    loadUnit,
    changes: queueChanges(QUEUE),
  },
};

export const Empty: Story = {
  args: {
    tabID: "storybook",
    items: [],
    loadUnit,
  },
};

// A realistically large queue: hundreds of units across many files and locales.
// The left pane virtualizes (only the rows near the viewport mount) inside a
// bounded, scroll-contained container with a per-file header, so the page never
// becomes a thousands-row scroll. Used for the mid-scroll sticky-header shot.
const LARGE_QUEUE: ReviewItem[] = (() => {
  const locales = ["nb", "de-DE", "fr-FR", "ja-JP", "es-ES", "pt-BR"];
  const out: ReviewItem[] = [];
  for (let f = 0; f < 40; f++) {
    const file = `locales/section-${String(f).padStart(2, "0")}.json`;
    for (let k = 0; k < 12; k++) {
      const locale = locales[(f + k) % locales.length];
      out.push({
        locale,
        file,
        key: `section${f}.item.${k}`,
        collection: f % 2 === 0 ? "Marketing" : "Docs",
        source: `Source string ${f}.${k} — ship every language without the toil`,
        target: `Target ${f}.${k}`,
        hasFindings: (f + k) % 5 === 0,
      });
    }
  }
  return out;
})();

export const LargeQueue: Story = {
  name: "Large queue (virtualized, contained)",
  args: {
    tabID: "storybook",
    items: LARGE_QUEUE,
    loadUnit,
    changes: queueChanges(LARGE_QUEUE),
  },
};

// One unit's review model, so the five layer cards draw what the reviewer is
// shown at a glance: the point, the neighbourhood, what was approved before,
// what the checks said, and where the wording came from.
const CONTEXT: ReviewContext = {
  point: {
    path: "content/marketing.json",
    profile: "retail",
    channel: "web",
    collection: "Marketing",
    ref: "retail/web",
    default: false,
    coordinates: { product: "kapimart", channel: "web", brand: "northsea" },
    voice: {
      name: "Kapimart retail",
      source: "pack:retail",
      field: "defaults.voice",
      guide: "Write in the second person. Keep sentences under twenty words.",
    },
    // Context, not findings. Most of these come from the terms store and carry
    // no severity, which is why the card draws them all in one neutral chip.
    term_rules: [
      { term: "cart", replacement: "basket", severity: "major" },
      { term: "sign in", replacement: "log in", severity: "minor" },
      { term: "checkout", replacement: "pay" },
      { term: "Kapimart", do_not_translate: true },
    ],
    terms_total: 40,
    profiles: [{ name: "retail", valid_from: "2026-01-01", state: "active" }],
    notes: ["The terms store was read three days ago."],
  },
  neighbourhood: {
    key: "cta.primary",
    before: [{ key: "hero.title", source: [{ text: "Ship every language without the toil" }] }],
    after: [
      {
        key: "cta.note",
        source: [{ text: "Your credits reset on " }, { ph: DATE_PH }, { text: "." }],
        target: [{ text: "Kredittene tilbakestilles " }, { ph: DATE_PH }, { text: "." }],
      },
    ],
    window: 2,
  },
  history: {
    prior: {
      source: "Get started with {product}",
      target: "Kom i gang med {product}",
      context_fingerprint: "abc123",
      governed: false,
    },
    match: { score: 88, source: "Get started with {product}", target: "Kom i gang med {product}" },
  },
  judgement: {
    ai_score: 74,
    ai_model: "claude-x",
    ai_findings: [{ severity: "minor", message: "The call to action is softer than the source." }],
  },
  provenance: {
    origin: { kind: "mt", engine: "deepl" },
    review_state: "rejected",
    by: "agent/desktop",
    at: "2026-08-30T10:00:00Z",
    note: "Too literal for this surface.",
    stale: true,
  },
};

async function loadUnitWithContext(item: ReviewItem): Promise<ReviewUnitDetail> {
  return { ...(await loadUnit(item)), context: CONTEXT };
}

/** The source rows: the author's own wording translate_after is holding. */
const SOURCE_ROWS: ReviewItem[] = [
  {
    locale: "en-US",
    language: "en-US",
    isSource: true,
    file: "locales/en-US.json",
    relative: "locales/en-US.json",
    key: "hero.title",
    collection: "Marketing",
    sourceLocale: "en-US",
    source: "Ship every language without the toil",
    status: "written",
    held: true,
  },
  {
    locale: "en-US",
    language: "en-US",
    isSource: true,
    file: "locales/en-US.json",
    relative: "locales/en-US.json",
    key: "cta.primary",
    collection: "Marketing",
    sourceLocale: "en-US",
    source: "Get started with {product}",
    status: "written",
    held: true,
  },
];

/** Every language the project has work in, the source language among them. */
const UNIFIED_QUEUE: ReviewItem[] = [
  ...SOURCE_ROWS,
  ...QUEUE.map((it) => ({ ...it, language: it.locale, sourceLocale: "en-US" })),
];

/**
 * The review model for a source unit, as GetSourceUnitContext assembles it.
 *
 * Source review and target review render one model, so the point card reads the
 * same either way. Without this the source pane draws "No point resolved",
 * which is the loader missing rather than the project having no point.
 */
async function loadSourceContext(item: ReviewItem): Promise<ReviewContext> {
  return {
    ...CONTEXT,
    neighbourhood: {
      key: item.key,
      before: [{ key: "nav.home", source: [{ text: "Home" }] }],
      after: [{ key: "cta.note", source: [{ text: "Your credits reset on " }, { ph: DATE_PH }] }],
      window: 2,
    },
    history: {},
    judgement: {},
    provenance: {},
  };
}

const unifiedArgs = {
  tabID: "storybook",
  items: UNIFIED_QUEUE,
  loadUnit: loadUnitWithContext,
  loadSourceContext,
  changes: queueChanges(UNIFIED_QUEUE),
};

export const AllLanguages: Story = {
  name: "All languages",
  args: unifiedArgs,
};

export const OneTargetLanguage: Story = {
  name: "One target language",
  args: { ...unifiedArgs, scope: { locale: "nb" } },
};

export const SourceLanguage: Story = {
  name: "The source language",
  args: { ...unifiedArgs, scope: { locale: "en-US" } },
};

/**
 * The five layer cards open, each headed by the line a reviewer scans.
 *
 * The Point card is the one to read for tone: every bound term rule is neutral
 * context, and the severities belong to the Checks card below it.
 */
export const LayersExpanded: Story = {
  name: "Layer cards expanded",
  args: { ...unifiedArgs, scope: { locale: "nb", collection: "Marketing" } },
};

/** The same page with every layer folded to its summary line. */
export const LayersCollapsed: Story = {
  name: "Layer cards collapsed",
  args: LayersExpanded.args,
  play: async ({ canvasElement }) => {
    const toggles = canvasElement.querySelectorAll<HTMLButtonElement>(
      "[data-slot$='-toggle'][aria-expanded='true']",
    );
    for (const toggle of toggles) toggle.click();
  },
};

/**
 * A translation with inline codes in the editor: each code is a chip the
 * reviewer types beside, and a save sends the text with the codes it read.
 */
export const FormattedEdit: Story = {
  name: "Edit: formatted translation",
  args: {
    tabID: "storybook",
    items: QUEUE,
    scope: { locale: "de-DE" },
    loadUnit,
    changes: queueChanges(QUEUE, {
      "de-DE:hero.title": {
        text: 'Lokalisierte Inhalte <x id="1"/>ohne Mühsal<x id="/1"/> ausliefern',
        codes: { "1": { kind: "paired", type: "fmt:bold" } },
      },
    }),
  },
};

/** A plural translation, edited a form at a time; each changed form is saved by its path. */
export const PluralEdit: Story = {
  name: "Edit: plural translation",
  args: {
    tabID: "storybook",
    items: QUEUE,
    scope: { locale: "de-DE" },
    loadUnit,
    changes: queueChanges(QUEUE, {
      "de-DE:hero.title": {
        text: '<x id="n/"/> Artikel im Warenkorb',
        codes: { "n/": { kind: "placeholder", type: "jsx:var", equiv: "count" } },
        structures: [
          {
            path: [0],
            kind: "plural",
            pivot: "count",
            branches: {
              one: '<x id="n/"/> Artikel im Warenkorb',
              other: '<x id="n/"/> Artikel im Warenkorb',
            },
          },
        ],
      },
    }),
  },
};

/**
 * Another writer saved the translation after the page read it. Edit it and
 * save: nothing is written, and the editor shows the text as it stands and asks
 * before applying the edit over it.
 */
export const StaleOnSave: Story = {
  name: "Edit: changed since it was opened",
  args: {
    tabID: "storybook",
    items: QUEUE,
    scope: { locale: "de-DE" },
    loadUnit,
    changes: (() => {
      const changes = queueChanges(QUEUE);
      const read = changes.read.bind(changes);
      let reads = 0;
      changes.read = async (request) => {
        const page = await read(request);
        if (++reads === 1) {
          changes.touch("locales/de-DE.json", "hero.title", "Lokalisierte Inhalte, ohne Mühe");
        }
        return page;
      };
      return changes;
    })(),
  },
};

/** The unit's recorded changes, open under its provenance. */
export const RecordedChanges: Story = {
  name: "Recorded changes",
  args: {
    tabID: "storybook",
    items: QUEUE,
    scope: { locale: "de-DE" },
    loadUnit,
    changes: queueChanges(
      QUEUE,
      {},
      {
        [JSON.stringify({ doc: "locales/en-US.json", block: "hero.title", edition: "de-DE" })]: [
          {
            record: "op-3",
            before: "r:1111111111111111",
            after: "r:2222222222222222",
            actor: { kind: "person", name: "Ingrid" },
            origin: "desktop",
            at: "2026-10-02T14:12:00Z",
          },
          {
            record: "op-2",
            before: "absent",
            after: "r:1111111111111111",
            basis: "r:0000000000000000",
            actor: { kind: "tool", name: "translate" },
            origin: "flow:up",
            at: "2026-10-01T09:30:00Z",
          },
        ],
      },
    ),
  },
  play: async ({ canvasElement }) => {
    const toggle = await waitFor(() => {
      const el = canvasElement.querySelector<HTMLButtonElement>(
        "[data-slot='review-changes-toggle']",
      );
      if (!el) throw new Error("the changes card has not drawn yet");
      return el;
    });
    if (toggle.getAttribute("aria-expanded") !== "true") toggle.click();
  },
};
