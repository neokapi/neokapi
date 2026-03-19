import { useMemo } from 'react';
import { Badge } from '@/components/ui/badge';
import { memoryLog } from '@/data/memory';
import { useFilter } from '@/context/FilterContext';

function formatRelativeTime(date: Date): string {
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d ago`;
}

export default function MemoryTab() {
  const { workspace, agent, search } = useFilter();

  const filtered = useMemo(() => {
    let entries = memoryLog;
    // Sort by timestamp descending
    entries = [...entries].sort(
      (a, b) => b.timestamp.getTime() - a.timestamp.getTime()
    );
    if (agent) entries = entries.filter((e) => e.agentId === agent);
    if (search) {
      const q = search.toLowerCase();
      entries = entries.filter((e) => e.summary.toLowerCase().includes(q));
    }
    // Workspace filter not needed since all memory is excalidraw for now
    void workspace;
    return entries;
  }, [workspace, agent, search]);

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Git log of agent-memory repo -- what agents learned per session
      </p>

      {filtered.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          No memory entries found.
        </p>
      ) : (
        <div className="space-y-0 rounded-lg border divide-y">
          {filtered.map((entry) => (
            <div
              key={entry.id}
              className="flex items-start gap-3 px-4 py-3 transition-colors hover:bg-accent/20"
            >
              <span className="font-mono text-[11px] text-muted-foreground/60 mt-0.5 shrink-0 w-14">
                {formatRelativeTime(entry.timestamp)}
              </span>
              <Badge variant="secondary" className="text-[10px] shrink-0 mt-0.5">
                {entry.agentName.split(' ')[0].toLowerCase()}
              </Badge>
              <div className="min-w-0 flex-1">
                <p className="text-xs text-foreground/80">{entry.summary}</p>
              </div>
              <span className="font-mono text-[10px] shrink-0 mt-0.5">
                <span className="text-green-500">+{entry.additions}</span>
                {entry.deletions > 0 && (
                  <span className="text-destructive ml-1">-{entry.deletions}</span>
                )}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
