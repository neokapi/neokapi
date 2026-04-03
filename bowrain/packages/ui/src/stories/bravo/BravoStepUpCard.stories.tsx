import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoStepUpCard } from "../../components/bravo/BravoStepUpCard";

const meta: Meta<typeof BravoStepUpCard> = {
  title: "Bravo/BravoStepUpCard",
  component: BravoStepUpCard,
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
type Story = StoryObj<typeof BravoStepUpCard>;

export const Default: Story = {
  args: {
    currentMode: "ask",
    requiredMode: "coworker",
    action: "Editing translations",
    onSwitchMode: fn(),
    onDismiss: fn(),
  },
};

export const VoiceToCoworker: Story = {
  args: {
    currentMode: "voice",
    requiredMode: "coworker",
    action: "Pushing to connector",
    onSwitchMode: fn(),
    onDismiss: fn(),
  },
};

export const LongAction: Story = {
  args: {
    currentMode: "ask",
    requiredMode: "bravo",
    action:
      "Running automated quality checks and applying batch corrections across all project files",
    onSwitchMode: fn(),
    onDismiss: fn(),
  },
};
