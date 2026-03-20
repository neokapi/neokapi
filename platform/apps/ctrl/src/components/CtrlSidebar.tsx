import { useLocation, useNavigate } from "@tanstack/react-router";
import {
  LayoutDashboard,
  Building2,
  Users,
  Activity,
  ToggleLeft,
  TrendingUp,
  Shield,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@neokapi/ui";

interface NavItem {
  path: string;
  label: string;
  icon: React.ReactNode;
}

const navItems: NavItem[] = [
  { path: "/", label: "Dashboard", icon: <LayoutDashboard /> },
  { path: "/workspaces", label: "Workspaces", icon: <Building2 /> },
  { path: "/users", label: "Users", icon: <Users /> },
  { path: "/events", label: "Events", icon: <Activity /> },
  { path: "/overrides", label: "Overrides", icon: <ToggleLeft /> },
  { path: "/upsells", label: "Upsells", icon: <TrendingUp /> },
];

function isActive(pathname: string, path: string) {
  if (path === "/") return pathname === "/";
  return pathname.startsWith(path);
}

function DesktopNav({
  pathname,
  onNavigate,
}: {
  pathname: string;
  onNavigate: (path: string) => void;
}) {
  return (
    <SidebarGroup>
      <SidebarGroupContent>
        <SidebarMenu>
          {navItems.map((item) => (
            <SidebarMenuItem key={item.path}>
              <SidebarMenuButton
                tooltip={item.label}
                isActive={isActive(pathname, item.path)}
                onClick={() => onNavigate(item.path)}
              >
                {item.icon}
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

function MobileNav({
  pathname,
  onNavigate,
}: {
  pathname: string;
  onNavigate: (path: string) => void;
}) {
  const { setOpenMobile } = useSidebar();

  const handleNav = (path: string) => {
    onNavigate(path);
    setOpenMobile(false);
  };

  return (
    <SidebarGroup>
      <SidebarGroupLabel>Admin</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {navItems.map((item) => (
            <SidebarMenuItem key={item.path}>
              <SidebarMenuButton
                isActive={isActive(pathname, item.path)}
                onClick={() => handleNav(item.path)}
              >
                {item.icon}
                <span>{item.label}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

export function CtrlSidebar() {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { isMobile } = useSidebar();

  const handleNavigate = (path: string) => {
    void navigate({ to: path });
  };

  if (isMobile) {
    return (
      <Sidebar collapsible="offcanvas">
        <SidebarHeader className="flex h-12 items-center px-4 border-b">
          <div className="flex items-center gap-2">
            <Shield className="size-4" />
            <span className="text-sm font-semibold">Control Plane</span>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <MobileNav pathname={pathname} onNavigate={handleNavigate} />
        </SidebarContent>
      </Sidebar>
    );
  }

  return (
    <Sidebar collapsible="none" className="!w-(--sidebar-width-icon)">
      <SidebarHeader className="flex items-center justify-center py-3">
        <Shield className="size-5 text-sidebar-foreground/70" />
      </SidebarHeader>
      <SidebarContent className="[&_[data-slot=sidebar-menu]]:gap-1 [&_[data-slot=sidebar-menu-button]]:justify-center [&_[data-slot=sidebar-menu-button]]:aspect-square [&_[data-slot=sidebar-menu-button]]:p-0 [&_[data-slot=sidebar-menu-button]_svg]:size-5 [&_svg]:stroke-[1.5]">
        <DesktopNav pathname={pathname} onNavigate={handleNavigate} />
      </SidebarContent>
    </Sidebar>
  );
}
