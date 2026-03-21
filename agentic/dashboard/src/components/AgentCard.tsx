import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useFilter } from "@/context/FilterContext";
import { useApi, type AgentProfile } from "@/context/ApiContext";

interface AgentCardProps {
  agent: AgentProfile;
}

function formatRelativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

function statusColor(status: string): string {
  switch (status) {
    case "healthy":
      return "bg-green-500";
    case "degraded":
      return "bg-yellow-500";
    case "failing":
      return "bg-red-500";
    default:
      return "bg-muted-foreground/40";
  }
}

export default function AgentCard({ agent }: AgentCardProps) {
  const { addToken } = useFilter();
  const api = useApi();

  function handleClick() {
    if (api.workspaces.length > 0) {
      const ws = api.workspaces[0];
      addToken({ key: "workspace", value: ws.slug, label: ws.name });
    }
    addToken({ key: "agent", value: agent.agent, label: agent.agent });
  }

  const successRate =
    agent.total_sessions > 0
      ? Math.round((agent.successful_count / agent.total_sessions) * 100)
      : 0;

  return (
    <Card
      className="min-w-[220px] max-w-[280px] flex-shrink-0 cursor-pointer transition-colors hover:bg-accent/30"
      onClick={handleClick}
    >
      <CardContent className="space-y-2.5 pt-4">
        {/* Name + Role */}
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <div className="text-sm font-semibold truncate">{agent.agent}</div>
          </div>
          <Badge variant="secondary" className="text-[10px] shrink-0">
            {agent.role}
          </Badge>
        </div>

        {/* Sessions stats */}
        <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
          <span className="font-mono tabular-nums">{agent.total_sessions} sessions</span>
          <span>&middot;</span>
          <span className="font-mono tabular-nums">{successRate}% success</span>
        </div>

        {/* Tokens used */}
        {agent.total_tokens_used > 0 && (
          <div className="text-[11px] text-muted-foreground/70">
            {(agent.total_tokens_used / 1000).toFixed(1)}k tokens used
          </div>
        )}

        {/* Status line */}
        <div className="flex items-center gap-1.5 text-xs">
          <span className={`inline-block h-2 w-2 rounded-full ${statusColor(agent.last_status)}`} />
          {agent.last_session_at ? (
            <span className="text-muted-foreground">
              Last: {formatRelativeTime(agent.last_session_at)}
            </span>
          ) : (
            <span className="text-muted-foreground">No sessions</span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
