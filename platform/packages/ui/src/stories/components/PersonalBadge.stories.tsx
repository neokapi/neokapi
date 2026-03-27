import type { Meta, StoryObj } from "@storybook/react-vite";
import { PersonalBadge } from "../../components/PersonalBadge";

const meta: Meta<typeof PersonalBadge> = {
  title: "Components/PersonalBadge",
  component: PersonalBadge,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24, display: "flex", gap: 16, alignItems: "center" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof PersonalBadge>;

export const Default: Story = {};

export const InContext: Story = {
  render: () => (
    <div className="flex items-center gap-2 text-sm">
      <span>Personal</span>
      <PersonalBadge />
    </div>
  ),
};
