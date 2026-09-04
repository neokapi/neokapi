import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TabBar } from "../components/TabBar";

const meta: Meta<typeof TabBar> = {
  title: "Components/TabBar",
  component: TabBar,
  tags: ["autodocs"],
  args: {
    onSelect: fn(),
    onClose: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ width: 800 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TabBar>;

export const SingleTab: Story = {
  args: {
    tabs: [{ id: "1", name: "translation", path: "/Users/dev/translation/kapi.yaml" }],
    activeTabID: "1",
  },
};

export const MultipleTabs: Story = {
  args: {
    tabs: [
      { id: "1", name: "translation", path: "/Users/dev/translation/kapi.yaml" },
      { id: "2", name: "check-pipeline", path: "/Users/dev/check-pipeline/kapi.yaml" },
      { id: "3", name: "New Project", path: "" },
    ],
    activeTabID: "2",
  },
};

export const ManyTabs: Story = {
  args: {
    tabs: Array.from({ length: 8 }, (_, i) => ({
      id: String(i + 1),
      name: `project-${i + 1}`,
      path: `/tmp/project-${i + 1}/kapi.yaml`,
    })),
    activeTabID: "3",
  },
};

export const NoTabs: Story = {
  args: {
    tabs: [],
    activeTabID: null,
  },
};
