import type { Meta, StoryObj } from "@storybook/react-vite";
import { TagChipComponent } from "../../components/editor/TagChipComponent";
import { boldOpen, linkOpen, codeOpen, lineBreak, imgTag } from "../fixtures";

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
    spanInfo: boldOpen,
    locked: true,
  },
};

/** An unlocked tag — deletable spans render without the dashed border. */
export const UnlockedLink: Story = {
  args: {
    spanInfo: linkOpen,
    locked: false,
  },
};

/** Placeholder tags (self-closing) are always rendered as locked by TagChipNode. */
export const LockedLineBreak: Story = {
  args: {
    spanInfo: lineBreak,
    locked: true,
  },
};

/** All node variants side by side showing locked vs unlocked rendering. */
export const AllVariants: Story = {
  render: () => (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
      <TagChipComponent spanInfo={boldOpen} locked />
      <TagChipComponent spanInfo={boldOpen} locked={false} />
      <TagChipComponent spanInfo={linkOpen} locked />
      <TagChipComponent spanInfo={linkOpen} locked={false} />
      <TagChipComponent spanInfo={codeOpen} locked />
      <TagChipComponent spanInfo={codeOpen} locked={false} />
      <TagChipComponent spanInfo={lineBreak} locked />
      <TagChipComponent spanInfo={imgTag} locked />
    </div>
  ),
};
