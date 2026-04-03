import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { cn } from "@neokapi/ui-primitives";

interface NavItem {
  path: string;
  label: string;
}

const navItems: NavItem[] = [
  { path: "/", label: "Dashboard" },
  { path: "/workspaces", label: "Workspaces" },
  { path: "/users", label: "Users" },
  { path: "/events", label: "Events" },
  { path: "/overrides", label: "Overrides" },
  { path: "/upsells", label: "Upsells" },
];

interface AdminSidebarProps {
  activePath: string;
  onNavigate?: (path: string) => void;
}

function AdminSidebar({ activePath, onNavigate }: AdminSidebarProps) {
  function isActive(path: string) {
    if (path === "/") return activePath === "/";
    return activePath.startsWith(path);
  }

  return (
    <aside className="flex w-56 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground h-[400px]">
      <div className="flex h-12 items-center px-4 border-b">
        <span className="text-sm font-semibold">Control Plane</span>
      </div>
      <nav className="flex-1 px-2 py-2">
        <ul className="flex flex-col gap-0.5">
          {navItems.map((item) => (
            <li key={item.path}>
              <button
                onClick={() => onNavigate?.(item.path)}
                className={cn(
                  "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors outline-none border-none cursor-pointer bg-transparent",
                  isActive(item.path)
                    ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                    : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                )}
              >
                <span>{item.label}</span>
              </button>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  );
}

const meta: Meta<typeof AdminSidebar> = {
  title: "Ctrl/AdminSidebar",
  component: AdminSidebar,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof AdminSidebar>;

export const Dashboard: Story = {
  args: { activePath: "/" },
};

export const Workspaces: Story = {
  args: { activePath: "/workspaces" },
};

export const WorkspaceDetail: Story = {
  args: { activePath: "/workspaces/ws-123" },
};

export const Users: Story = {
  args: { activePath: "/users" },
};

export const Events: Story = {
  args: { activePath: "/events" },
};

export const Overrides: Story = {
  args: { activePath: "/overrides" },
};

export const Upsells: Story = {
  args: { activePath: "/upsells" },
};

export const Interactive: Story = {
  render: () => {
    const [active, setActive] = useState("/");
    return <AdminSidebar activePath={active} onNavigate={setActive} />;
  },
};
