import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { UpsellTable } from "./UpsellTable";
import { sampleUpsells } from "../ctrl-fixtures";

const meta: Meta<typeof UpsellTable> = {
  title: "Ctrl/UpsellTable",
  component: UpsellTable,
  parameters: { layout: "padded" },
  args: { onRowClick: fn() },
  decorators: [
    (Story) => (
      <div className="w-full max-w-5xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof UpsellTable>;

export const Default: Story = {
  args: { upsells: sampleUpsells, loading: false },
};

export const Loading: Story = {
  args: { upsells: [], loading: true },
};

export const Empty: Story = {
  args: { upsells: [], loading: false },
};
