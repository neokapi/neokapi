import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StartHere } from "../components/StartHere";
import { DesktopGuide } from "../guide/DesktopGuide";

const meta: Meta<typeof StartHere> = {
  title: "Components/StartHere",
  component: StartHere,
  tags: ["autodocs"],
  args: {
    onDismiss: fn(),
    onCreateSampleProject: fn(),
    hasOpenProject: true,
  },
  decorators: [
    (Story) => (
      <DesktopGuide navigate={fn()} hasProject onOpenUrl={fn()}>
        <div className="mx-auto max-w-3xl p-6">
          <Story />
        </div>
      </DesktopGuide>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StartHere>;

/** A project tab is open, so every tour can start. */
export const WithAProjectOpen: Story = {};

/** No project is open: the project tours wait for one and say so. */
export const NoProjectOpen: Story = { args: { hasOpenProject: false } };
