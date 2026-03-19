import { useState, useMemo } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { useFilter } from '@/context/FilterContext';
import { sessions } from '@/data/sessions';
import SessionDetail from './SessionDetail';

type SortKey = 'agent' | 'started' | 'duration' | 'status';
type SortDir = 'asc' | 'desc';

function formatDuration(secs: number): string {
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return m > 0 ? `${m}m ${s}s` : `${s}s`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export default function SessionTable() {
  const { workspace, agent } = useFilter();
  const [sortKey, setSortKey] = useState<SortKey>('started');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  const filtered = useMemo(() => {
    let s = sessions;
    if (workspace) s = s.filter((sess) => sess.workspace === workspace);
    if (agent) s = s.filter((sess) => sess.agentId === agent);
    return s;
  }, [workspace, agent]);

  const sorted = useMemo(() => {
    const copy = [...filtered];
    copy.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case 'agent':
          cmp = a.agentName.localeCompare(b.agentName);
          break;
        case 'started':
          cmp = new Date(a.startTime).getTime() - new Date(b.startTime).getTime();
          break;
        case 'duration':
          cmp = a.durationSecs - b.durationSecs;
          break;
        case 'status':
          cmp = a.status.localeCompare(b.status);
          break;
      }
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return copy;
  }, [filtered, sortKey, sortDir]);

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(key);
      setSortDir('desc');
    }
  }

  function sortIndicator(key: SortKey) {
    if (sortKey !== key) return '';
    return sortDir === 'asc' ? ' \u2191' : ' \u2193';
  }

  const selectedSession = selectedSessionId
    ? sessions.find((s) => s.id === selectedSessionId) ?? null
    : null;

  // Get unique tool names per session (short list for badges)
  function uniqueTools(session: (typeof sessions)[0]) {
    return [...new Set(session.toolCalls.map((tc) => tc.tool))];
  }

  return (
    <>
      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort('agent')}
              >
                Agent{sortIndicator('agent')}
              </TableHead>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort('started')}
              >
                Started{sortIndicator('started')}
              </TableHead>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort('duration')}
              >
                Duration{sortIndicator('duration')}
              </TableHead>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort('status')}
              >
                Status{sortIndicator('status')}
              </TableHead>
              <TableHead>Tools</TableHead>
              <TableHead className="hidden md:table-cell">Summary</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                  No sessions found.
                </TableCell>
              </TableRow>
            ) : (
              sorted.map((sess) => (
                <TableRow
                  key={sess.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedSessionId(sess.id)}
                >
                  <TableCell className="font-medium">{sess.agentName}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {formatDate(sess.startTime)}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {formatDuration(sess.durationSecs)}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        sess.status === 'succeeded'
                          ? 'default'
                          : sess.status === 'failed'
                            ? 'destructive'
                            : 'secondary'
                      }
                      className="text-xs"
                    >
                      {sess.status}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {uniqueTools(sess).map((tool) => (
                        <Badge key={tool} variant="outline" className="text-[10px]">
                          {tool}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="hidden max-w-[300px] truncate text-xs text-muted-foreground md:table-cell">
                    {sess.summary}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {selectedSession && (
        <SessionDetail
          session={selectedSession}
          onClose={() => setSelectedSessionId(null)}
        />
      )}
    </>
  );
}
