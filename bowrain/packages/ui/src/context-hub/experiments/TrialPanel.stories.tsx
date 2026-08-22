import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { TrialPanel } from "./TrialPanel";
import { withContextHub } from "../../stories/voiceHubFixtures";

const pad: Decorator = (Story) => (
  <div style={{ maxWidth: 720, padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof TrialPanel> = {
  title: "Context/Experiments/TrialPanel",
  component: TrialPanel,
  tags: ["autodocs"],
  decorators: [withContextHub, pad],
  args: {
    changesetId: "cs-1",
    projectId: "p-web",
    stream: "trial/retire-utilise",
    projectName: "Marketing Website",
  },
};

export default meta;
type Story = StoryObj<typeof TrialPanel>;

/** The mock adapter answers with one raised and one cleared finding. */
export const Default: Story = {};
