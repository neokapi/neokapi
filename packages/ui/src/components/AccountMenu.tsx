import type { User } from "../types/api";
import { LogOut, Settings } from "./icons";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from "./ui/dropdown-menu";

interface AccountMenuProps {
  user: User;
  onSignOut: () => void;
  variant?: "default" | "sidebar" | "icon";
  onSettings?: () => void;
  collapsed?: boolean;
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

export function AccountMenu({ user, onSignOut, variant = "default", onSettings, collapsed = false }: AccountMenuProps) {
  if (variant === "sidebar") {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger
          className="flex items-center gap-2 w-full px-3 py-2.5 bg-transparent border-none cursor-pointer transition-colors outline-none"
          style={{ color: "var(--semantic-text)" }}
          onMouseEnter={(e: React.MouseEvent<HTMLButtonElement>) => {
            e.currentTarget.style.background = "var(--semantic-surface-elevated)";
          }}
          onMouseLeave={(e: React.MouseEvent<HTMLButtonElement>) => {
            e.currentTarget.style.background = "transparent";
          }}
        >
          <UserAvatar user={user} size={collapsed ? 28 : 28} />
          {!collapsed && (
            <div className="flex-1 min-w-0 text-left">
              <div className="text-sm font-medium truncate">{user.name || user.email}</div>
              {user.name && user.email && (
                <div className="text-[11px] truncate" style={{ color: "color-mix(in srgb, var(--semantic-text) 50%, transparent)" }}>
                  {user.email}
                </div>
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
        <DropdownMenuTrigger
          className="flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-opacity outline-none opacity-60 hover:opacity-100"
        >
          <UserAvatar user={user} size={24} />
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

  // Default variant (unchanged behavior)
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
        <DropdownMenuItem onClick={onSignOut}>
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
