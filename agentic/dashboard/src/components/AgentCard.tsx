import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import type { Agent } from '../data/agents';

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
  const statusVariant = agent.status === 'active'
    ? 'default'
    : agent.status === 'idle'
      ? 'secondary'
      : 'outline';

  return (
    <Card className="min-w-[260px] max-w-[320px] flex-shrink-0">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div>
            <h3 className="text-base font-semibold">{agent.name}</h3>
            <p className="text-sm text-muted-foreground">{agent.title}</p>
          </div>
          <Badge variant={statusVariant} className="capitalize">
            {agent.status}
          </Badge>
        </div>
        <div className="mt-2 flex flex-wrap gap-1.5">
          <Badge variant="secondary">{agent.role}</Badge>
          <Badge variant="outline" className="font-mono text-xs">
            {agent.model}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-1 text-xs text-muted-foreground">
          <div>{agent.schedule}</div>
          {agent.targetLanguage && <div>{agent.targetLanguage}</div>}
        </div>

        <div className="rounded-md bg-muted p-2.5">
          <div className="flex items-center gap-1.5 text-xs">
            <Badge
              variant={agent.lastSession.status === 'succeeded' ? 'default' : 'destructive'}
              className="h-5 text-[10px]"
            >
              {agent.lastSession.status}
            </Badge>
            <span className="text-muted-foreground">{agent.lastSession.duration}</span>
            <span className="ml-auto font-mono text-[10px] text-muted-foreground">
              {formatRelativeTime(agent.lastSession.time)}
            </span>
          </div>
        </div>

        <Separator />

        <div className="grid grid-cols-3 gap-2 text-center">
          <div>
            <div className="font-mono text-sm font-semibold">{agent.stats.sessionsThisWeek}</div>
            <div className="text-[10px] text-muted-foreground">sessions/wk</div>
          </div>
          <div>
            <div className="font-mono text-sm font-semibold">{agent.stats.toolCallsToday}</div>
            <div className="text-[10px] text-muted-foreground">tools today</div>
          </div>
          <div>
            <div className="font-mono text-sm font-semibold">{agent.stats.issuesFiled}</div>
            <div className="text-[10px] text-muted-foreground">issues</div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
