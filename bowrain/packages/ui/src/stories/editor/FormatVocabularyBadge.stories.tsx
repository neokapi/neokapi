import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FormatVocabularyBadge } from "../../components/editor/FormatVocabularyBadge";
import {
  boldOpen,
  boldClose,
  italicOpen,
  linkOpen,
  linkClose,
  codeOpen,
  lineBreak,
  imgTag,
} from "../fixtures";

const meta: Meta<typeof FormatVocabularyBadge> = {
  title: "Editor/Tags/FormatVocabularyBadge",
  component: FormatVocabularyBadge,
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
type Story = StoryObj<typeof FormatVocabularyBadge>;

export const Default: Story = {
  args: {
    runs: [boldOpen, boldClose, italicOpen],
    onClick: fn(),
  },
};

export const WithMultipleSpans: Story = {
  args: {
    runs: [boldOpen, boldClose, italicOpen, linkOpen, linkClose, codeOpen, lineBreak, imgTag],
    onClick: fn(),
  },
};

export const Empty: Story = {
  args: {
    runs: [],
    onClick: fn(),
  },
};
