import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TranslationEditor } from "../../components/TranslationEditor";
import { sampleProject } from "../fixtures";
import { withProviders } from "../decorators";

const meta: Meta<typeof TranslationEditor> = {
  title: "Editor/TranslationEditor",
  component: TranslationEditor,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ width: "100vw", height: "100vh", overflow: "auto" }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof TranslationEditor>;

export const Default: Story = {
  args: {
    project: sampleProject,
    fileName: "messages.json",
    onBack: fn(),
  },
};

export const WithExportHandler: Story = {
  args: {
    project: sampleProject,
    fileName: "messages.json",
    onBack: fn(),
    onExport: fn(),
  },
};
