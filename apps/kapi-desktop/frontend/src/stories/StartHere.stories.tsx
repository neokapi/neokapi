import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StartHere } from "../components/StartHere";

const meta: Meta<typeof StartHere> = {
  title: "Components/StartHere",
  component: StartHere,
  tags: ["autodocs"],
  args: {
    onDismiss: fn(),
    onCreateSampleProject: fn(),
    onConnectAssistant: fn(),
    hasOpenProject: true,
  },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-3xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StartHere>;

/** A project tab is open, so Connect an assistant opens its Assistant section. */
export const WithAProjectOpen: Story = {};

/** No project is open: Connect an assistant waits for one and says so. */
export const NoProjectOpen: Story = { args: { hasOpenProject: false } };
