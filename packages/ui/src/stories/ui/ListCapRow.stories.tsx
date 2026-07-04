import type { Meta, StoryObj } from "@storybook/react-vite";
import { ListCapRow } from "../../components/ui/list-cap-row";

const meta: Meta<typeof ListCapRow> = {
  title: "Foundations/ListCapRow",
  component: ListCapRow,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Honest render-cap footer for capped listings: 'Showing first N of M — refine filters'. Rendered at the foot of a list whenever it truncates, so a large collection never silently drops rows. Returns null when nothing was cut. Mirrors the shared cap contract used by the bowrain kit.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ListCapRow>;

export const Capped: Story = {
  name: "Capped — thousands of files",
  args: {
    shown: 500,
    total: 12_345,
    noun: "files",
    hint: "Refine the glob or filters to narrow the list.",
  },
};

export const NotCapped: Story = {
  name: "Not capped (renders nothing)",
  args: {
    shown: 50,
    total: 50,
    noun: "files",
  },
};

export const ReviewUnits: Story = {
  name: "Capped — review units",
  args: {
    shown: 1000,
    total: 8420,
    noun: "units",
    hint: "Filter by status or locale to reach the rest.",
  },
};
