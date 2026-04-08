import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { InlineCodeEditor } from "../../components/editor/InlineCodeEditor";
import {
  simpleBoldCodedText,
  simpleBoldSpans,
  linkAndItalicCodedText,
  linkAndItalicSpans,
  richCodedText,
  richSpans,
} from "./fixtures";

const meta: Meta<typeof InlineCodeEditor> = {
  title: "Editor/Core/InlineCodeEditor",
  component: InlineCodeEditor,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Lexical-based rich text editor for translatable content with visual inline code chips. " +
          "Renders formatting (bold, italic, links) as styled tag chips. Enter saves, Escape cancels. " +
          "Ctrl+1..9 inserts tags from the palette.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof InlineCodeEditor>;

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

export const Compact: Story = {
  name: "Compact (no palette/preview)",
  args: {
    initialCodedText: simpleBoldCodedText,
    initialSpans: simpleBoldSpans,
    sourceSpans: simpleBoldSpans,
    onSave: fn(),
    onCancel: fn(),
    compact: true,
  },
};
