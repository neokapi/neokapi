import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TagPalette } from "@neokapi/ui-primitives";
import {
  boldOpen,
  boldClose,
  italicOpen,
  italicClose,
  linkOpen,
  linkClose,
  codeOpen,
  codeClose,
  lineBreak,
  richRuns,
} from "../fixtures";
import { codeSpans, toEditorText } from "../../components/editor/editorText";

const meta: Meta<typeof TagPalette> = {
  title: "Editor/Tags/TagPalette",
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
    sourceSpans: codeSpans([boldOpen, boldClose]),
  },
};

export const MultiplePairs: Story = {
  args: {
    sourceSpans: codeSpans([boldOpen, boldClose, italicOpen, italicClose, linkOpen, linkClose]),
  },
};

export const WithCodeAndBreak: Story = {
  args: {
    sourceSpans: codeSpans([codeOpen, codeClose, lineBreak]),
  },
};

export const AllTags: Story = {
  args: {
    sourceSpans: codeSpans(richRuns),
  },
};

/** Some tags already used — shown dimmed */
export const PartiallyUsed: Story = {
  args: {
    sourceSpans: codeSpans([boldOpen, boldClose, italicOpen, italicClose]),
    usedSpans: codeSpans([boldOpen, boldClose]),
  },
};

export const Empty: Story = {
  args: {
    sourceSpans: codeSpans([]),
  },
};
