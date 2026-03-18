import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { BinView } from "../../components/BinView";
import { sampleArchivedProjects } from "./fixtures";

const meta: Meta<typeof BinView> = {
  title: "Components/BinView",
  component: BinView,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof BinView>;

export const Empty: Story = {
  args: { projects: [], onRestoreProject: fn(), onPermanentlyDelete: fn() },
};

export const WithProjects: Story = {
  args: { projects: sampleArchivedProjects, onRestoreProject: fn(), onPermanentlyDelete: fn() },
};

export const Loading: Story = {
  args: { projects: [], loading: true, onRestoreProject: fn(), onPermanentlyDelete: fn() },
};

export const CustomRetention: Story = {
  args: {
    projects: sampleArchivedProjects,
    retentionDays: 7,
    onRestoreProject: fn(),
    onPermanentlyDelete: fn(),
  },
};
