import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReviewPage } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { ReviewItem, ReviewUnitDetail } from "../types/api";

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
    tm_score: 78,
    findings: [
      {
        category: "placeholder",
        severity: "major",
        message: "placeholder {product} missing from target",
        suggestion: "Carry {product} into the translation verbatim",
        fixable: false,
      },
    ],
  },
  "nb:hero.title": {
    status: "translated",
    origin: { kind: "ai", engine: "claude" },
    tm_score: 92,
  },
  "de-DE:hero.title": {
    status: "reviewed",
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
    editable: true,
    ...extra,
  };
}

const noopDecide = async () => {};
const noopSave = async () => {};

export const Queue: Story = {
  args: {
    tabID: "storybook",
    items: QUEUE,
    loadUnit,
    onDecide: noopDecide,
    onSaveTarget: noopSave,
  },
};

export const ScopedToCollection: Story = {
  args: {
    tabID: "storybook",
    items: QUEUE,
    scope: { collection: "Marketing", locale: "nb" },
    loadUnit,
    onDecide: noopDecide,
    onSaveTarget: noopSave,
  },
};

export const Empty: Story = {
  args: {
    tabID: "storybook",
    items: [],
    loadUnit,
    onDecide: noopDecide,
    onSaveTarget: noopSave,
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
    onDecide: noopDecide,
    onSaveTarget: noopSave,
  },
};
