import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeliveryPanel } from "../../components/DeliveryPanel";
import type { LocaleTranslationStats } from "../../types/api";

const localeStats: LocaleTranslationStats[] = [
  {
    locale: "fr-FR",
    display_name: "French (France)",
    translated_blocks: 50,
    total_blocks: 50,
    translated_words: 3800,
    total_words: 3800,
    percentage: 100,
    approved_blocks: 50,
    failing_checks: 0,
    ship_state: "governed",
  },
  {
    locale: "de-DE",
    display_name: "German (Germany)",
    translated_blocks: 50,
    total_blocks: 50,
    translated_words: 3800,
    total_words: 3800,
    percentage: 100,
    approved_blocks: 12,
    failing_checks: 0,
    ship_state: "ai_shippable",
  },
  {
    locale: "ja-JP",
    display_name: "Japanese (Japan)",
    translated_blocks: 12,
    total_blocks: 50,
    translated_words: 900,
    total_words: 3800,
    percentage: 23.7,
    approved_blocks: 3,
    failing_checks: 0,
    ship_state: "pending",
  },
];

const meta: Meta<typeof DeliveryPanel> = {
  title: "Components/DeliveryPanel",
  component: DeliveryPanel,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 420, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    localeStats,
    onOpenConnectors: () => {},
    onOpenReview: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof DeliveryPanel>;

export const Default: Story = {
  args: {
    connectors: [
      {
        id: "conn-wp",
        name: "Marketing WordPress",
        lastSync: "2026-07-17T09:12:00Z",
      },
      {
        id: "conn-git",
        name: "docs-repo",
        lastSync: "2026-07-16T18:40:00Z",
        lastError: "push failed: remote rejected ref (protected branch)",
      },
    ],
  },
};

export const NoConnectors: Story = {
  args: { connectors: [] },
};

export const NeverSynced: Story = {
  args: {
    connectors: [{ id: "conn-figma", name: "Design files", lastSync: "" }],
  },
};
