import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { useFilter } from '@/context/FilterContext';
import { useApi, type AgentProfile } from '@/context/ApiContext';

interface AgentCardProps {
  agent: AgentProfile;
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
  const { addToken } = useFilter();
  const api = useApi();

  const isIdle = !agent.lastActive;

  function handleClick() {
    // Add workspace token first if available, then agent
    if (api.workspaces.length > 0) {
      const ws = api.workspaces[0];
      addToken({ key: 'workspace', value: ws.slug, label: ws.name });
    }
    addToken({ key: 'agent', value: agent.id, label: agent.displayName });
  }

  return (
    <Card
      className="min-w-[220px] max-w-[280px] flex-shrink-0 cursor-pointer transition-colors hover:bg-accent/30"
      onClick={handleClick}
    >
      <CardContent className="space-y-2.5 pt-4">
        {/* Name + Role */}
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <div className="text-sm font-semibold truncate">{agent.displayName}</div>
          </div>
          <Badge variant="secondary" className="text-[10px] shrink-0">
            {agent.role}
          </Badge>
        </div>

        {/* Model */}
        <Badge variant="outline" className="text-[10px]">
          {agent.model}
        </Badge>

        {/* Schedule */}
        <p className="text-[11px] text-muted-foreground">{agent.schedule}</p>

        {/* Status line */}
        <div className="flex items-center gap-1.5 text-xs">
          <span
            className={`inline-block h-2 w-2 rounded-full ${
              isIdle ? 'bg-muted-foreground/40' : 'bg-green-500'
            }`}
          />
          {isIdle ? (
            <span className="text-muted-foreground">Idle</span>
          ) : (
            <span className="text-muted-foreground">
              Last: {formatRelativeTime(agent.lastActive!)}
            </span>
          )}
        </div>

        {/* Events today */}
        <div className="text-[11px] text-muted-foreground/70">
          {agent.eventsToday} event{agent.eventsToday !== 1 ? 's' : ''} today
        </div>
      </CardContent>
    </Card>
  );
}
