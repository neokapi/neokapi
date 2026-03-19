import { useLocation, useNavigate } from "@tanstack/react-router";
import { LayoutDashboard, Building2, Users, Activity, ToggleLeft, TrendingUp } from "lucide-react";
import { cn } from "@neokapi/ui";

interface NavItem {
  path: string;
  label: string;
  icon: React.ReactNode;
}

const navItems: NavItem[] = [
  { path: "/", label: "Dashboard", icon: <LayoutDashboard className="h-4 w-4" /> },
  { path: "/workspaces", label: "Workspaces", icon: <Building2 className="h-4 w-4" /> },
  { path: "/users", label: "Users", icon: <Users className="h-4 w-4" /> },
  { path: "/events", label: "Events", icon: <Activity className="h-4 w-4" /> },
  { path: "/overrides", label: "Overrides", icon: <ToggleLeft className="h-4 w-4" /> },
  { path: "/upsells", label: "Upsells", icon: <TrendingUp className="h-4 w-4" /> },
];

export function AdminSidebar() {
  const navigate = useNavigate();
  const { pathname } = useLocation();

  function isActive(path: string) {
    if (path === "/") return pathname === "/";
    return pathname.startsWith(path);
  }

  return (
    <aside className="flex w-56 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground">
      <div className="flex h-12 items-center px-4 border-b">
        <span className="text-sm font-semibold">Control Plane</span>
      </div>
      <nav className="flex-1 px-2 py-2">
        <ul className="flex flex-col gap-0.5">
          {navItems.map((item) => (
            <li key={item.path}>
              <button
                onClick={() => void navigate({ to: item.path })}
                className={cn(
                  "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors outline-none border-none cursor-pointer bg-transparent",
                  isActive(item.path)
                    ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                    : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                )}
              >
                {item.icon}
                <span>{item.label}</span>
              </button>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  );
}
