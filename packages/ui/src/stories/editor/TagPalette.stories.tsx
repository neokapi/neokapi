import type { Meta, StoryObj } from "@storybook/react";
import { fn } from "@storybook/test";
import { TagPalette } from "../../components/editor/TagPalette";
import {
  boldOpen, boldClose, italicOpen, italicClose,
  linkOpen, linkClose, codeOpen, codeClose,
  lineBreak, richSpans,
} from "../fixtures";

const meta: Meta<typeof TagPalette> = {
  title: "Editor/TagPalette",
  component: TagPalette,
  tags: ["autodocs"],
  args: {
    onInsert: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 600, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TagPalette>;

export const BoldPair: Story = {
  args: {
    sourceSpans: [boldOpen, boldClose],
  },
};

export const MultiplePairs: Story = {
  args: {
    sourceSpans: [boldOpen, boldClose, italicOpen, italicClose, linkOpen, linkClose],
  },
};

export const WithCodeAndBreak: Story = {
  args: {
    sourceSpans: [codeOpen, codeClose, lineBreak],
  },
};

export const AllTags: Story = {
  args: {
    sourceSpans: richSpans,
  },
};

/** Some tags already used — shown dimmed */
export const PartiallyUsed: Story = {
  args: {
    sourceSpans: [boldOpen, boldClose, italicOpen, italicClose],
    usedSpans: [boldOpen, boldClose],
  },
};

export const Empty: Story = {
  args: {
    sourceSpans: [],
  },
};
