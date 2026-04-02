import { cn } from "@neokapi/ui-primitives";
import type { BravoConversation } from "../../types/api";

export interface BravoConversationListProps {
  conversations: BravoConversation[];
  activeId?: string;
  onSelect?: (conv: BravoConversation) => void;
  onDelete?: (conv: BravoConversation) => void;
  onNew?: () => void;
  loading?: boolean;
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function statusDot(status: BravoConversation["status"]): string {
  if (status === "active") return "bg-green-500";
  if (status === "completed") return "bg-muted-foreground";
  return "bg-destructive";
}

export function BravoConversationList({
  conversations,
  activeId,
  onSelect,
  onDelete,
  onNew,
  loading,
}: BravoConversationListProps) {
  return (
    <div className="flex flex-col gap-1">
      <button
        onClick={onNew}
        className="flex items-center gap-2 rounded-md border border-dashed px-3 py-2 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
      >
        <span className="text-lg leading-none">+</span>
        New conversation
      </button>

      {loading && conversations.length === 0 && (
        <div className="py-8 text-center text-sm text-muted-foreground">Loading...</div>
      )}

      {!loading && conversations.length === 0 && (
        <div className="py-8 text-center text-sm text-muted-foreground">No conversations yet</div>
      )}

      {conversations.map((conv) => (
        <button
          key={conv.id}
          onClick={() => onSelect?.(conv)}
          className={cn(
            "group flex items-start gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-accent",
            activeId === conv.id && "bg-accent",
          )}
        >
          <span className={cn("mt-1.5 h-2 w-2 shrink-0 rounded-full", statusDot(conv.status))} />
          <div className="min-w-0 flex-1">
            <div className="truncate font-medium">{conv.title || "Untitled"}</div>
            <div className="text-xs text-muted-foreground">{timeAgo(conv.updated_at)}</div>
          </div>
          {onDelete && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                onDelete(conv);
              }}
              className="shrink-0 opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-destructive transition-opacity"
              aria-label="Delete conversation"
            >
              &times;
            </button>
          )}
        </button>
      ))}
    </div>
  );
}
