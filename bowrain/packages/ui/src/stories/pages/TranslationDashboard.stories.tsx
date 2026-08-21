import type { Meta, StoryObj } from "@storybook/react-vite";
import { TranslationDashboard } from "../../components/TranslationDashboard";
import { DeliveryPanel } from "../../components/DeliveryPanel";
import { withProviders } from "../decorators";
import {
  sampleDashboardStats,
  largeDashboardStats,
  shipStateDashboardStats,
  complianceDashboardStats,
} from "../fixtures";

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

/**
 * Server-derived ship states: the ship-readiness band lists each locale's
 * state (governed / AI-shippable / pending) and the collection heatmap carries
 * the compact rollup indicators.
 */
export const WithShipStates: Story = {
  args: {
    stats: shipStateDashboardStats,
    projectName: "Demo App",
  },
};

/**
 * Server-derived compliance rates beside each ship-state badge: fr-FR is
 * voice-informed (worker draft scoring has run), the others are checks-only —
 * the tooltip states the basis so the number is never over-read.
 */
export const WithComplianceRates: Story = {
  args: {
    stats: complianceDashboardStats,
    projectName: "Demo App",
  },
};

/** Ship readiness beside the read-only delivery panel, as the route composes it. */
export const WithDelivery: Story = {
  args: {
    stats: shipStateDashboardStats,
    projectName: "Demo App",
    delivery: (
      <DeliveryPanel
        localeStats={shipStateDashboardStats.locale_stats}
        connectors={[
          { id: "conn-wp", name: "Marketing WordPress", lastSync: "2026-07-17T09:12:00Z" },
          {
            id: "conn-git",
            name: "docs-repo",
            lastSync: "2026-07-16T18:40:00Z",
            lastError: "push failed: remote rejected ref (protected branch)",
          },
        ]}
        onOpenConnectors={() => {}}
        onOpenReview={() => {}}
      />
    ),
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
