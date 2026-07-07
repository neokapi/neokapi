import type { Meta, StoryObj } from "@storybook/react-vite";
import { withRouter } from "@neokapi/storybook-config/preview";
import { CtrlShell } from "../components/CtrlShell";
import { DashboardRoute } from "./dashboard";
import { WorkspacesRoute } from "./workspaces";
import { UsersRoute } from "./users";
import { EventsRoute } from "./events";
import { OverridesRoute } from "./overrides";
import { UpsellsRoute } from "./upsells";
import { SlugReservationsRoute } from "./slug-reservations";
import { WorkspaceDetailRoute } from "./workspace-detail";

// The ctrl admin pages are thin data-fetching wrappers over the storied
// tables/cards. These stories mount each page inside the real CtrlShell (which
// supplies the SidebarProvider + BreadcrumbProvider the pages need) and the
// shared withRouter decorator (for useNavigate/useSearch/useUrlFilters). No
// admin backend is present, so the react-query fetches settle into the page's
// empty/loading state — exactly the cold-start surface, rendered in context.

function withCtrlShell(Story: React.ComponentType) {
  return (
    <CtrlShell>
      <Story />
    </CtrlShell>
  );
}

const meta: Meta = {
  title: "Ctrl/Pages",
  parameters: { layout: "fullscreen" },
  // withRouter is outermost so the shell (and its router-aware sidebar) and the
  // page both resolve router hooks; withCtrlShell wraps the page in the shell.
  decorators: [withRouter, withCtrlShell],
};

export default meta;
type Story = StoryObj;

export const Dashboard: Story = { render: () => <DashboardRoute /> };
export const Workspaces: Story = { render: () => <WorkspacesRoute /> };
export const Users: Story = { render: () => <UsersRoute /> };
export const Events: Story = { render: () => <EventsRoute /> };
export const Overrides: Story = { render: () => <OverridesRoute /> };
export const Upsells: Story = { render: () => <UpsellsRoute /> };
export const SlugReservations: Story = { render: () => <SlugReservationsRoute /> };
export const WorkspaceDetail: Story = { render: () => <WorkspaceDetailRoute /> };
