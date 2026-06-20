import type { Meta, StoryObj } from "@storybook/react-vite";
import { MultimodalShowcase } from "@neokapi/kapi-lab";

// A pre-recorded (canned-data) walkthrough of the multimodal localization story —
// image OCR, audio subtitles, video — that plays anywhere with no engine, model
// download, or ffmpeg. The reliable companion to the live in-browser labs.
const meta: Meta<typeof MultimodalShowcase> = {
  title: "Lab/Explorers/MultimodalShowcase",
  component: MultimodalShowcase,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MultimodalShowcase>;

export const Default: Story = {
  render: () => <MultimodalShowcase className="max-w-3xl" />,
};

export const StartOnVideo: Story = {
  render: () => <MultimodalShowcase initialChapter={2} className="max-w-3xl" />,
};
