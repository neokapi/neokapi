import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BravoModeSelector } from "../../components/bravo/BravoModeSelector";

const meta: Meta<typeof BravoModeSelector> = {
  title: "Bravo/BravoModeSelector",
  component: BravoModeSelector,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 320, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BravoModeSelector>;

export const AskMode: Story = {
  args: {
    mode: "ask",
    onChange: fn(),
  },
};

export const CoworkerMode: Story = {
  args: {
    mode: "coworker",
    onChange: fn(),
  },
};

export const BravoMode: Story = {
  args: {
    mode: "bravo",
    onChange: fn(),
  },
};
