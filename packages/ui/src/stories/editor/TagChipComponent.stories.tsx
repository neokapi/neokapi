import type { Meta, StoryObj } from "@storybook/react";
import { TagChipComponent } from "../../components/editor/TagChipComponent";
import {
  boldOpen, boldClose, italicOpen, italicClose,
  linkOpen, linkClose, codeOpen, codeClose,
  lineBreak, imgTag, underlineOpen, strikeOpen, supOpen,
} from "../fixtures";

const meta: Meta<typeof TagChipComponent> = {
  title: "Editor/TagChipComponent",
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
  args: { spanInfo: boldOpen, index: 1, pairIndex: 1 },
};

export const BoldClosing: Story = {
  args: { spanInfo: boldClose, index: 2, pairIndex: 1 },
};

export const ItalicOpening: Story = {
  args: { spanInfo: italicOpen, index: 1, pairIndex: 1 },
};

export const LinkOpening: Story = {
  args: { spanInfo: linkOpen, index: 1, pairIndex: 1 },
};

export const CodeOpening: Story = {
  args: { spanInfo: codeOpen, index: 1, pairIndex: 1 },
};

export const LineBreak: Story = {
  args: { spanInfo: lineBreak, index: 1 },
};

export const Image: Story = {
  args: { spanInfo: imgTag, index: 1 },
};

export const Highlighted: Story = {
  args: { spanInfo: boldOpen, index: 1, pairIndex: 1, highlighted: true },
};

export const Dimmed: Story = {
  args: { spanInfo: boldOpen, index: 1, pairIndex: 1, dimmed: true },
};

/** All semantic categories side by side */
export const AllCategories: Story = {
  render: () => (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
      <TagChipComponent spanInfo={boldOpen} index={1} pairIndex={1} />
      <TagChipComponent spanInfo={boldClose} index={2} pairIndex={1} />
      <TagChipComponent spanInfo={italicOpen} index={3} pairIndex={2} />
      <TagChipComponent spanInfo={italicClose} index={4} pairIndex={2} />
      <TagChipComponent spanInfo={linkOpen} index={5} pairIndex={3} />
      <TagChipComponent spanInfo={linkClose} index={6} pairIndex={3} />
      <TagChipComponent spanInfo={codeOpen} index={7} pairIndex={4} />
      <TagChipComponent spanInfo={codeClose} index={8} pairIndex={4} />
      <TagChipComponent spanInfo={underlineOpen} index={9} pairIndex={5} />
      <TagChipComponent spanInfo={strikeOpen} index={10} pairIndex={6} />
      <TagChipComponent spanInfo={supOpen} index={11} pairIndex={7} />
      <TagChipComponent spanInfo={lineBreak} index={12} />
      <TagChipComponent spanInfo={imgTag} index={13} />
    </div>
  ),
};
