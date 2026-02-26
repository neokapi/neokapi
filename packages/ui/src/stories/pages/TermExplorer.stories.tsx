import type { Meta, StoryObj } from "@storybook/react";
import { fn } from "@storybook/test";
import { TermExplorer } from "../../components/terms/TermExplorer";
import { withProviders } from "../decorators";

const meta: Meta<typeof TermExplorer> = {
  title: "Pages/TermExplorer",
  component: TermExplorer,
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
type Story = StoryObj<typeof TermExplorer>;

export const Default: Story = {
  args: {
    sourceLocale: "en-US",
    targetLocales: ["fr-FR", "de-DE"],
    projectName: "Demo App",
    onBack: fn(),
  },
};
