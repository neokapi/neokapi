import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { AgentSession } from '@/data/sessions';
import { agents } from '@/data/agents';

interface SessionDetailProps {
  session: AgentSession;
  onClose: () => void;
}

function formatDuration(secs: number): string {
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return m > 0 ? `${m}m ${s}s` : `${s}s`;
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  });
}

export default function SessionDetail({ session, onClose }: SessionDetailProps) {
  const agent = agents.find((a) => a.id === session.agentId);

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{session.agentName}</DialogTitle>
          <DialogDescription>
            {agent?.role} -- {session.workspace}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant={
              session.status === 'succeeded'
                ? 'default'
                : session.status === 'failed'
                  ? 'destructive'
                  : 'secondary'
            }
          >
            {session.status}
          </Badge>
          <span className="font-mono text-xs text-muted-foreground">
            {formatDate(session.startTime)}
          </span>
          <span className="font-mono text-xs text-muted-foreground">
            {formatTime(session.startTime)} &mdash; {formatTime(session.endTime)}
          </span>
          <span className="font-mono text-xs font-semibold">
            {formatDuration(session.durationSecs)}
          </span>
        </div>

        <div className="rounded-md bg-muted p-3">
          <p className="text-sm text-muted-foreground">{session.summary}</p>
        </div>

        <div>
          <h4 className="mb-2 text-sm font-semibold">
            Tool Calls ({session.toolCalls.length})
          </h4>
          <div className="max-h-[300px] overflow-y-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tool</TableHead>
                  <TableHead>Time</TableHead>
                  <TableHead>Duration</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {session.toolCalls.map((tc, i) => (
                  <TableRow key={i}>
                    <TableCell className="font-mono text-xs">{tc.tool}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {formatTime(tc.timestamp)}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {tc.durationMs}ms
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={tc.success ? 'default' : 'destructive'}
                        className="text-[10px]"
                      >
                        {tc.success ? 'OK' : 'FAIL'}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
