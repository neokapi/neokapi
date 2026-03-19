import { ScrollArea } from '@/components/ui/scroll-area';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { activityFeed } from '@/data/activity';
import { useFilter } from '@/context/FilterContext';

function formatRelativeTime(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

export default function ActivityFeed() {
  const { workspace, agent } = useFilter();

  let filtered = activityFeed;
  if (workspace) filtered = filtered.filter((e) => e.workspace === workspace);
  if (agent) filtered = filtered.filter((e) => e.agentId === agent);

  return (
    <Card className="flex h-full flex-col">
      <CardHeader>
        <CardTitle className="text-sm">Activity Feed</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 p-0">
        <ScrollArea className="h-[500px] px-4 pb-4">
          <div className="space-y-2">
            {filtered.map((entry) => (
              <div
                key={entry.id}
                className="rounded-md border-l-2 border-l-primary bg-muted/50 p-3"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="secondary" className="text-xs">
                    {entry.agentName}
                  </Badge>
                  <span className="font-mono text-[10px] text-muted-foreground">
                    {formatRelativeTime(entry.timestamp)}
                  </span>
                </div>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                  {entry.action}
                </p>
                {entry.toolsUsed.length > 0 && (
                  <div className="mt-1.5 flex flex-wrap gap-1">
                    {entry.toolsUsed.map((tool) => (
                      <Badge key={tool} variant="outline" className="text-[9px]">
                        {tool}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
}
