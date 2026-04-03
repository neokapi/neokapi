import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleCompletionChart } from "../../components/LocaleCompletionChart";
import { sampleLocaleStats } from "./fixtures";

const meta: Meta<typeof LocaleCompletionChart> = {
  title: "Components/LocaleCompletionChart",
  component: LocaleCompletionChart,
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
type Story = StoryObj<typeof LocaleCompletionChart>;

export const Default: Story = {
  args: { localeStats: sampleLocaleStats },
};
