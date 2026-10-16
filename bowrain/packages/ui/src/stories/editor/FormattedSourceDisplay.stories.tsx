import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormattedSourceDisplay } from "../../components/editor/FormattedSourceDisplay";
import {
  simpleBoldRuns,
  linkAndItalicRuns,
  codeInlineRuns,
  lineBreakRuns,
  richRuns,
  mdFormattingRuns,
} from "../fixtures";

const meta: Meta<typeof FormattedSourceDisplay> = {
  title: "Editor/Core/FormattedSourceDisplay",
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
    runs: simpleBoldRuns,
  },
};

export const LinkAndItalic: Story = {
  args: {
    runs: linkAndItalicRuns,
  },
};

export const CodeInline: Story = {
  args: {
    runs: codeInlineRuns,
  },
};

export const WithLineBreak: Story = {
  args: {
    runs: lineBreakRuns,
  },
};

export const RichMarkup: Story = {
  args: {
    runs: richRuns,
  },
};

export const MarkdownFormatting: Story = {
  args: {
    runs: mdFormattingRuns,
  },
};

export const PlainText: Story = {
  args: {
    runs: [{ text: "Welcome to Neokapi — the AI-native localization engine." }],
  },
};
