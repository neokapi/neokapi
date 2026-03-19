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

function isToday(iso: string): boolean {
  const d = new Date(iso);
  const today = new Date();
  return d.toDateString() === today.toDateString();
}

function isYesterday(iso: string): boolean {
  const d = new Date(iso);
  const yesterday = new Date();
  yesterday.setDate(yesterday.getDate() - 1);
  return d.toDateString() === yesterday.toDateString();
}

function isThisWeek(iso: string): boolean {
  const d = new Date(iso);
  const now = new Date();
  const startOfWeek = new Date(now);
  startOfWeek.setDate(now.getDate() - now.getDay());
  startOfWeek.setHours(0, 0, 0, 0);
  return d >= startOfWeek;
}

function isWithinDays(iso: string, days: number): boolean {
  const d = new Date(iso);
  const cutoff = new Date();
  cutoff.setDate(cutoff.getDate() - days);
  cutoff.setHours(0, 0, 0, 0);
  return d >= cutoff;
}

export default function SessionTable() {
  const { workspace, agent, status, search, tokens } = useFilter();
  const [sortKey, setSortKey] = useState<SortKey>('started');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  const timeToken = tokens.find((t) => t.key === 'time');
  const toolToken = tokens.find((t) => t.key === 'tool');

  const filtered = useMemo(() => {
    let s = sessions;
    if (workspace) s = s.filter((sess) => sess.workspace === workspace);
    if (agent) s = s.filter((sess) => sess.agentId === agent);
    if (status) s = s.filter((sess) => sess.status === status);
    if (search) {
      const q = search.toLowerCase();
      s = s.filter((sess) => sess.summary.toLowerCase().includes(q));
    }
    if (timeToken) {
      const tv = timeToken.value;
      switch (tv) {
        case 'today':
          s = s.filter((sess) => isToday(sess.startTime));
          break;
        case 'yesterday':
          s = s.filter((sess) => isYesterday(sess.startTime));
          break;
        case 'this-week':
          s = s.filter((sess) => isThisWeek(sess.startTime));
          break;
        case 'this-month':
          s = s.filter((sess) => isWithinDays(sess.startTime, 30));
          break;
        case '7d':
          s = s.filter((sess) => isWithinDays(sess.startTime, 7));
          break;
        case '14d':
          s = s.filter((sess) => isWithinDays(sess.startTime, 14));
          break;
        case '30d':
          s = s.filter((sess) => isWithinDays(sess.startTime, 30));
          break;
      }
    }
    if (toolToken) {
      s = s.filter((sess) => sess.toolCalls.some((tc) => tc.tool === toolToken.value));
    }
    return s;
  }, [workspace, agent, status, search, timeToken, toolToken]);

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
