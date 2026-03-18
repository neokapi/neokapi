import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceLanguageSettings } from "../../components/WorkspaceLanguageSettings";
import { withProviders } from "../decorators";
import { sampleWorkspace } from "./fixtures";

const meta: Meta<typeof WorkspaceLanguageSettings> = {
  title: "Components/WorkspaceLanguageSettings",
  component: WorkspaceLanguageSettings,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspaceLanguageSettings>;

export const WithLanguages: Story = {
  args: { workspace: sampleWorkspace, onUpdate: fn() },
};

export const DefaultLanguages: Story = {
  args: {
    workspace: { ...sampleWorkspace, languages: undefined },
    onUpdate: fn(),
  },
};
