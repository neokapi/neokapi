import type { Meta, StoryObj } from "@storybook/react";
import { TrendAreaChart } from "../../components/pulse";
import { mockActivityData } from "./pulse-fixtures";

const meta: Meta<typeof TrendAreaChart> = {
  title: "Pulse/TrendAreaChart",
  component: TrendAreaChart,
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof TrendAreaChart>;

export const Default: Story = { args: { data: mockActivityData } };
export const Empty: Story = { args: { data: [] } };
export const Tall: Story = { args: { data: mockActivityData, height: 400 } };
