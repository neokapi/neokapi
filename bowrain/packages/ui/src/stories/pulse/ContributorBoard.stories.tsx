import type { Meta, StoryObj } from "@storybook/react";
import { ContributorBoard } from "../../components/pulse";
import { mockContributors } from "./pulse-fixtures";

const meta: Meta<typeof ContributorBoard> = {
  title: "Pulse/ContributorBoard",
  component: ContributorBoard,
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof ContributorBoard>;

export const Default: Story = { args: { contributors: mockContributors } };
export const Empty: Story = { args: { contributors: [] } };
