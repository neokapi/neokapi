import type { Meta, StoryObj } from "@storybook/react-vite";
import { TranslationDashboard } from "../../components/TranslationDashboard";
import { withProviders } from "../decorators";
import { sampleDashboardStats, largeDashboardStats } from "../fixtures";

const meta: Meta<typeof TranslationDashboard> = {
  title: "Pages/Translation/TranslationDashboard",
  component: TranslationDashboard,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 1100, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TranslationDashboard>;

/** No data yet — shows the empty onboarding state. */
export const Empty: Story = {
  args: {
    stats: null,
    projectName: "New Project",
  },
};

/** Typical project with 3 locales, 3 files, and 2 collections. */
export const Default: Story = {
  args: {
    stats: sampleDashboardStats,
    projectName: "Demo App",
  },
};

/** Large project with 6 locales and 4 files across 2 collections. */
export const LargeProject: Story = {
  args: {
    stats: largeDashboardStats,
    projectName: "Marketing Platform",
  },
};

/** Dashboard without a project name in the header. */
export const NoProjectName: Story = {
  args: {
    stats: sampleDashboardStats,
  },
};

/** Fully translated project — all locales at 100%. */
export const FullyTranslated: Story = {
  args: {
    stats: {
      ...sampleDashboardStats,
      locale_stats: sampleDashboardStats.locale_stats.map((l) => ({
        ...l,
        translated_blocks: l.total_blocks,
        translated_words: l.total_words,
        percentage: 100,
      })),
    },
    projectName: "Completed Project",
  },
};

/** Single locale — minimal chart layout. */
export const SingleLocale: Story = {
  args: {
    stats: {
      ...sampleDashboardStats,
      locale_stats: [sampleDashboardStats.locale_stats[0]],
      item_stats: sampleDashboardStats.item_stats.map((item) => ({
        ...item,
        locales: [item.locales[0]],
      })),
      collection_stats: sampleDashboardStats.collection_stats.map((coll) => ({
        ...coll,
        locales: [coll.locales[0]],
      })),
    },
    projectName: "French Only",
  },
};
