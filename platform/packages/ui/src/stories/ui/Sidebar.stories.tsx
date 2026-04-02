import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarInset,
  SidebarTrigger,
} from "@neokapi/ui-primitives/components/ui/sidebar";

const meta: Meta<typeof Sidebar> = {
  title: "Foundations/Sidebar",
  component: Sidebar,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

const items = [
  { title: "Dashboard", url: "#" },
  { title: "Projects", url: "#" },
  { title: "Translation Memory", url: "#" },
  { title: "Terminology", url: "#" },
  { title: "Settings", url: "#" },
];

export const Default: Story = {
  render: () => (
    <SidebarProvider>
      <Sidebar>
        <SidebarHeader>
          <span className="px-2 text-sm font-semibold">Bowrain</span>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Workspace</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {items.map((item) => (
                  <SidebarMenuItem key={item.title}>
                    <SidebarMenuButton>
                      <span>{item.title}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          <span className="px-2 text-xs text-muted-foreground">v1.0.0</span>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <header className="flex h-12 items-center gap-2 border-b px-4">
          <SidebarTrigger />
          <span className="text-sm font-medium">Main Content</span>
        </header>
        <div className="p-4">
          <p className="text-sm text-muted-foreground">Page content goes here.</p>
        </div>
      </SidebarInset>
    </SidebarProvider>
  ),
};
