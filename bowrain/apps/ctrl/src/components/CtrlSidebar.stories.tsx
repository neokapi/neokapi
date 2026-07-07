import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarProvider } from "@neokapi/ui";
import { withRouter } from "@neokapi/storybook-config/preview";
import { CtrlSidebar } from "./CtrlSidebar";

// CtrlSidebar reads router state (useNavigate/useLocation) and sidebar state
// (useSidebar), so it needs both a router and a SidebarProvider in the tree.
// The shared withRouter decorator (innermost) mounts it as a throwaway
// in-memory router; SidebarProvider (outermost) supplies the collapse state.

const meta: Meta<typeof CtrlSidebar> = {
  title: "Ctrl/CtrlSidebar",
  component: CtrlSidebar,
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof CtrlSidebar>;

export const Expanded: Story = {
  decorators: [
    (Story) => (
      <SidebarProvider>
        <div className="h-[80vh]">
          <Story />
        </div>
      </SidebarProvider>
    ),
    withRouter,
  ],
};

export const Collapsed: Story = {
  decorators: [
    (Story) => (
      <SidebarProvider defaultOpen={false}>
        <div className="h-[80vh]">
          <Story />
        </div>
      </SidebarProvider>
    ),
    withRouter,
  ],
};
