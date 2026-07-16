import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleDemandView } from "./LocaleDemandView";

// Locale demand — DESIGN PROTOTYPE (mock data only). Composed page used for
// the PR screenshots: map + language table, with optional drill-down open.

const meta: Meta<typeof LocaleDemandView> = {
  title: "Locale Demand/Page",
  component: LocaleDemandView,
  parameters: { layout: "fullscreen" },
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
