import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { OpBuilder } from "./OpBuilder";
import { withBrandHub } from "../../stories/brandHubFixtures";

const pad: Decorator = (Story) => (
  <div style={{ maxWidth: 480, padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof OpBuilder> = {
  title: "Brand Hub/Experiments/OpBuilder",
  component: OpBuilder,
  tags: ["autodocs"],
  decorators: [withBrandHub, pad],
  args: { changesetId: "cs-1" },
};

export default meta;
type Story = StoryObj<typeof OpBuilder>;

/** Pick an action and build an op against the live concept graph. */
export const Default: Story = {};
