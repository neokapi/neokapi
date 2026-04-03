import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { InlineCodeLegend } from "../../components/editor/InlineCodeLegend";
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
  imgTag,
  underlineOpen,
  strikeOpen,
} from "../fixtures";

const meta: Meta<typeof InlineCodeLegend> = {
  title: "Editor/Tags/InlineCodeLegend",
  component: InlineCodeLegend,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 360, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof InlineCodeLegend>;

export const Default: Story = {
  args: {
    spans: [boldOpen, boldClose, italicOpen, italicClose, linkOpen, linkClose],
    onClose: fn(),
  },
};

export const WithManyCategories: Story = {
  args: {
    spans: [
      boldOpen,
      boldClose,
      italicOpen,
      italicClose,
      underlineOpen,
      strikeOpen,
      linkOpen,
      linkClose,
      codeOpen,
      codeClose,
      lineBreak,
      imgTag,
    ],
    onClose: fn(),
  },
};
