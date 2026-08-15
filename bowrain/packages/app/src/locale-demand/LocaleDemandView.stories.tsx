import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@neokapi/ui";
import { LocaleDemandView } from "./LocaleDemandView";

// Locale demand on the sample dataset — the view a workspace gets before it
// connects PostHog, carrying the sample-data notice and the per-surface marks.
// Composed page used for the PR screenshots: map + language table, with
// optional drill-down open.

const connectAction = (
  <Button size="sm" variant="outline">
    Connect PostHog
  </Button>
);

const meta: Meta<typeof LocaleDemandView> = {
  title: "Locale Demand/Page",
  component: LocaleDemandView,
  parameters: { layout: "fullscreen" },
  args: { connectAction },
};

export default meta;
type Story = StoryObj<typeof LocaleDemandView>;

export const Default: Story = {};

export const CountryDrillDown: Story = {
  args: { initialSelection: { kind: "country", code: "BR" } },
};

export const LanguageDrillDown: Story = {
  args: { initialSelection: { kind: "language", code: "ko" } },
};

/** A configured connector that failed: same fixture, the reason named in the notice. */
export const DegradedToSample: Story = {
  args: {
    degradedReason: "personal API key was rejected",
    connectAction: (
      <Button size="sm" variant="outline">
        Fix connection
      </Button>
    ),
  },
};
