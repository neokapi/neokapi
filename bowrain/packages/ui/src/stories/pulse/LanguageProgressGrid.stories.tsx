import type { Meta, StoryObj } from "@storybook/react";
import { LanguageProgressGrid } from "../../components/pulse";
import { mockLanguages } from "./pulse-fixtures";

const meta: Meta<typeof LanguageProgressGrid> = {
  title: "Pulse/LanguageProgressGrid",
  component: LanguageProgressGrid,
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof LanguageProgressGrid>;

export const Default: Story = { args: { languages: mockLanguages } };
export const Empty: Story = { args: { languages: [] } };
