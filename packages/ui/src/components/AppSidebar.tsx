import type { Workspace, User } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Globe, BookOpen, Brain, Settings, Sun, Moon, Monitor } from "./icons";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";
import { AccountMenu } from "./AccountMenu";
import { SidebarGlass } from "./ui/sidebar";

export type View = "translate" | "termbase" | "memory" | "settings";

export interface NavItem {
  id: string;
  label: string;
  icon: React.ReactNode;
}

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

export interface AppSidebarProps<V extends string = string> {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  onSelectWorkspace: (ws: Workspace) => void;
  onCreateWorkspace?: () => void;
  activeView: V;
  onViewChange: (view: V) => void;
  extraNavItems?: NavItem[];
  user: User | null;
  onSignOut?: () => void;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  topSpacer?: number;
  collapsedWidth?: number;
  connectionState?: ConnectionState;
  pendingChanges?: number;
  /** Show theme toggle in sidebar footer (default true for web, false for desktop). */
  showThemeToggle?: boolean;
}

const defaultNavItems: NavItem[] = [
  { id: "translate", label: "Translate", icon: <Globe className="w-4 h-4" /> },
  { id: "termbase", label: "Termbase", icon: <BookOpen className="w-4 h-4" /> },
  { id: "memory", label: "Memory", icon: <Brain className="w-4 h-4" /> },
  { id: "settings", label: "Settings", icon: <Settings className="w-4 h-4" /> },
];

export function AppSidebar<V extends string = string>({
  workspaces,
  activeWorkspace,
  onSelectWorkspace,
  onCreateWorkspace,
  activeView,
  onViewChange,
  extraNavItems = [],
  user,
  onSignOut,
  topSpacer = 0,
  showThemeToggle = true,
}: AppSidebarProps<V>) {
  const navItems = [
    ...defaultNavItems.slice(0, -1),
    ...extraNavItems,
    defaultNavItems[defaultNavItems.length - 1],
  ];

  return (
    <SidebarGlass.Root side="left" collapsible="icon">
      {/* Top spacer (e.g. macOS traffic lights) */}
      {topSpacer > 0 && <div style={{ height: topSpacer }} className="shrink-0" />}

      <SidebarGlass.Header className="border-none px-3 py-2.5">
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeWorkspace={activeWorkspace}
          onSelectWorkspace={onSelectWorkspace}
          onCreateWorkspace={onCreateWorkspace}
        />
      </SidebarGlass.Header>

      <SidebarGlass.Content className="px-2 py-2">
        <SidebarGlass.Group>
          <SidebarGlass.GroupContent>
            <SidebarGlass.Menu>
              {navItems.map(({ id, label, icon }) => (
                <SidebarGlass.MenuItem key={id}>
                  <SidebarGlass.MenuButton
                    data-testid={`nav-${id}`}
                    isActive={activeView === id}
                    tooltip={label}
                    onClick={() => onViewChange(id as V)}
                  >
                    {icon}
                    <span>{label}</span>
                  </SidebarGlass.MenuButton>
                </SidebarGlass.MenuItem>
              ))}
            </SidebarGlass.Menu>
          </SidebarGlass.GroupContent>
        </SidebarGlass.Group>
      </SidebarGlass.Content>

      <SidebarGlass.Footer>
        {showThemeToggle && <ThemeToggle />}
        {user && onSignOut && (
          <AccountMenu user={user} onSignOut={onSignOut} variant="sidebar" />
        )}
      </SidebarGlass.Footer>

      <SidebarGlass.Rail />
    </SidebarGlass.Root>
  );
}

// ---------------------------------------------------------------------------
// Internal: ThemeToggle
// ---------------------------------------------------------------------------

const nextTheme: Record<Theme, Theme> = { light: "dark", dark: "system", system: "light" };
const themeIcons: Record<Theme, React.ReactNode> = {
  light: <Sun className="w-4 h-4" />,
  dark: <Moon className="w-4 h-4" />,
  system: <Monitor className="w-4 h-4" />,
};
const themeLabels: Record<Theme, string> = {
  light: "Light",
  dark: "Dark",
  system: "System",
};

function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  return (
    <SidebarGlass.MenuButton
      data-testid="theme-toggle"
      onClick={() => setTheme(nextTheme[theme])}
      tooltip={`Theme: ${themeLabels[theme]}`}
      size="sm"
    >
      {themeIcons[theme]}
      <span>{themeLabels[theme]}</span>
    </SidebarGlass.MenuButton>
  );
}
