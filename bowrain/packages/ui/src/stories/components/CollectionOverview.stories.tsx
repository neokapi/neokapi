import type { Meta, StoryObj } from "@storybook/react-vite";
import { CollectionOverview } from "../../components/collections/CollectionOverview";
import type {
  CollectionTranslationStats,
  LocaleTranslationStats,
  ShipState,
} from "../../types/api";

const meta: Meta<typeof CollectionOverview> = {
  title: "Workspace/Collections/CollectionOverview",
  component: CollectionOverview,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 1100, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    onOpenCollection: () => {},
    onOpenAllItems: () => {},
    onReviewCollection: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof CollectionOverview>;

function locale(
  code: string,
  total: number,
  translated: number,
  approved: number,
  ship?: ShipState,
): LocaleTranslationStats {
  return {
    locale: code,
    translated_blocks: translated,
    total_blocks: total,
    translated_words: translated * 7,
    total_words: total * 7,
    percentage: total > 0 ? (translated / total) * 100 : 0,
    approved_blocks: approved,
    ship_state: ship,
  };
}

function collection(
  id: string,
  name: string,
  product: string,
  channel: string,
  locales: LocaleTranslationStats[],
  items = 12,
): CollectionTranslationStats {
  const blocks = locales[0]?.total_blocks ?? 0;
  return {
    collection_id: id,
    collection_name: name,
    channel: `${product}/${channel}`,
    coordinates: { product, channel },
    item_count: items,
    block_count: blocks,
    word_count: blocks * 7,
    locales,
  };
}

/**
 * The dogfood shape: two products across several surfaces, one target language.
 * The grouping axis is derived — product covers every collection and cuts them
 * into the fewest groups, so it wins over channel without being named in code.
 */
export const OneTargetLanguage: Story = {
  args: {
    itemTotal: 1008,
    collections: [
      collection("c1", "bowrain-app", "bowrain", "app", [locale("nb", 820, 610, 410, "pending")]),
      collection("c2", "bowrain-docs", "bowrain", "docs", [
        locale("nb", 1400, 1400, 1400, "governed"),
      ]),
      collection("c3", "bowrain-email", "bowrain", "email", [locale("nb", 90, 88, 12, "pending")]),
      collection("c4", "neokapi-cli", "neokapi", "cli", [
        locale("nb", 640, 640, 90, "ai_shippable"),
      ]),
      collection("c5", "neokapi-docs", "neokapi", "docs", [locale("nb", 3100, 2010, 900)]),
      collection("c6", "neokapi-engine", "neokapi", "engine", [locale("nb", 210, 40, 0)]),
    ],
  },
};

/** Twenty target languages: the rails stack into a ladder that scrolls in-card. */
export const ManyTargetLanguages: Story = {
  args: {
    itemTotal: 1008,
    collections: ["app", "docs"].map((channel, ci) =>
      collection(
        `c${ci}`,
        `bowrain-${channel}`,
        "bowrain",
        channel,
        Array.from({ length: 20 }, (_, i) =>
          locale(
            ["nb", "de", "fr", "es", "it", "nl", "pt", "pl", "sv", "da"][i % 10] +
              (i < 10 ? "" : "-x"),
            800,
            800 - i * 35,
            400 - i * 18,
          ),
        ),
      ),
    ),
  },
};

/**
 * Coordinates are optional. With none persisted there is no axis to group by
 * and no axis control — the collections render as one flat list rather than
 * under an invented heading.
 */
export const NoCoordinates: Story = {
  args: {
    itemTotal: 40,
    collections: [
      {
        collection_id: "c1",
        collection_name: "Uploaded files",
        item_count: 12,
        block_count: 300,
        word_count: 2100,
        locales: [locale("nb", 300, 120, 60)],
      },
      {
        collection_id: "c2",
        collection_name: "Marketing",
        item_count: 4,
        block_count: 80,
        word_count: 900,
        locales: [locale("nb", 80, 80, 80, "governed")],
      },
    ],
  },
};

/**
 * A collection missing the grouping axis falls into a remainder group named for
 * what it lacks, and items belonging to no collection are shown last as
 * themselves.
 */
export const PartialCoordinatesAndUngrouped: Story = {
  args: {
    itemTotal: 60,
    collections: [
      collection("c1", "bowrain-app", "bowrain", "app", [locale("nb", 400, 300, 200)]),
      collection("c2", "bowrain-docs", "bowrain", "docs", [locale("nb", 900, 900, 600)]),
      collection("c3", "neokapi-cli", "neokapi", "cli", [locale("nb", 300, 150, 30)]),
      {
        collection_id: "c4",
        collection_name: "Legacy import",
        coordinates: { market: "de" },
        item_count: 6,
        block_count: 120,
        word_count: 800,
        locales: [locale("nb", 120, 20, 0)],
      },
      {
        collection_id: "",
        collection_name: "",
        ungrouped: true,
        item_count: 9,
        block_count: 40,
        word_count: 260,
        locales: [locale("nb", 40, 5, 0)],
      },
    ],
  },
};
