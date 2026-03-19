import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { CheckCircle2, XCircle } from 'lucide-react';
import type { Agent } from '@/data/agents';

interface AgentCardProps {
  agent: Agent;
}

function formatRelativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

export default function AgentCard({ agent }: AgentCardProps) {
  return (
    <Card className="min-w-[260px] max-w-[320px] flex-shrink-0">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <CardTitle className="text-base">{agent.name}</CardTitle>
          <Badge
            variant={
              agent.status === 'active'
                ? 'default'
                : agent.status === 'idle'
                  ? 'secondary'
                  : 'outline'
            }
            className="text-xs capitalize"
          >
            {agent.status}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap gap-1.5">
          <Badge variant="secondary">{agent.role}</Badge>
          <Badge variant="outline">{agent.model}</Badge>
        </div>

        <p className="text-xs text-muted-foreground">{agent.schedule}</p>

        <div className="flex items-center gap-1.5 rounded-md bg-muted p-2 text-xs">
          {agent.lastSession.status === 'succeeded' ? (
            <CheckCircle2 className="h-3.5 w-3.5 text-green-500" />
          ) : (
            <XCircle className="h-3.5 w-3.5 text-destructive" />
          )}
          <span>{agent.lastSession.duration}</span>
          <span className="ml-auto font-mono text-[10px] text-muted-foreground">
            {formatRelativeTime(agent.lastSession.time)}
          </span>
        </div>

        <div className="grid grid-cols-3 gap-2 border-t pt-3">
          <div className="text-center">
            <div className="font-mono text-sm font-semibold">
              {agent.stats.sessionsThisWeek}
            </div>
            <div className="text-[10px] text-muted-foreground">sessions/wk</div>
          </div>
          <div className="text-center">
            <div className="font-mono text-sm font-semibold">
              {agent.stats.toolCallsToday}
            </div>
            <div className="text-[10px] text-muted-foreground">tools today</div>
          </div>
          <div className="text-center">
            <div className="font-mono text-sm font-semibold">
              {agent.stats.issuesFiled}
            </div>
            <div className="text-[10px] text-muted-foreground">issues</div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
