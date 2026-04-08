import type { Meta, StoryObj } from "@storybook/react-vite";
import { InlinePreview } from "@neokapi/ui-primitives";
import {
  simpleBoldCodedText,
  simpleBoldSpans,
  linkAndItalicCodedText,
  linkAndItalicSpans,
  codeInlineCodedText,
  codeInlineSpans,
  lineBreakCodedText,
  lineBreakSpans,
  richCodedText,
  richSpans,
} from "../fixtures";

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
    codedText: simpleBoldCodedText,
    spans: simpleBoldSpans,
  },
};

export const LinkAndItalic: Story = {
  args: {
    codedText: linkAndItalicCodedText,
    spans: linkAndItalicSpans,
  },
};

export const CodeInline: Story = {
  args: {
    codedText: codeInlineCodedText,
    spans: codeInlineSpans,
  },
};

export const WithLineBreak: Story = {
  args: {
    codedText: lineBreakCodedText,
    spans: lineBreakSpans,
  },
};

export const RichMarkup: Story = {
  args: {
    codedText: richCodedText,
    spans: richSpans,
  },
};

export const EmptyText: Story = {
  args: { codedText: "", spans: [] },
};
