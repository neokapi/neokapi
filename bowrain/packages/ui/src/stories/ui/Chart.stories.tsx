import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Bar, BarChart, XAxis, YAxis } from "recharts";

const meta: Meta<typeof ChartContainer> = {
  title: "Foundations/Chart",
  component: ChartContainer,
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
type Story = StoryObj<typeof ChartContainer>;

const data = [
  { month: "Jan", translated: 186, established: 80 },
  { month: "Feb", translated: 305, established: 200 },
  { month: "Mar", translated: 237, established: 120 },
  { month: "Apr", translated: 73, established: 190 },
  { month: "May", translated: 209, established: 130 },
];

const config: ChartConfig = {
  translated: { label: "Translated", color: "var(--chart-1)" },
  established: { label: "Established", color: "var(--chart-2)" },
};

export const Default: Story = {
  render: () => (
    <ChartContainer config={config}>
      <BarChart data={data}>
        <XAxis dataKey="month" />
        <YAxis />
        <Bar dataKey="translated" fill="var(--color-translated)" radius={4} />
        <Bar dataKey="established" fill="var(--color-established)" radius={4} />
      </BarChart>
    </ChartContainer>
  ),
};

export const WithTooltipAndLegend: Story = {
  render: () => (
    <ChartContainer config={config}>
      <BarChart data={data}>
        <XAxis dataKey="month" />
        <YAxis />
        <ChartTooltip content={<ChartTooltipContent />} />
        <ChartLegend content={<ChartLegendContent />} />
        <Bar dataKey="translated" fill="var(--color-translated)" radius={4} />
        <Bar dataKey="established" fill="var(--color-established)" radius={4} />
      </BarChart>
    </ChartContainer>
  ),
};
