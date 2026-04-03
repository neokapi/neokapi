import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { VisualEditorToolbar } from "../../components/editor/VisualEditorToolbar";
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
  underlineOpen,
  underlineClose,
  strikeOpen,
  strikeClose,
  richSpans,
} from "../fixtures";

const meta: Meta<typeof VisualEditorToolbar> = {
  title: "Editor/Visual/VisualEditorToolbar",
  component: VisualEditorToolbar,
  tags: ["autodocs"],
  args: {
    onInsertTag: fn(),
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
type Story = StoryObj<typeof VisualEditorToolbar>;

/** Bold only */
export const BoldOnly: Story = {
  args: {
    sourceSpans: [boldOpen, boldClose],
  },
};

/** Bold + italic */
export const BoldAndItalic: Story = {
  args: {
    sourceSpans: [boldOpen, boldClose, italicOpen, italicClose],
  },
};

/** All tag types from rich spans */
export const AllTags: Story = {
  args: {
    sourceSpans: richSpans,
  },
};

/** Extended set including underline, strikethrough */
export const ExtendedTags: Story = {
  args: {
    sourceSpans: [
      boldOpen,
      boldClose,
      italicOpen,
      italicClose,
      underlineOpen,
      underlineClose,
      strikeOpen,
      strikeClose,
      linkOpen,
      linkClose,
      codeOpen,
      codeClose,
      lineBreak,
    ],
  },
};

/** Disabled state (not editing) */
export const Disabled: Story = {
  args: {
    sourceSpans: richSpans,
    disabled: true,
  },
};

/** No spans — renders nothing */
export const Empty: Story = {
  args: {
    sourceSpans: [],
  },
};
