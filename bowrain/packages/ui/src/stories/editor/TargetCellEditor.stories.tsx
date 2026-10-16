import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { InlineCodeEditor as TargetCellEditor } from "@neokapi/ui-primitives";
import { simpleBoldRuns, linkAndItalicRuns, richRuns } from "../fixtures";
import { codeSpans, toEditorText } from "../../components/editor/editorText";

const meta: Meta<typeof TargetCellEditor> = {
  title: "Editor/Core/TargetCellEditor",
  component: TargetCellEditor,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TargetCellEditor>;

export const Empty: Story = {
  args: {
    initialCodedText: "",
    initialSpans: [],
    sourceSpans: codeSpans(simpleBoldRuns),
    onSave: fn(),
    onCancel: fn(),
  },
};

export const WithBoldText: Story = {
  args: {
    initialCodedText: toEditorText(simpleBoldRuns).codedText,
    initialSpans: toEditorText(simpleBoldRuns).spans,
    sourceSpans: codeSpans(simpleBoldRuns),
    onSave: fn(),
    onCancel: fn(),
  },
};

export const WithLinks: Story = {
  args: {
    initialCodedText: toEditorText(linkAndItalicRuns).codedText,
    initialSpans: toEditorText(linkAndItalicRuns).spans,
    sourceSpans: codeSpans(linkAndItalicRuns),
    onSave: fn(),
    onCancel: fn(),
  },
};

export const RichContent: Story = {
  args: {
    initialCodedText: toEditorText(richRuns).codedText,
    initialSpans: toEditorText(richRuns).spans,
    sourceSpans: codeSpans(richRuns),
    onSave: fn(),
    onCancel: fn(),
  },
};
