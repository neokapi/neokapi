import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Run } from "@neokapi/kapi-format";
import { CodedTextDisplay } from "../../components/resource-browser/CodedTextDisplay";

const meta: Meta<typeof CodedTextDisplay> = {
  title: "Resource Browser/CodedTextDisplay",
  component: CodedTextDisplay,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Renders a localization Run sequence as text interleaved with inline-code chips. Falls back to plain text when no runs are provided.",
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

const boldRuns: Run[] = [
  { text: "Click " },
  { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
  { text: "here" },
  { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
  { text: " to continue" },
];

export const BoldAndItalic: Story = {
  args: {
    text: "Click here to continue",
    runs: boldRuns,
  },
};

const placeholderRuns: Run[] = [
  { ph: { id: "e1", type: "entity:person", data: "Alice", equiv: "Alice" } },
  { text: " is a contributor" },
];

export const Placeholders: Story = {
  args: {
    text: "Alice is a contributor",
    runs: placeholderRuns,
  },
};

const mixedRuns: Run[] = [
  { ph: { id: "e1", type: "entity:person", data: "Bob", equiv: "Bob" } },
  { text: " has " },
  { pcOpen: { id: "2", type: "fmt:bold", data: "<strong>", equiv: "strong" } },
  { text: "completed" },
  { pcClose: { id: "2", type: "fmt:bold", data: "</strong>", equiv: "strong" } },
  { text: " " },
  { ph: { id: "e2", type: "entity:number", data: "42", equiv: "42" } },
  { text: " tasks successfully" },
];

export const MixedContent: Story = {
  args: {
    text: "Bob has completed 42 tasks successfully",
    runs: mixedRuns,
  },
};
