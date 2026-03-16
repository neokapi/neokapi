import type { User, NotificationInfo, ActivityInfo, TaskInfo } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Sun, Moon, Monitor, WifiOff, LogOut } from "./icons";
import { NotificationCenter } from "./NotificationCenter";
import { ActivityIndicator, TaskIndicator } from "./ActivityTaskIndicators";
import { Button } from "./ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu";

type ConnectionState = "disconnected" | "connecting" | "connected" | "offline";

export interface TopBarProps {
  user: User | null;
  onSignOut?: () => void;
  connectionState?: ConnectionState;
  pendingChanges?: number;
  notifications?: NotificationInfo[];
  unreadCount?: number;
  onMarkNotificationRead?: (id: string) => void;
  onMarkAllNotificationsRead?: () => void;
  onDeleteNotification?: (id: string) => void;
  onNotificationClick?: (notification: NotificationInfo) => void;
  activities?: ActivityInfo[];
  onActivityClick?: (activity: ActivityInfo) => void;
  onViewAllActivities?: () => void;
  myTasks?: TaskInfo[];
  onTaskClick?: (task: TaskInfo) => void;
  onCompleteTask?: (taskId: string) => void;
  onViewAllTasks?: () => void;
  leftSlot?: React.ReactNode;
}

const nextTheme: Record<Theme, Theme> = { light: "dark", dark: "system", system: "light" };
const themeIcons: Record<Theme, React.ReactNode> = {
  light: <Sun className="size-4" />,
  dark: <Moon className="size-4" />,
  system: <Monitor className="size-4" />,
};
const themeLabels: Record<Theme, string> = { light: "Light", dark: "Dark", system: "System" };

function UserAvatar({ user, size = 28 }: { user: User; size?: number }) {
  const initials = (user.name || user.email || "?")
    .split(/[\s@]+/)
    .slice(0, 2)
    .map((s) => s[0]?.toUpperCase() ?? "")
    .join("");

  return (
    <div
      className="flex items-center justify-center rounded-full bg-muted text-muted-foreground text-xs font-medium"
      style={{
        width: size,
        height: size,
        ...(user.avatar_url
          ? {
              backgroundImage: `url(${user.avatar_url})`,
              backgroundSize: "cover",
              backgroundPosition: "center",
              color: "transparent",
            }
          : {}),
      }}
    >
      {initials}
    </div>
  );
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
  activities,
  onActivityClick,
  onViewAllActivities,
  myTasks,
  onTaskClick,
  onCompleteTask,
  onViewAllTasks,
  leftSlot,
}: TopBarProps) {
  const { theme, setTheme } = useTheme();
  const isOffline = connectionState === "offline";

  return (
    <>
      {/* Left slot (e.g. stream selector) */}
      {leftSlot}

      {/* Offline pending-changes indicator */}
      {isOffline && pendingChanges != null && pendingChanges > 0 && (
        <span className="flex items-center gap-1 text-xs text-amber-500">
          <WifiOff className="size-3" />
          <span>{pendingChanges} pending</span>
        </span>
      )}

      {/* Activity indicator */}
      {activities && (
        <ActivityIndicator
          activities={activities}
          onActivityClick={onActivityClick}
          onViewAll={onViewAllActivities}
        />
      )}

      {/* Task indicator */}
      {myTasks && (
        <TaskIndicator
          tasks={myTasks}
          onTaskClick={onTaskClick}
          onCompleteTask={onCompleteTask}
          onViewAll={onViewAllTasks}
        />
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
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setTheme(nextTheme[theme])}
        title={`Theme: ${themeLabels[theme]}`}
        data-testid="theme-toggle"
      >
        {themeIcons[theme]}
      </Button>

      {/* Account menu */}
      {user && onSignOut && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" className="rounded-full">
              <UserAvatar user={user} size={24} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel className="font-normal">
              <div className="flex flex-col gap-1">
                <p className="text-sm font-medium leading-none">{user.name || "User"}</p>
                <p className="text-xs leading-none text-muted-foreground">{user.email}</p>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onSignOut}>
              <LogOut />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </>
  );
}
