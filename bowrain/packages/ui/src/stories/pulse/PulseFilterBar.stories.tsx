import type { Meta, StoryObj } from "@storybook/react";
import { PulseFilterBar } from "../../components/pulse";

const meta: Meta<typeof PulseFilterBar> = {
  title: "Pulse/PulseFilterBar",
  component: PulseFilterBar,
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof PulseFilterBar>;

export const WithFilters: Story = {
  args: {
    filters: [
      { key: "language", value: "fr-FR" },
      { key: "project", value: "Web Application" },
    ],
    onRemove: (key: string) => console.log("remove", key),
    onClear: () => console.log("clear"),
  },
};

export const Empty: Story = {
  args: {
    filters: [],
    onRemove: (key: string) => console.log("remove", key),
    onClear: () => console.log("clear"),
    presets: [
      { label: "This week", filters: [{ key: "time", value: "this-week" }] },
      { label: "Needs help", filters: [{ key: "progress", value: "<50" }] },
    ],
  },
};

export const ManyFilters: Story = {
  args: {
    filters: [
      { key: "language", value: "fr-FR" },
      { key: "language", value: "de-DE" },
      { key: "project", value: "Web Application" },
      { key: "contributor", value: "Alice Chen" },
      { key: "period", value: "last-30-days" },
    ],
    onRemove: (key: string) => console.log("remove", key),
    onClear: () => console.log("clear"),
  },
};
