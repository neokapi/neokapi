import type { Meta, StoryObj } from "@storybook/react-vite";
import { TagChipComponent } from "@neokapi/ui-primitives";
import { boldOpen, linkOpen, codeOpen, lineBreak, imgTag, chip } from "../fixtures";

/**
 * TagChipNode is a Lexical DecoratorNode that renders a TagChipComponent.
 * Since Lexical nodes require an editor context, these stories render the
 * visual output directly via TagChipComponent with the `locked` prop that
 * TagChipNode passes through its `decorate()` method.
 */
const meta: Meta<typeof TagChipComponent> = {
  title: "Editor/Tags/TagChipNode",
  component: TagChipComponent,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TagChipComponent>;

/** A locked tag — TagChipNode sets locked=true when the span is non-deletable. */
export const LockedBold: Story = {
  args: {
    spanInfo: chip(boldOpen),
    locked: true,
  },
};

/** An unlocked tag — deletable spans render without the dashed border. */
export const UnlockedLink: Story = {
  args: {
    spanInfo: chip(linkOpen),
    locked: false,
  },
};

/** Placeholder tags (self-closing) are always rendered as locked by TagChipNode. */
export const LockedLineBreak: Story = {
  args: {
    spanInfo: chip(lineBreak),
    locked: true,
  },
};

/** All node variants side by side showing locked vs unlocked rendering. */
export const AllVariants: Story = {
  render: () => (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
      <TagChipComponent spanInfo={chip(boldOpen)} locked />
      <TagChipComponent spanInfo={chip(boldOpen)} locked={false} />
      <TagChipComponent spanInfo={chip(linkOpen)} locked />
      <TagChipComponent spanInfo={chip(linkOpen)} locked={false} />
      <TagChipComponent spanInfo={chip(codeOpen)} locked />
      <TagChipComponent spanInfo={chip(codeOpen)} locked={false} />
      <TagChipComponent spanInfo={chip(lineBreak)} locked />
      <TagChipComponent spanInfo={chip(imgTag)} locked />
    </div>
  ),
};
