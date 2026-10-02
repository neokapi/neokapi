import type { Meta, StoryObj } from "@storybook/react-vite";
import PdfExplorer from "../PdfExplorer";

const meta = {
  title: "Lab/PDF explorer",
  component: PdfExplorer,
  parameters: { layout: "padded" },
  // The story shows the launch shell. The docs host supplies engine URLs and
  // published PDF fixtures for an executable experiment.
  args: { assets: null },
} satisfies Meta<typeof PdfExplorer>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Idle: Story = {};
