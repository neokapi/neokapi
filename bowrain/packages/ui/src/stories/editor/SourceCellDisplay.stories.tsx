import type { Meta, StoryObj } from "@storybook/react-vite";
import { SourceCellDisplay } from "../../components/editor/SourceCellDisplay";
import {
  simpleBoldRuns,
  linkAndItalicRuns,
  codeInlineRuns,
  lineBreakRuns,
  richRuns,
} from "../fixtures";

const meta: Meta<typeof SourceCellDisplay> = {
  title: "Editor/Core/SourceCellDisplay",
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

export const PlainText: Story = {
  args: {
    runs: [{ text: "Welcome to Neokapi — the AI-native localization engine." }],
  },
};
