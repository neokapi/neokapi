import type { Meta, StoryObj } from "@storybook/react-vite";
import { CodedTextDisplay } from "../../components/resource-browser/CodedTextDisplay";
import type { SpanInfo } from "../../types/span";

const meta: Meta<typeof CodedTextDisplay> = {
  title: "Resource Browser/CodedTextDisplay",
  component: CodedTextDisplay,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Renders localization text with inline codes displayed as tag chips. Falls back to plain text when no coded text or spans are provided.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof CodedTextDisplay>;

export const PlainText: Story = {
  args: {
    text: "Welcome to the application",
  },
};

const boldSpans: SpanInfo[] = [
  { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" },
  { span_type: "closing", type: "fmt:bold", id: "1", data: "</b>" },
];

export const BoldAndItalic: Story = {
  args: {
    text: "Click here to continue",
    codedText: "Click \uE001here\uE002 to continue",
    spans: boldSpans,
  },
};

const placeholderSpans: SpanInfo[] = [
  { span_type: "placeholder", type: "entity:person", id: "e1", data: "Alice" },
];

export const Placeholders: Story = {
  args: {
    text: "Alice is a contributor",
    codedText: "\uE003 is a contributor",
    spans: placeholderSpans,
  },
};

const mixedSpans: SpanInfo[] = [
  { span_type: "placeholder", type: "entity:person", id: "e1", data: "Bob" },
  { span_type: "opening", type: "fmt:bold", id: "2", data: "<strong>" },
  { span_type: "closing", type: "fmt:bold", id: "2", data: "</strong>" },
  { span_type: "placeholder", type: "entity:number", id: "e2", data: "42" },
];

export const MixedContent: Story = {
  args: {
    text: "Bob has completed 42 tasks successfully",
    codedText: "\uE003 has \uE001completed\uE002 \uE003 tasks successfully",
    spans: mixedSpans,
  },
};
