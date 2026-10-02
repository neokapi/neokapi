import React from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { LabLaunch } from "./LabLaunch";

const meta = {
  title: "Labs/Launch",
  component: LabLaunch,
  parameters: { layout: "padded" },
  args: { children: <div className="rounded-lg border p-6">Experiment workspace</div> },
} satisfies Meta<typeof LabLaunch>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Closed: Story = {};
export const Terminal: Story = {
  args: {
    label: "Open terminal",
    description:
      "Opening the terminal downloads and starts the browser engine. Select a sample, then press Enter to run its command.",
  },
};
function Unavailable(): React.ReactElement {
  throw new Error("Example experiment failure");
}
export const Recovery: Story = {
  args: { children: <Unavailable /> },
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole("button", { name: "Open experiment" }));
  },
};
const Pending = React.lazy(() => new Promise<{ default: React.ComponentType }>(() => {}));
export const Loading: Story = {
  args: { children: <Pending /> },
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole("button", { name: "Open experiment" }));
  },
};
