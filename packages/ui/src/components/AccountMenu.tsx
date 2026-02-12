import type { User } from "../types/api";
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
}

export function AccountMenu({ user, onSignOut }: AccountMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center gap-2 px-3 py-1.5 bg-transparent border border-input rounded-lg cursor-pointer text-foreground hover:bg-accent transition-colors">
        <div
          className="w-7 h-7 rounded-full bg-primary text-primary-foreground flex items-center justify-center text-xs font-bold"
          style={{
            backgroundImage: user.avatar_url ? `url(${user.avatar_url})` : undefined,
            backgroundSize: "cover",
          }}
        >
          {!user.avatar_url && (user.name || "?")[0].toUpperCase()}
        </div>
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
