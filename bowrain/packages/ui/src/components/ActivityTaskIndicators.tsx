import { Badge, cn } from "@neokapi/ui-primitives";
import { useState, useEffect, useRef, useCallback } from "react";
import type { ActivityInfo, TaskInfo } from "../types/api";
import { Clock, CircleCheck } from "./icons";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

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

function useClickOutside(ref: React.RefObject<HTMLDivElement | null>, onClose: () => void) {
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [ref, onClose]);
}

// ---------------------------------------------------------------------------
// Count badge
// ---------------------------------------------------------------------------

function CountBadge({ count }: { count: number }) {
  if (count <= 0) return null;
  return (
    <span className="absolute -top-0.5 -right-0.5 flex items-center justify-center min-w-[14px] h-3.5 rounded-full bg-destructive text-destructive-foreground text-[9px] font-bold px-0.5 leading-none">
      {count > 99 ? "99+" : count}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Dot indicator (no count, just presence)
// ---------------------------------------------------------------------------

function DotIndicator() {
  return <span className="absolute -top-0 -right-0 w-2 h-2 rounded-full bg-primary" />;
}

// ---------------------------------------------------------------------------
// Activity indicator (header icon + dropdown)
// ---------------------------------------------------------------------------

function activityColor(type: string): string {
  if (type.includes("failed") || type.includes("drift")) return "bg-destructive";
  if (type.includes("completed") || type.includes("passed") || type.includes("merged"))
    return "bg-success";
  if (type.includes("created")) return "bg-info";
  return "bg-muted-foreground";
}

export interface ActivityIndicatorProps {
  activities: ActivityInfo[];
  newCount?: number;
  onActivityClick?: (activity: ActivityInfo) => void;
  onViewAll?: () => void;
  onMarkSeen?: () => void;
}

export function ActivityIndicator({
  activities,
  newCount = 0,
  onActivityClick,
  onViewAll,
  onMarkSeen,
}: ActivityIndicatorProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useClickOutside(ref, close);

  const hasNew = newCount > 0;

  return (
    <div ref={ref} className="relative">
      <button
        className="flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-colors text-muted-foreground hover:text-foreground relative"
        title="Recent activity"
        onClick={() => {
          setOpen((prev) => {
            if (!prev && hasNew) {
              onMarkSeen?.();
            }
            return !prev;
          });
        }}
      >
        <Clock className="w-4 h-4" />
        {hasNew && <DotIndicator />}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 max-h-96 rounded-lg border border-border bg-popover text-popover-foreground shadow-xl overflow-hidden z-50">
          <div className="flex items-center justify-between px-3 py-2 border-b border-border">
            <span className="text-sm font-medium">Activity</span>
            {onViewAll && (
              <button
                className="text-xs text-muted-foreground hover:text-foreground"
                onClick={() => {
                  onViewAll();
                  setOpen(false);
                }}
              >
                View all
              </button>
            )}
          </div>

          <div className="overflow-y-auto max-h-80">
            {activities.length === 0 ? (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                No recent activity
              </div>
            ) : (
              activities.slice(0, 20).map((a) => (
                <div
                  key={a.id}
                  className="flex items-start gap-2 px-3 py-2.5 border-b border-border/50 last:border-b-0 hover:bg-accent/50 transition-colors cursor-pointer"
                  onClick={() => {
                    onActivityClick?.(a);
                    setOpen(false);
                  }}
                >
                  <div className="pt-1.5 shrink-0">
                    <div className={cn("w-1.5 h-1.5 rounded-full", activityColor(a.type))} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-xs leading-snug">
                      <span className="font-medium">{a.actor_name || "System"}</span>{" "}
                      <span className="text-muted-foreground">{a.summary}</span>
                    </p>
                    <div className="text-[10px] text-muted-foreground/60 mt-0.5">
                      {timeAgo(a.created_at)}
                    </div>
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

// ---------------------------------------------------------------------------
// Task indicator (header icon + dropdown)
// ---------------------------------------------------------------------------

const priorityColors: Record<string, string> = {
  low: "bg-muted text-muted-foreground",
  normal: "bg-info/10 text-info dark:bg-info/20 dark:text-info",
  high: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
  urgent: "bg-destructive/10 text-destructive dark:bg-destructive/20 dark:text-destructive",
};

export interface TaskIndicatorProps {
  tasks: TaskInfo[];
  onTaskClick?: (task: TaskInfo) => void;
  onCompleteTask?: (taskId: string) => void;
  onViewAll?: () => void;
}

export function TaskIndicator({
  tasks,
  onTaskClick,
  onCompleteTask,
  onViewAll,
}: TaskIndicatorProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useClickOutside(ref, close);

  const actionableCount = tasks.filter(
    (t) => t.status === "open" || t.status === "in_progress",
  ).length;

  return (
    <div ref={ref} className="relative">
      <button
        className="flex items-center justify-center w-7 h-7 rounded bg-transparent border-none cursor-pointer transition-colors text-muted-foreground hover:text-foreground relative"
        title="My tasks"
        onClick={() => setOpen((prev) => !prev)}
      >
        <CircleCheck className="w-4 h-4" />
        <CountBadge count={actionableCount} />
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 max-h-96 rounded-lg border border-border bg-popover text-popover-foreground shadow-xl overflow-hidden z-50">
          <div className="flex items-center justify-between px-3 py-2 border-b border-border">
            <span className="text-sm font-medium">My Tasks</span>
            {onViewAll && (
              <button
                className="text-xs text-muted-foreground hover:text-foreground"
                onClick={() => {
                  onViewAll();
                  setOpen(false);
                }}
              >
                View all
              </button>
            )}
          </div>

          <div className="overflow-y-auto max-h-80">
            {tasks.length === 0 ? (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                No tasks assigned to you
              </div>
            ) : (
              tasks.slice(0, 15).map((t) => {
                const isActive = t.status === "open" || t.status === "in_progress";
                const isOverdue = t.due_at && new Date(t.due_at) < new Date() && isActive;

                return (
                  <div
                    key={t.id}
                    className={cn(
                      "flex items-start gap-2 px-3 py-2.5 border-b border-border/50 last:border-b-0 hover:bg-accent/50 transition-colors cursor-pointer",
                      isOverdue && "bg-destructive/5",
                    )}
                    onClick={() => {
                      onTaskClick?.(t);
                      setOpen(false);
                    }}
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-1.5">
                        <p className="text-xs font-medium leading-snug truncate flex-1">
                          {t.title}
                        </p>
                        <Badge
                          variant="outline"
                          className={cn(
                            "text-[9px] shrink-0 h-4",
                            priorityColors[t.priority] ?? "",
                          )}
                        >
                          {t.priority}
                        </Badge>
                      </div>
                      <div className="flex items-center gap-1.5 mt-0.5">
                        <span className="text-[10px] text-muted-foreground/60">
                          {timeAgo(t.created_at)}
                        </span>
                        {isOverdue && (
                          <span className="text-[10px] text-destructive font-medium">Overdue</span>
                        )}
                      </div>
                    </div>

                    {isActive && onCompleteTask && (
                      <button
                        className="text-[10px] px-1.5 py-0.5 rounded bg-success/10 text-success dark:bg-success/20 dark:text-success hover:opacity-80 transition-opacity shrink-0 mt-0.5"
                        onClick={(e) => {
                          e.stopPropagation();
                          onCompleteTask(t.id);
                        }}
                      >
                        Done
                      </button>
                    )}
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
}
