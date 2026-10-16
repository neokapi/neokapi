import type { Meta, StoryObj } from "@storybook/react-vite";
import { InlinePreview } from "@neokapi/ui-primitives";
import {
  simpleBoldRuns,
  linkAndItalicRuns,
  codeInlineRuns,
  lineBreakRuns,
  richRuns,
} from "../fixtures";
import { codeSpans, toEditorText } from "../../components/editor/editorText";

const meta: Meta<typeof InlinePreview> = {
  title: "Editor/Formatting/InlinePreview",
  component: InlinePreview,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 600, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof InlinePreview>;

export const BoldText: Story = {
  args: {
    ...toEditorText(simpleBoldRuns),
  },
};

export const LinkAndItalic: Story = {
  args: {
    ...toEditorText(linkAndItalicRuns),
  },
};

export const CodeInline: Story = {
  args: {
    ...toEditorText(codeInlineRuns),
  },
};

export const WithLineBreak: Story = {
  args: {
    ...toEditorText(lineBreakRuns),
  },
};

export const RichMarkup: Story = {
  args: {
    ...toEditorText(richRuns),
  },
};

export const EmptyText: Story = {
  args: { codedText: "", spans: [] },
};
