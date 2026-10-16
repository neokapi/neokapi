import type { Meta, StoryObj } from "@storybook/react-vite";
import { TagChipComponent } from "@neokapi/ui-primitives";
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
  supOpen,
  chip,
} from "../fixtures";

const meta: Meta<typeof TagChipComponent> = {
  title: "Editor/Tags/TagChipComponent",
  component: TagChipComponent,
  tags: ["autodocs"],
  argTypes: {
    highlighted: { control: "boolean" },
    dimmed: { control: "boolean" },
    index: { control: { type: "number", min: 1, max: 20 } },
    pairIndex: { control: { type: "number", min: 1, max: 10 } },
  },
};

export default meta;
type Story = StoryObj<typeof TagChipComponent>;

export const BoldOpening: Story = {
  args: { spanInfo: chip(boldOpen), index: 1, pairIndex: 1 },
};

export const BoldClosing: Story = {
  args: { spanInfo: chip(boldClose), index: 2, pairIndex: 1 },
};

export const ItalicOpening: Story = {
  args: { spanInfo: chip(italicOpen), index: 1, pairIndex: 1 },
};

export const LinkOpening: Story = {
  args: { spanInfo: chip(linkOpen), index: 1, pairIndex: 1 },
};

export const CodeOpening: Story = {
  args: { spanInfo: chip(codeOpen), index: 1, pairIndex: 1 },
};

export const LineBreak: Story = {
  args: { spanInfo: chip(lineBreak), index: 1 },
};

export const Image: Story = {
  args: { spanInfo: chip(imgTag), index: 1 },
};

export const Highlighted: Story = {
  args: { spanInfo: chip(boldOpen), index: 1, pairIndex: 1, highlighted: true },
};

export const Dimmed: Story = {
  args: { spanInfo: chip(boldOpen), index: 1, pairIndex: 1, dimmed: true },
};

/** All semantic categories side by side */
export const AllCategories: Story = {
  render: () => (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
      <TagChipComponent spanInfo={chip(boldOpen)} index={1} pairIndex={1} />
      <TagChipComponent spanInfo={chip(boldClose)} index={2} pairIndex={1} />
      <TagChipComponent spanInfo={chip(italicOpen)} index={3} pairIndex={2} />
      <TagChipComponent spanInfo={chip(italicClose)} index={4} pairIndex={2} />
      <TagChipComponent spanInfo={chip(linkOpen)} index={5} pairIndex={3} />
      <TagChipComponent spanInfo={chip(linkClose)} index={6} pairIndex={3} />
      <TagChipComponent spanInfo={chip(codeOpen)} index={7} pairIndex={4} />
      <TagChipComponent spanInfo={chip(codeClose)} index={8} pairIndex={4} />
      <TagChipComponent spanInfo={chip(underlineOpen)} index={9} pairIndex={5} />
      <TagChipComponent spanInfo={chip(strikeOpen)} index={10} pairIndex={6} />
      <TagChipComponent spanInfo={chip(supOpen)} index={11} pairIndex={7} />
      <TagChipComponent spanInfo={chip(lineBreak)} index={12} />
      <TagChipComponent spanInfo={chip(imgTag)} index={13} />
    </div>
  ),
};
