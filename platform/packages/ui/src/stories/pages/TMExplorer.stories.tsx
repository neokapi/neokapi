import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TMExplorer } from "../../components/tm/TMExplorer";
import { withProviders } from "../decorators";

const meta: Meta<typeof TMExplorer> = {
  title: "Pages/Translation/TMExplorer",
  component: TMExplorer,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 960, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TMExplorer>;

export const Default: Story = {
  args: {
    sourceLocale: "en-US",
    targetLocales: ["fr-FR", "de-DE", "ja-JP"],
    onBack: fn(),
  },
};
