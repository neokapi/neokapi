import type { Meta, StoryObj } from "@storybook/react-vite";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import {
  simpleBoldCodedText, simpleBoldSpans,
  linkAndItalicCodedText, linkAndItalicSpans,
  codeInlineCodedText, codeInlineSpans,
  lineBreakCodedText, lineBreakSpans,
  richCodedText, richSpans,
} from "../fixtures";

const meta: Meta<typeof SourceCellDisplay> = {
  title: "Editor/SourceCellDisplay",
  component: SourceCellDisplay,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 600, padding: 16, fontSize: 14, lineHeight: 1.6 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof SourceCellDisplay>;

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

export const PlainText: Story = {
  args: {
    codedText: "Welcome to Gokapi — the AI-native localization engine.",
    spans: [],
  },
};
