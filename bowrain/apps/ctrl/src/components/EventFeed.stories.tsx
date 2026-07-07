import type { Meta, StoryObj } from "@storybook/react-vite";
import { EventFeed } from "./EventFeed";
import { sampleEvents } from "../ctrl-fixtures";

const meta: Meta<typeof EventFeed> = {
  title: "Ctrl/EventFeed",
  component: EventFeed,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="w-full max-w-3xl">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof EventFeed>;

export const Default: Story = {
  args: { events: sampleEvents, loading: false },
};

export const Loading: Story = {
  args: { events: [], loading: true },
};

export const Empty: Story = {
  args: { events: [], loading: false },
};
