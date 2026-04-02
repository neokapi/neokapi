import type { ActivityInfo } from "../types/api";
import { cn } from "@neokapi/ui-primitives";

export interface ActivityFeedProps {
  activities: ActivityInfo[];
  loading?: boolean;
  hasMore?: boolean;
  onLoadMore?: () => void;
  onActivityClick?: (activity: ActivityInfo) => void;
}

// ---------------------------------------------------------------------------
// Agent-specific helpers
// ---------------------------------------------------------------------------

/** Whether an activity type belongs to the @bravo agent subsystem. */
function isAgentActivity(type: string): boolean {
  return type.startsWith("agent.");
}

/** Generate a human-readable summary for agent events when the server-provided summary is generic. */
function agentSummary(activity: ActivityInfo): string {
  if (activity.summary) return activity.summary;

  const data = activity.data ?? {};
  switch (activity.type) {
    case "agent.conversation.created":
      return data.title
        ? `started a conversation: "${data.title}"`
        : "started a conversation with @bravo";
    case "agent.message.sent":
      return data.blocks_count
        ? `asked @bravo to process ${data.blocks_count} blocks`
        : "sent a message to @bravo";
    case "agent.tool.executed":
      return data.tool
        ? `@bravo ran ${data.tool}${data.duration ? ` (${data.duration})` : ""}`
        : "@bravo executed a tool";
    case "agent.tool.approved":
      return data.tool ? `approved @bravo to run ${data.tool}` : "approved a @bravo tool call";
    case "agent.tool.denied":
      return data.tool ? `denied @bravo from running ${data.tool}` : "denied a @bravo tool call";
    case "agent.code.executed":
      return data.language
        ? `@bravo ran a ${data.language} script${data.exit_code === "0" ? "" : " (failed)"}`
        : "@bravo executed code in sandbox";
    default:
      return "@bravo performed an action";
  }
}

/** Actor display name — use "@bravo" when the actor is the agent itself. */
function actorName(activity: ActivityInfo): string {
  if (isAgentActivity(activity.type)) {
    // For tool.executed and code.executed the actor is the agent.
    if (activity.type === "agent.tool.executed" || activity.type === "agent.code.executed") {
      return "@bravo";
    }
  }
  return activity.actor_name || "System";
}

function activityColor(type: string): string {
  // Agent-specific colors
  if (type === "agent.tool.denied") return "text-destructive";
  if (type === "agent.tool.approved") return "text-green-600 dark:text-green-400";
  if (type === "agent.conversation.created") return "text-blue-600 dark:text-blue-400";
  if (type === "agent.tool.executed" || type === "agent.code.executed")
    return "text-purple-600 dark:text-purple-400";
  if (type === "agent.message.sent") return "text-blue-600 dark:text-blue-400";

  // General colors
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
      {activities.map((activity) => {
        const isAgent = isAgentActivity(activity.type);
        const summary = isAgent ? agentSummary(activity) : activity.summary;
        const actor = actorName(activity);

        return (
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
                  <span className="font-medium">{actor}</span>{" "}
                  <span className="text-muted-foreground">{summary}</span>
                </p>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {timeAgo(activity.created_at)}
                  {activity.project_id && activity.data?.name && (
                    <span> in {activity.data.name}</span>
                  )}
                  {isAgent && (
                    <span className="ml-1 text-purple-600 dark:text-purple-400 font-medium">
                      @bravo
                    </span>
                  )}
                </p>
              </div>
            </div>
          </button>
        );
      })}
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
