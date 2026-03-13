import type { User, NotificationInfo } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Sun, Moon, Monitor, WifiOff } from "./icons";
import { AccountMenu } from "./AccountMenu";
import { NotificationCenter } from "./NotificationCenter";

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

export interface TopBarProps {
  user: User | null;
  onSignOut?: () => void;
  /** Server connection state — maps to a status dot on the avatar. */
  connectionState?: ConnectionState;
  /** Number of pending sync changes (shown when offline). */
  pendingChanges?: number;
  /** Notification data (omit to hide notification center). */
  notifications?: NotificationInfo[];
  unreadCount?: number;
  onMarkNotificationRead?: (id: string) => void;
  onMarkAllNotificationsRead?: () => void;
  onDeleteNotification?: (id: string) => void;
  onNotificationClick?: (notification: NotificationInfo) => void;
}

const nextTheme: Record<Theme, Theme> = { light: "dark", dark: "system", system: "light" };
const themeIcons: Record<Theme, React.ReactNode> = {
  light: <Sun className="w-4 h-4" />,
  dark: <Moon className="w-4 h-4" />,
  system: <Monitor className="w-4 h-4" />,
};
const themeLabels: Record<Theme, string> = { light: "Light", dark: "Dark", system: "System" };

const iconBtnClass =
  "flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-colors text-muted-foreground hover:text-foreground";

function connectionToAvatarStatus(state?: ConnectionState) {
  switch (state) {
    case "connected":
      return "online" as const;
    case "offline":
      return "away" as const;
    case "connecting":
      return "busy" as const;
    default:
      return undefined;
  }
}

export function TopBar({
  user,
  onSignOut,
  connectionState,
  pendingChanges,
  notifications,
  unreadCount,
  onMarkNotificationRead,
  onMarkAllNotificationsRead,
  onDeleteNotification,
  onNotificationClick,
}: TopBarProps) {
  const { theme, setTheme } = useTheme();
  const isOffline = connectionState === "offline";

  return (
    <div className="h-10 shrink-0 flex items-center justify-end gap-1 px-3">
      {/* Offline pending-changes indicator */}
      {isOffline && pendingChanges != null && pendingChanges > 0 && (
        <span className="flex items-center gap-1 text-xs text-amber-500 mr-1">
          <WifiOff className="w-3 h-3" />
          <span>{pendingChanges} pending</span>
        </span>
      )}

      {/* Notification center */}
      {notifications &&
      onMarkNotificationRead &&
      onMarkAllNotificationsRead &&
      onDeleteNotification ? (
        <NotificationCenter
          notifications={notifications}
          unreadCount={unreadCount ?? 0}
          onMarkRead={onMarkNotificationRead}
          onMarkAllRead={onMarkAllNotificationsRead}
          onDelete={onDeleteNotification}
          onNotificationClick={onNotificationClick}
        />
      ) : null}

      {/* Theme toggle */}
      <button
        data-testid="theme-toggle"
        className={iconBtnClass}
        title={`Theme: ${themeLabels[theme]}`}
        onClick={() => setTheme(nextTheme[theme])}
      >
        {themeIcons[theme]}
      </button>

      {/* Account menu */}
      {user && onSignOut && (
        <AccountMenu
          user={user}
          onSignOut={onSignOut}
          variant="icon"
          status={connectionToAvatarStatus(connectionState)}
        />
      )}
    </div>
  );
}
