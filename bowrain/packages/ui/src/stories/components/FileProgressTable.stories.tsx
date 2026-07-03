import type { Meta, StoryObj } from "@storybook/react-vite";
import { FileProgressTable } from "../../components/FileProgressTable";
import { sampleItemStats } from "./fixtures";

const meta: Meta<typeof FileProgressTable> = {
  title: "Workspace/Collections/FileProgressTable",
  component: FileProgressTable,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FileProgressTable>;

export const Default: Story = {
  args: { itemStats: sampleItemStats, locales: ["fr-FR", "de-DE"] },
};

/**
 * Many files: the table scrolls inside the card with a sticky header row.
 * Scroll the list to see the header stay pinned.
 */
export const ManyFiles: Story = {
  args: {
    itemStats: Array.from({ length: 60 }, (_, i) => ({
      item_name: `content/pages/section-${String(i + 1).padStart(2, "0")}.json`,
      item_id: `item-${i + 1}`,
      format: i % 3 === 0 ? "xliff" : "json",
      collection_id: "c1",
      block_count: 40 + ((i * 13) % 200),
      word_count: 400 + ((i * 137) % 4000),
      locales: [
        {
          locale: "fr-FR",
          translated_blocks: (i * 7) % 40,
          total_blocks: 40,
          translated_words: ((i * 7) % 40) * 10,
          total_words: 400,
          percentage: ((i * 7) % 41) * 2.5,
        },
        {
          locale: "de-DE",
          translated_blocks: (i * 11) % 40,
          total_blocks: 40,
          translated_words: ((i * 11) % 40) * 10,
          total_words: 400,
          percentage: ((i * 11) % 41) * 2.5,
        },
      ],
    })),
    locales: ["fr-FR", "de-DE"],
  },
};
