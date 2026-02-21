import type { User } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Sun, Moon, Monitor, Bell } from "./icons";
import { AccountMenu } from "./AccountMenu";

export interface TopBarProps {
  user: User | null;
  onSignOut?: () => void;
}

const nextTheme: Record<Theme, Theme> = { light: "dark", dark: "system", system: "light" };
const themeIcons: Record<Theme, React.ReactNode> = {
  light: <Sun className="w-4 h-4" />,
  dark: <Moon className="w-4 h-4" />,
  system: <Monitor className="w-4 h-4" />,
};
const themeLabels: Record<Theme, string> = { light: "Light", dark: "Dark", system: "System" };

const iconBtnClass =
  "flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-colors";

export function TopBar({ user, onSignOut }: TopBarProps) {
  const { theme, setTheme } = useTheme();

  return (
    <div
      className="h-10 shrink-0 flex items-center justify-end gap-1 px-3"
    >
      {/* Notification bell (placeholder) */}
      <button
        className={iconBtnClass}
        title="Notifications"
        style={{ color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)" }}
        onMouseEnter={(e) => { e.currentTarget.style.color = "var(--semantic-text)"; }}
        onMouseLeave={(e) => { e.currentTarget.style.color = "color-mix(in srgb, var(--semantic-text) 60%, transparent)"; }}
      >
        <Bell className="w-4 h-4" />
      </button>

      {/* Theme toggle */}
      <button
        data-testid="theme-toggle"
        className={iconBtnClass}
        title={`Theme: ${themeLabels[theme]}`}
        onClick={() => setTheme(nextTheme[theme])}
        style={{ color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)" }}
        onMouseEnter={(e) => { e.currentTarget.style.color = "var(--semantic-text)"; }}
        onMouseLeave={(e) => { e.currentTarget.style.color = "color-mix(in srgb, var(--semantic-text) 60%, transparent)"; }}
      >
        {themeIcons[theme]}
      </button>

      {/* Account menu */}
      {user && onSignOut && (
        <AccountMenu user={user} onSignOut={onSignOut} variant="icon" />
      )}
    </div>
  );
}
