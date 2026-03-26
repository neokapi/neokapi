import type { User } from "../types/api";
import { cn } from "../lib/utils";
import { LogOut, Settings } from "./icons";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from "./ui/dropdown-menu";

type AvatarStatus = "online" | "offline" | "away" | "busy";

interface AccountMenuProps {
  user: User;
  onSignOut: () => void;
  variant?: "default" | "sidebar" | "icon";
  onSettings?: () => void;
  collapsed?: boolean;
  /** Status indicator dot on the avatar (icon variant only). */
  status?: AvatarStatus;
}

function UserAvatar({ user, size = 28 }: { user: User; size?: number }) {
  return (
    <div
      className="rounded-full bg-primary text-primary-foreground flex items-center justify-center font-bold shrink-0"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.4,
        backgroundImage: user.avatar_url ? `url(${user.avatar_url})` : undefined,
        backgroundSize: "cover",
      }}
    >
      {!user.avatar_url && (user.name || "?")[0].toUpperCase()}
    </div>
  );
}

const statusLabels: Record<AvatarStatus, string> = {
  online: "Connected",
  away: "Offline",
  busy: "Connecting",
  offline: "Disconnected",
};

function StatusDot({ status }: { status: AvatarStatus }) {
  return (
    <span
      title={statusLabels[status]}
      data-testid="connection-status"
      className={cn(
        "absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 rounded-full border-2 border-background",
        status === "online" && "bg-green-500",
        status === "away" && "bg-amber-500",
        status === "busy" && "bg-red-500",
        status === "offline" && "bg-muted-foreground/40",
      )}
    />
  );
}

export function AccountMenu({
  user,
  onSignOut,
  variant = "default",
  onSettings,
  collapsed = false,
  status,
}: AccountMenuProps) {
  if (variant === "sidebar") {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger className="flex items-center gap-2 w-full px-3 py-2.5 bg-transparent border-none cursor-pointer transition-colors outline-none text-sidebar-foreground hover:bg-sidebar-accent">
          <UserAvatar user={user} size={28} />
          {!collapsed && (
            <div className="flex-1 min-w-0 text-left">
              <div className="text-sm font-medium truncate">{user.name || user.email}</div>
              {user.name && user.email && (
                <div className="text-[11px] truncate text-muted-foreground">{user.email}</div>
              )}
            </div>
          )}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" side="top" className="w-[200px] backdrop-blur-lg">
          <DropdownMenuLabel className="text-xs text-muted-foreground font-normal">
            {user.email}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          {onSettings && (
            <DropdownMenuItem onClick={onSettings} className="flex items-center gap-2">
              <Settings className="w-4 h-4" />
              <span>Settings</span>
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onClick={onSignOut} className="flex items-center gap-2">
            <LogOut className="w-4 h-4" />
            <span>Sign out</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  // Icon-only variant (avatar only, no label or border)
  if (variant === "icon") {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger className="flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-opacity outline-none opacity-60 hover:opacity-100">
          <span className="relative inline-flex">
            <UserAvatar user={user} size={24} />
            {status && <StatusDot status={status} />}
          </span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel className="text-xs text-muted-foreground font-normal">
            {user.email || user.name}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          {onSettings && (
            <DropdownMenuItem onClick={onSettings} className="flex items-center gap-2">
              <Settings className="w-4 h-4" />
              <span>Settings</span>
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onClick={onSignOut} className="flex items-center gap-2">
            <LogOut className="w-4 h-4" />
            <span>Sign out</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  // Default variant
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center gap-2 px-3 py-1.5 bg-transparent border border-input rounded-lg cursor-pointer text-foreground hover:bg-accent transition-colors">
        <UserAvatar user={user} size={28} />
        <span className="text-[13px]">{user.name || user.email}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuLabel className="text-xs text-muted-foreground font-normal">
          {user.email}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {onSettings && (
          <DropdownMenuItem onClick={onSettings} className="flex items-center gap-2">
            <Settings className="w-4 h-4" />
            <span>Settings</span>
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={onSignOut} className="flex items-center gap-2">
          <LogOut className="w-4 h-4" />
          <span>Sign out</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
