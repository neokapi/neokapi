import { useMemo } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ExternalLink } from 'lucide-react';
import { activityFeed } from '@/data/activity';
import { useFilter } from '@/context/FilterContext';

function formatTime(date: Date): string {
  return date.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  });
}

function dateLabel(date: Date): string {
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  if (date.toDateString() === today.toDateString()) return 'Today';
  if (date.toDateString() === yesterday.toDateString()) return 'Yesterday';
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'short',
    day: 'numeric',
  });
}

export default function ActivityTab() {
  const { workspace, agent, search } = useFilter();

  const filtered = useMemo(() => {
    let entries = activityFeed;
    if (workspace) entries = entries.filter((e) => e.workspace === workspace);
    if (agent) entries = entries.filter((e) => e.agentId === agent);
    if (search) {
      const q = search.toLowerCase();
      entries = entries.filter((e) => e.action.toLowerCase().includes(q));
    }
    return entries;
  }, [workspace, agent, search]);


  // Group by date
  const grouped = useMemo(() => {
    const groups: { label: string; entries: typeof filtered }[] = [];
    let currentLabel = '';
    for (const entry of filtered) {
      const label = dateLabel(entry.timestamp);
      if (label !== currentLabel) {
        groups.push({ label, entries: [] });
        currentLabel = label;
      }
      groups[groups.length - 1].entries.push(entry);
    }
    return groups;
  }, [filtered]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          Agent actions from the Bowrain activity feed
        </p>
        <Button
          variant="ghost"
          size="sm"
          render={
            <a
              href="https://dev.bowrain.cloud/activity"
              target="_blank"
              rel="noopener noreferrer"
            />
          }
          className="gap-1.5 text-muted-foreground hover:text-foreground"
        >
          View full feed in Bowrain
          <ExternalLink className="h-3.5 w-3.5" />
        </Button>
      </div>

      {grouped.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          No activity found.
        </p>
      ) : (
        grouped.map((group) => (
          <div key={group.label}>
            <div className="sticky top-0 z-10 mb-2 bg-background/95 backdrop-blur">
              <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                {group.label}
              </span>
            </div>
            <div className="space-y-1.5">
              {group.entries.map((entry) => (
                <div
                  key={entry.id}
                  className="flex items-start gap-3 rounded-md px-3 py-2 transition-colors hover:bg-accent/30"
                >
                  <span className="font-mono text-[11px] text-muted-foreground/60 mt-0.5 shrink-0 w-14">
                    {formatTime(entry.timestamp)}
                  </span>
                  <Badge variant="secondary" className="text-[10px] shrink-0 mt-0.5">
                    {entry.agentName}
                  </Badge>
                  <div className="min-w-0 flex-1">
                    <p className="text-xs text-foreground/80 leading-relaxed">
                      {entry.action}
                    </p>
                    {entry.toolsUsed.length > 0 && (
                      <div className="mt-1 flex flex-wrap gap-1">
                        {entry.toolsUsed.map((tool) => (
                          <Badge key={tool} variant="outline" className="text-[9px]">
                            {tool}
                          </Badge>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))
      )}
    </div>
  );
}
