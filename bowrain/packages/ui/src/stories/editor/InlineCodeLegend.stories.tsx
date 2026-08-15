import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { InlineCodeLegend } from "@neokapi/ui-primitives";
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
  unknownOpen,
  unknownClose,
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

/**
 * A span type no vocabulary pack defines. The chip reads "widget>", derived
 * from the type name — the kapi kit's story of the same name renders it
 * identically, because both kits resolve through one registry.
 */
export const UnknownType: Story = {
  args: {
    spans: [unknownOpen, unknownClose],
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
