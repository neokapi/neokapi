import type { Meta, StoryObj } from "@storybook/react";
import { TermExplorerPublic } from "../../components/pulse";
import { mockTerms } from "./pulse-fixtures";

const meta: Meta<typeof TermExplorerPublic> = {
  title: "Pulse/TermExplorerPublic",
  component: TermExplorerPublic,
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof TermExplorerPublic>;

export const Default: Story = { args: { terms: mockTerms } };
export const Empty: Story = { args: { terms: [] } };
