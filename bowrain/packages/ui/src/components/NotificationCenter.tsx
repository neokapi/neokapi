import { useState, useEffect, useRef, useCallback } from "react";
import type { NotificationInfo } from "../types/api";
import { Bell, Check, Trash2 } from "./icons";

interface NotificationCenterProps {
  notifications: NotificationInfo[];
  unreadCount: number;
  onMarkRead: (id: string) => void;
  onMarkAllRead: () => void;
  onDelete: (id: string) => void;
  onNotificationClick?: (notification: NotificationInfo) => void;
}

/** Notification icon type → color mapping. */
function typeColor(type: string): string {
  switch (type) {
    case "review.assigned":
      return "text-blue-500";
    case "review.completed":
      return "text-green-500";
    case "extraction.completed":
      return "text-purple-500";
    case "task.assigned":
    case "content.ready":
      return "text-blue-500";
    case "content.available":
      return "text-teal-500";
    case "quality.gate.failed":
      return "text-red-500";
    case "flow.failed":
      return "text-red-500";
    default:
      return "text-muted-foreground";
  }
}

/** Format relative time. */
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

export function NotificationCenter({
  notifications,
  unreadCount,
  onMarkRead,
  onMarkAllRead,
  onDelete,
  onNotificationClick,
}: NotificationCenterProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  const toggle = useCallback(() => setOpen((prev) => !prev), []);

  return (
    <div ref={ref} className="relative">
      {/* Bell button with unread badge */}
      <button
        className="flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-colors text-muted-foreground hover:text-foreground relative"
        title="Notifications"
        onClick={toggle}
      >
        <Bell className="w-4 h-4" />
        {unreadCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 flex items-center justify-center min-w-[14px] h-3.5 rounded-full bg-destructive text-destructive-foreground text-[9px] font-bold px-0.5 leading-none">
            {unreadCount > 99 ? "99+" : unreadCount}
          </span>
        )}
      </button>

      {/* Dropdown panel */}
      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 max-h-96 rounded-lg border border-border bg-popover text-popover-foreground shadow-xl overflow-hidden z-50">
          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b border-border">
            <span className="text-sm font-medium">Notifications</span>
            {unreadCount > 0 && (
              <button
                className="text-xs text-muted-foreground hover:text-foreground"
                onClick={onMarkAllRead}
              >
                Mark all read
              </button>
            )}
          </div>

          {/* List */}
          <div className="overflow-y-auto max-h-80">
            {notifications.length === 0 ? (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                No notifications
              </div>
            ) : (
              notifications.map((n) => (
                <div
                  key={n.id}
                  className={`flex items-start gap-2 px-3 py-2.5 border-b border-border/50 last:border-b-0 hover:bg-accent/50 transition-colors cursor-pointer ${!n.read ? "bg-accent/20" : ""}`}
                  onClick={() => {
                    if (!n.read) onMarkRead(n.id);
                    onNotificationClick?.(n);
                  }}
                >
                  {/* Unread dot */}
                  <div className="pt-1.5 w-2 shrink-0">
                    {!n.read && <div className="w-1.5 h-1.5 rounded-full bg-primary" />}
                  </div>

                  {/* Content */}
                  <div className="flex-1 min-w-0">
                    <div className={`text-xs font-medium truncate ${typeColor(n.type)}`}>
                      {n.title}
                    </div>
                    {n.body && (
                      <div className="text-xs text-muted-foreground mt-0.5 line-clamp-2">
                        {n.body}
                      </div>
                    )}
                    <div className="text-[10px] text-muted-foreground/60 mt-1">
                      {timeAgo(n.created_at)}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-0.5 shrink-0">
                    {!n.read && (
                      <button
                        className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground"
                        title="Mark as read"
                        onClick={(e) => {
                          e.stopPropagation();
                          onMarkRead(n.id);
                        }}
                      >
                        <Check className="w-3 h-3" />
                      </button>
                    )}
                    <button
                      className="p-1 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                      title="Delete"
                      onClick={(e) => {
                        e.stopPropagation();
                        onDelete(n.id);
                      }}
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
