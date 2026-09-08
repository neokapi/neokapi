import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@neokapi/ui-primitives";
import type { User, NotificationInfo, ActivityInfo, TaskInfo } from "../types/api";
import { useTheme, type Theme } from "../context/ThemeContext";
import { Sun, Moon, Monitor, LogOut, Settings } from "./icons";
import { ConnectionIndicator, type ConnectionState } from "./ConnectionIndicator";
import { NotificationCenter } from "./NotificationCenter";
import { ActivityIndicator, TaskIndicator } from "./ActivityTaskIndicators";

export interface TopBarProps {
  user: User | null;
  onSignOut?: () => void;
  onSettings?: () => void;
  connectionState?: ConnectionState;
  pendingChanges?: number;
  failedChanges?: number;
  /** Ask the backend to reconnect now (desktop shells only). */
  onRetryConnection?: () => void;
  notifications?: NotificationInfo[];
  unreadCount?: number;
  onMarkNotificationRead?: (id: string) => void;
  onMarkAllNotificationsRead?: () => void;
  onDeleteNotification?: (id: string) => void;
  onNotificationClick?: (notification: NotificationInfo) => void;
  activities?: ActivityInfo[];
  newActivityCount?: number;
  onActivityClick?: (activity: ActivityInfo) => void;
  onViewAllActivities?: () => void;
  onMarkActivitiesSeen?: () => void;
  myTasks?: TaskInfo[];
  /** Actionable task total for the badge, counted server-side. */
  myTaskCount?: number;
  onTaskClick?: (task: TaskInfo) => void;
  onCompleteTask?: (taskId: string) => void;
  onViewAllTasks?: () => void;
  leftSlot?: React.ReactNode;
  /** Slot rendered immediately before the user avatar (e.g. @bravo trigger). */
  beforeAvatarSlot?: React.ReactNode;
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
  onSettings,
  connectionState,
  pendingChanges,
  notifications,
  unreadCount,
  onMarkNotificationRead,
  onMarkAllNotificationsRead,
  onDeleteNotification,
  onNotificationClick,
  activities,
  newActivityCount,
  onActivityClick,
  onViewAllActivities,
  onMarkActivitiesSeen,
  myTasks,
  myTaskCount,
  onTaskClick,
  onCompleteTask,
  onViewAllTasks,
  leftSlot,
  beforeAvatarSlot,
  failedChanges,
  onRetryConnection,
}: TopBarProps) {
  const { theme, setTheme } = useTheme();

  return (
    <>
      {/* Left slot (e.g. stream selector) */}
      {leftSlot}

      <ConnectionIndicator
        connectionState={connectionState}
        pendingChanges={pendingChanges}
        failedChanges={failedChanges}
        onRetryConnection={onRetryConnection}
      />

      {/* Activity indicator */}
      {activities && (
        <ActivityIndicator
          activities={activities}
          newCount={newActivityCount}
          onActivityClick={onActivityClick}
          onViewAll={onViewAllActivities}
          onMarkSeen={onMarkActivitiesSeen}
        />
      )}

      {/* Task indicator */}
      {myTasks && (
        <TaskIndicator
          tasks={myTasks}
          count={myTaskCount}
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
      <button
        className="flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-colors text-muted-foreground hover:text-foreground"
        onClick={() => setTheme(nextTheme[theme])}
        title={`Theme: ${themeLabels[theme]}`}
        data-testid="theme-toggle"
      >
        {themeIcons[theme]}
      </button>

      {/* Slot before avatar (e.g. @bravo trigger) */}
      {beforeAvatarSlot}

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
            {onSettings && (
              <DropdownMenuItem onClick={onSettings}>
                <Settings />
                Settings
              </DropdownMenuItem>
            )}
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
