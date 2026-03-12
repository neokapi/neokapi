import type { CollabUser } from "../hooks/useCollaboration";

/** Props for the PresenceAvatars component. */
export interface PresenceAvatarsProps {
  /** Connected users to display. */
  users: CollabUser[];
  /** Current user ID (excluded from display). */
  currentUserId?: string;
  /** Maximum number of avatars to show before "+N" overflow. */
  maxVisible?: number;
}

/**
 * Displays a row of overlapping user avatars showing who else is
 * currently editing the same document.
 */
export function PresenceAvatars({
  users,
  currentUserId,
  maxVisible = 5,
}: PresenceAvatarsProps) {
  // Exclude the current user from the display.
  const others = currentUserId
    ? users.filter((u) => u.userId !== currentUserId)
    : users;

  if (others.length === 0) return null;

  const visible = others.slice(0, maxVisible);
  const overflow = others.length - maxVisible;

  return (
    <div className="flex items-center -space-x-2" data-testid="presence-avatars">
      {visible.map((user) => (
        <div
          key={user.userId}
          className="relative flex h-7 w-7 items-center justify-center rounded-full border-2 border-background text-[10px] font-medium text-white"
          style={{ backgroundColor: user.color }}
          title={user.name}
        >
          {user.avatarUrl ? (
            <img
              src={user.avatarUrl}
              alt={user.name}
              className="h-full w-full rounded-full object-cover"
            />
          ) : (
            <span>{user.name.charAt(0).toUpperCase()}</span>
          )}
          {/* Green online indicator dot */}
          <span className="absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-background bg-emerald-500" />
        </div>
      ))}
      {overflow > 0 && (
        <div
          className="flex h-7 w-7 items-center justify-center rounded-full border-2 border-background bg-muted text-[10px] font-medium text-muted-foreground"
          title={`${overflow} more`}
        >
          +{overflow}
        </div>
      )}
    </div>
  );
}
