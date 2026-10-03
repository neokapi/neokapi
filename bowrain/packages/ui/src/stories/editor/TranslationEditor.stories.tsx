import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { TranslationEditor } from "../../components/TranslationEditor";
import { sampleBlocks, sampleProject } from "../fixtures";
import { createProvidersDecorator, withProviders } from "../decorators";

const meta: Meta<typeof TranslationEditor> = {
  title: "Editor/Core/TranslationEditor",
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

/**
 * Someone saves the first block's French translation while it is open here.
 * Saving shows their translation beside this one and asks before anything is
 * written over it.
 */
export const SaveMeetsAChangedTranslation: Story = {
  args: {
    project: sampleProject,
    fileName: "messages.json",
    onBack: fn(),
  },
  decorators: [
    createProvidersDecorator(sampleBlocks, {
      concurrentEdit: { blockId: "blk-1", locale: "fr-FR", text: "Bienvenue dans Neokapi" },
    }),
  ],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(await canvas.findByTestId("target-display"));
    await canvas.findByTestId("unified-target-editor");
    await userEvent.click(canvas.getByTestId("unified-save"));
    await expect(await body.findByTestId("stale-change-dialog")).toBeInTheDocument();
    await expect(body.getByTestId("stale-current")).toHaveTextContent("Bienvenue dans Neokapi");
  },
};
