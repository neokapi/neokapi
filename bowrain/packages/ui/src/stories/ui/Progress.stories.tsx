import { Progress } from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta: Meta<typeof Progress> = {
  title: "Foundations/Progress",
  component: Progress,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Progress>;

export const Default: Story = {
  render: () => (
    <div className="flex flex-col gap-6">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">Translation Progress — 0%</p>
        <Progress value={0} />
      </div>
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">Translation Progress — 25%</p>
        <Progress value={25} />
      </div>
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">Translation Progress — 50%</p>
        <Progress value={50} />
      </div>
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">Translation Progress — 75%</p>
        <Progress value={75} />
      </div>
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">Translation Progress — 100%</p>
        <Progress value={100} />
      </div>
    </div>
  ),
};
