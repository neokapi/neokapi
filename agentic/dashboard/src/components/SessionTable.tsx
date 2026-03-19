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
import { useFilter } from '../context/FilterContext';
import { sessions } from '../data/sessions';
import type { AgentSession } from '../data/sessions';
import SessionDetail from './SessionDetail';

type SortKey = 'agent' | 'time' | 'duration' | 'status';
type SortDir = 'asc' | 'desc';

function formatDuration(secs: number): string {
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return m > 0 ? `${m}m ${s}s` : `${s}s`;
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export default function SessionTable() {
  const { filters } = useFilter();
  const [sortKey, setSortKey] = useState<SortKey>('time');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [selectedSession, setSelectedSession] = useState<AgentSession | null>(null);

  const handleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(key);
      setSortDir('desc');
    }
  };

  const sortIndicator = (key: SortKey) => {
    if (sortKey !== key) return '';
    return sortDir === 'asc' ? ' \u2191' : ' \u2193';
  };

  const filtered = useMemo(() => {
    let s = sessions;
    if (filters.workspace) {
      s = s.filter((sess) => sess.workspace === filters.workspace);
    }
    if (filters.agent) {
      s = s.filter((sess) => sess.agentId === filters.agent);
    }

    const sorted = [...s].sort((a, b) => {
      const dir = sortDir === 'asc' ? 1 : -1;
      switch (sortKey) {
        case 'agent':
          return dir * a.agentName.localeCompare(b.agentName);
        case 'time':
          return dir * (new Date(a.startTime).getTime() - new Date(b.startTime).getTime());
        case 'duration':
          return dir * (a.durationSecs - b.durationSecs);
        case 'status':
          return dir * a.status.localeCompare(b.status);
        default:
          return 0;
      }
    });

    return sorted.slice(0, 50);
  }, [filters.workspace, filters.agent, sortKey, sortDir]);

  const uniqueTools = (sess: AgentSession) => {
    const tools = new Set(sess.toolCalls.map((tc) => tc.tool));
    return [...tools];
  };

  return (
    <>
      <div className="rounded-md border">
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
                onClick={() => handleSort('time')}
              >
                Started{sortIndicator('time')}
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
              <TableHead className="hidden lg:table-cell">Summary</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                  No sessions found.
                </TableCell>
              </TableRow>
            ) : (
              filtered.map((sess) => (
                <TableRow
                  key={sess.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedSession(sess)}
                >
                  <TableCell className="font-medium">{sess.agentName}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {formatTime(sess.startTime)}
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
                  <TableCell className="hidden max-w-xs truncate text-xs text-muted-foreground lg:table-cell">
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
          onClose={() => setSelectedSession(null)}
        />
      )}
    </>
  );
}
