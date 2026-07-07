import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@neokapi/ui";
import { withRouter } from "@neokapi/storybook-config/preview";
import { CtrlShell } from "./CtrlShell";

// CtrlShell owns its SidebarProvider + BreadcrumbProvider, but its embedded
// CtrlSidebar reads router state — so the shell still needs a router in the
// tree. withRouter (innermost) supplies it.

const meta: Meta<typeof CtrlShell> = {
  title: "Ctrl/CtrlShell",
  component: CtrlShell,
  parameters: { layout: "fullscreen" },
  decorators: [withRouter],
};

export default meta;
type Story = StoryObj<typeof CtrlShell>;

export const Default: Story = {
  args: {
    children: (
      <div className="mx-auto w-full max-w-4xl space-y-4">
        <h2 className="text-lg font-semibold">Dashboard</h2>
        <p className="text-sm text-muted-foreground">
          Page content renders inside the shell, beside the collapsible admin sidebar.
        </p>
      </div>
    ),
  },
};

export const WithHeaderAction: Story = {
  args: {
    headerSlot: <Button size="sm">New workspace</Button>,
    children: (
      <div className="mx-auto w-full max-w-4xl">
        <p className="text-sm text-muted-foreground">Content area with a header action slot.</p>
      </div>
    ),
  },
};
