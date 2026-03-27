import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TargetCellEditor } from "../../components/editor/TargetCellEditor";
import {
  simpleBoldCodedText,
  simpleBoldSpans,
  linkAndItalicCodedText,
  linkAndItalicSpans,
  richCodedText,
  richSpans,
} from "../fixtures";

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
    sourceSpans: simpleBoldSpans,
    onSave: fn(),
    onCancel: fn(),
  },
};

export const WithBoldText: Story = {
  args: {
    initialCodedText: simpleBoldCodedText,
    initialSpans: simpleBoldSpans,
    sourceSpans: simpleBoldSpans,
    onSave: fn(),
    onCancel: fn(),
  },
};

export const WithLinks: Story = {
  args: {
    initialCodedText: linkAndItalicCodedText,
    initialSpans: linkAndItalicSpans,
    sourceSpans: linkAndItalicSpans,
    onSave: fn(),
    onCancel: fn(),
  },
};

export const RichContent: Story = {
  args: {
    initialCodedText: richCodedText,
    initialSpans: richSpans,
    sourceSpans: richSpans,
    onSave: fn(),
    onCancel: fn(),
  },
};
