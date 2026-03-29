import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToolRunnerPage } from "../components/ToolRunnerPage";

const meta: Meta<typeof ToolRunnerPage> = {
  title: "Pages/ToolRunnerPage",
  component: ToolRunnerPage,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <div style={{ height: 600 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToolRunnerPage>;

export const Default: Story = {};
