import type { Meta, StoryObj } from "@storybook/react-vite";
import { WordCountChart } from "../../components/WordCountChart";
import { sampleLocaleStats } from "./fixtures";

const meta: Meta<typeof WordCountChart> = {
  title: "Misc/WordCountChart",
  component: WordCountChart,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 600, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WordCountChart>;

export const Default: Story = {
  args: { localeStats: sampleLocaleStats },
};
