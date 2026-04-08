import type { Meta, StoryObj } from "@storybook/react-vite";
import { InlinePreview } from "../../components/editor/InlinePreview";
import {
  simpleBoldCodedText,
  simpleBoldSpans,
  linkAndItalicCodedText,
  linkAndItalicSpans,
  richCodedText,
  richSpans,
} from "./fixtures";

const meta: Meta<typeof InlinePreview> = {
  title: "Editor/Core/InlinePreview",
  component: InlinePreview,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 500, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof InlinePreview>;

export const Bold: Story = {
  args: { codedText: simpleBoldCodedText, spans: simpleBoldSpans },
};

export const LinksAndItalic: Story = {
  args: { codedText: linkAndItalicCodedText, spans: linkAndItalicSpans },
};

export const Rich: Story = {
  args: { codedText: richCodedText, spans: richSpans },
};

export const PlainText: Story = {
  args: { codedText: "Hello world with no inline codes", spans: [] },
};

export const Empty: Story = {
  args: { codedText: "", spans: [] },
};
