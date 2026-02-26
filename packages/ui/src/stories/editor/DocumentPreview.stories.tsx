import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { DocumentPreview } from "../../components/editor/DocumentPreview";
import { sampleBlocks } from "../fixtures";
import { withProviders } from "../decorators";

const meta: Meta<typeof DocumentPreview> = {
  title: "Editor/DocumentPreview",
  component: DocumentPreview,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ width: "100%", height: 500 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    projectId: "proj-demo-1",
    itemName: "messages.json",
    targetLocale: "fr-FR",
    onBlockSelect: fn(),
    blocks: sampleBlocks,
  },
};

export default meta;
type Story = StoryObj<typeof DocumentPreview>;

/** Default preview — source content mode, no block selected */
export const Default: Story = {};

/** Preview with a selected block highlighted */
export const WithSelectedBlock: Story = {
  args: {
    selectedBlockId: "blk-2",
  },
};

/** Controlled source mode via prop */
export const SourceMode: Story = {
  args: {
    previewContentMode: "source",
    selectedBlockId: "blk-1",
  },
};

/** Controlled target mode via prop */
export const TargetMode: Story = {
  args: {
    previewContentMode: "target",
    selectedBlockId: "blk-1",
  },
};

/** Inline mode with spacer height — used inside VisualEditorLayout */
export const InlineMode: Story = {
  args: {
    selectedBlockId: "blk-2",
    spacerHeight: 400,
    onSpacerPosition: fn(),
    onContentHeight: fn(),
  },
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ width: "100%", height: "auto", minHeight: 600 }}>
        <Story />
      </div>
    ),
  ],
};
