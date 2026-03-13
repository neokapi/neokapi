import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormattedSourceDisplay } from "../../components/editor/FormattedSourceDisplay";
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
  mdFormattingCodedText,
  mdFormattingSpans,
} from "../fixtures";

const meta: Meta<typeof FormattedSourceDisplay> = {
  title: "Editor/FormattedSourceDisplay",
  component: FormattedSourceDisplay,
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
type Story = StoryObj<typeof FormattedSourceDisplay>;

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

export const MarkdownFormatting: Story = {
  args: {
    codedText: mdFormattingCodedText,
    spans: mdFormattingSpans,
  },
};

export const PlainText: Story = {
  args: {
    codedText: "Welcome to Neokapi — the AI-native localization engine.",
    spans: [],
  },
};
