import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ScrollArea } from "@/components/ui/scroll-area";
import type { AuditSession } from "@/context/ApiContext";

interface SessionDetailProps {
  session: AuditSession;
  onClose: () => void;
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
}

function formatDuration(startIso: string, endIso: string): string {
  const diffMs = new Date(endIso).getTime() - new Date(startIso).getTime();
  const secs = Math.floor(diffMs / 1000);
  if (secs < 60) return `${secs}s`;
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return `${m}m ${s}s`;
}

function parseDataPreview(data: string): string {
  try {
    const parsed = JSON.parse(data);
    return JSON.stringify(parsed, null, 0).slice(0, 80);
  } catch {
    return data.slice(0, 80);
  }
}

export default function SessionDetail({ session, onClose }: SessionDetailProps) {
  const displayName = session.actor || "Unknown";

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{displayName}</DialogTitle>
          <DialogDescription>
            Audit session with {session.eventCount} event
            {session.eventCount !== 1 ? "s" : ""}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="default">{session.eventCount} events</Badge>
          <span className="font-mono text-xs text-muted-foreground">
            {formatDate(session.startTime)}
          </span>
          <span className="font-mono text-xs text-muted-foreground">
            {formatTime(session.startTime)} &mdash; {formatTime(session.endTime)}
          </span>
          <span className="font-mono text-xs font-semibold">
            {formatDuration(session.startTime, session.endTime)}
          </span>
        </div>

        <div>
          <h4 className="mb-2 text-sm font-semibold">Events ({session.eventCount})</h4>
          <ScrollArea className="max-h-[300px] rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Type</TableHead>
                  <TableHead>Time</TableHead>
                  <TableHead>Data</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {session.events.map((evt, i) => (
                  <TableRow key={i}>
                    <TableCell>
                      <Badge variant="outline" className="text-[10px]">
                        {evt.event_type}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {formatTime(evt.created_at)}
                    </TableCell>
                    <TableCell className="max-w-[200px] truncate font-mono text-xs text-muted-foreground">
                      {parseDataPreview(evt.data)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </ScrollArea>
        </div>
      </DialogContent>
    </Dialog>
  );
}
