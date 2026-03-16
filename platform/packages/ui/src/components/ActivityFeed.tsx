import type { ActivityInfo } from "../types/api";
import { cn } from "../lib/utils";

export interface ActivityFeedProps {
  activities: ActivityInfo[];
  loading?: boolean;
  hasMore?: boolean;
  onLoadMore?: () => void;
  onActivityClick?: (activity: ActivityInfo) => void;
}

function activityColor(type: string): string {
  if (type.includes("failed") || type.includes("drift")) return "text-destructive";
  if (type.includes("completed") || type.includes("passed") || type.includes("merged"))
    return "text-green-600 dark:text-green-400";
  if (type.includes("created")) return "text-blue-600 dark:text-blue-400";
  return "text-muted-foreground";
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(dateStr).toLocaleDateString();
}

export function ActivityFeed({
  activities,
  loading,
  hasMore,
  onLoadMore,
  onActivityClick,
}: ActivityFeedProps) {
  if (loading && activities.length === 0) {
    return (
      <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
        Loading activities...
      </div>
    );
  }

  if (activities.length === 0) {
    return (
      <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
        No activities yet
      </div>
    );
  }

  return (
    <div className="space-y-1">
      {activities.map((activity) => (
        <button
          key={activity.id}
          type="button"
          className={cn(
            "w-full text-left px-3 py-2 rounded-md transition-colors",
            "hover:bg-accent/50 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
            onActivityClick ? "cursor-pointer" : "cursor-default",
          )}
          onClick={() => onActivityClick?.(activity)}
        >
          <div className="flex items-start gap-3">
            <div
              className={cn(
                "mt-0.5 w-2 h-2 rounded-full shrink-0",
                activityColor(activity.type).replace("text-", "bg-"),
              )}
            />
            <div className="flex-1 min-w-0">
              <p className="text-sm leading-snug">
                <span className="font-medium">{activity.actor_name || "System"}</span>{" "}
                <span className="text-muted-foreground">{activity.summary}</span>
              </p>
              <p className="text-xs text-muted-foreground mt-0.5">
                {timeAgo(activity.created_at)}
                {activity.project_id && activity.data?.name && (
                  <span> in {activity.data.name}</span>
                )}
              </p>
            </div>
          </div>
        </button>
      ))}
      {hasMore && (
        <button
          type="button"
          className="w-full text-center py-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
          onClick={onLoadMore}
          disabled={loading}
        >
          {loading ? "Loading..." : "Load more"}
        </button>
      )}
    </div>
  );
}
