import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MobileWorkspaceSwitcher } from "../../components/MobileWorkspaceSwitcher";
import { SidebarProvider } from "../../components/ui/sidebar";
import { sampleWorkspace, personalWorkspace } from "./fixtures";

const meta: Meta<typeof MobileWorkspaceSwitcher> = {
  title: "Layout/MobileWorkspaceSwitcher",
  component: MobileWorkspaceSwitcher,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <SidebarProvider>
        <div style={{ width: 260, padding: 8, border: "1px solid var(--border)", borderRadius: 8 }}>
          <Story />
        </div>
      </SidebarProvider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof MobileWorkspaceSwitcher>;

export const Default: Story = {
  args: {
    workspaces: [sampleWorkspace, personalWorkspace],
    activeWorkspace: sampleWorkspace,
    onSelectWorkspace: fn(),
    onCreateWorkspace: fn(),
  },
};
