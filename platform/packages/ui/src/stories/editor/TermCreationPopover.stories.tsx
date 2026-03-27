import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TermCreationPopover } from "../../components/editor/TermCreationPopover";

const meta: Meta<typeof TermCreationPopover> = {
  title: "Editor/Terminology/TermCreationPopover",
  component: TermCreationPopover,
  tags: ["autodocs"],
  args: {
    onSubmit: fn(),
    onClose: fn(),
    sourceLocale: "en-US",
    targetLocale: "fr-FR",
  },
  decorators: [
    (Story) => (
      <div style={{ padding: 100, minHeight: 400 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TermCreationPopover>;

/** Popover open with pre-selected source text */
export const Open: Story = {
  args: {
    open: true,
    selectedText: "localization",
  },
};

/** Popover closed */
export const Closed: Story = {
  args: {
    open: false,
    selectedText: "",
  },
};

/** Multi-word source text selection */
export const MultiWordSelection: Story = {
  args: {
    open: true,
    selectedText: "translation memory",
  },
};

/** Japanese target locale */
export const JapaneseTarget: Story = {
  args: {
    open: true,
    selectedText: "quality assurance",
    targetLocale: "ja-JP",
  },
};
