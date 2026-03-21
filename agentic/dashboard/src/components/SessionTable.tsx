import { useState, useMemo } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { useFilter } from "@/context/FilterContext";
import { useApi, groupAuditSessions, type AuditSession } from "@/context/ApiContext";
import SessionDetail from "./SessionDetail";

type SortKey = "actor" | "started" | "events" | "type";
type SortDir = "asc" | "desc";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
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

function isToday(iso: string): boolean {
  return new Date(iso).toDateString() === new Date().toDateString();
}

function isYesterday(iso: string): boolean {
  const yesterday = new Date();
  yesterday.setDate(yesterday.getDate() - 1);
  return new Date(iso).toDateString() === yesterday.toDateString();
}

function isWithinDays(iso: string, days: number): boolean {
  const cutoff = new Date();
  cutoff.setDate(cutoff.getDate() - days);
  cutoff.setHours(0, 0, 0, 0);
  return new Date(iso) >= cutoff;
}

function isThisWeek(iso: string): boolean {
  const now = new Date();
  const startOfWeek = new Date(now);
  startOfWeek.setDate(now.getDate() - now.getDay());
  startOfWeek.setHours(0, 0, 0, 0);
  return new Date(iso) >= startOfWeek;
}

function getActorDisplayName(actor: string): string {
  return actor || "Unknown";
}

function getEventTypes(session: AuditSession): string[] {
  return [...new Set(session.events.map((e) => e.event_type))];
}

export default function SessionTable() {
  const { agent, status, search, tokens } = useFilter();
  const api = useApi();
  const [sortKey, setSortKey] = useState<SortKey>("started");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  const timeToken = tokens.find((t) => t.key === "time");
  const toolToken = tokens.find((t) => t.key === "tool");

  const sessions = useMemo(() => groupAuditSessions(api.auditLog), [api.auditLog]);

  const filtered = useMemo(() => {
    let s = sessions;
    if (agent) s = s.filter((sess) => sess.actor === agent);
    if (status) {
      // No real status from audit log, skip
    }
    if (search) {
      const q = search.toLowerCase();
      s = s.filter((sess) =>
        sess.events.some(
          (e) => e.event_type.toLowerCase().includes(q) || e.data.toLowerCase().includes(q),
        ),
      );
    }
    if (timeToken) {
      const tv = timeToken.value;
      switch (tv) {
        case "today":
          s = s.filter((sess) => isToday(sess.startTime));
          break;
        case "yesterday":
          s = s.filter((sess) => isYesterday(sess.startTime));
          break;
        case "this-week":
          s = s.filter((sess) => isThisWeek(sess.startTime));
          break;
        case "this-month":
        case "30d":
          s = s.filter((sess) => isWithinDays(sess.startTime, 30));
          break;
        case "7d":
          s = s.filter((sess) => isWithinDays(sess.startTime, 7));
          break;
        case "14d":
          s = s.filter((sess) => isWithinDays(sess.startTime, 14));
          break;
      }
    }
    if (toolToken) {
      s = s.filter((sess) => sess.events.some((e) => e.event_type === toolToken.value));
    }
    // Suppress unused variable warning
    void status;
    return s;
  }, [sessions, agent, status, search, timeToken, toolToken]);

  const sorted = useMemo(() => {
    const copy = [...filtered];
    copy.sort((a, b) => {
      let cmp = 0;
      switch (sortKey) {
        case "actor":
          cmp = getActorDisplayName(a.actor).localeCompare(getActorDisplayName(b.actor));
          break;
        case "started":
          cmp = new Date(a.startTime).getTime() - new Date(b.startTime).getTime();
          break;
        case "events":
          cmp = a.eventCount - b.eventCount;
          break;
        case "type":
          cmp = (getEventTypes(a)[0] ?? "").localeCompare(getEventTypes(b)[0] ?? "");
          break;
      }
      return sortDir === "asc" ? cmp : -cmp;
    });
    return copy;
  }, [filtered, sortKey, sortDir]);

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  }

  function sortIndicator(key: SortKey) {
    if (sortKey !== key) return "";
    return sortDir === "asc" ? " \u2191" : " \u2193";
  }

  const selectedSession = selectedSessionId
    ? (sessions.find((s) => s.id === selectedSessionId) ?? null)
    : null;

  if (api.loading) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Loading audit log...</p>;
  }

  return (
    <>
      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="cursor-pointer select-none" onClick={() => handleSort("actor")}>
                Actor{sortIndicator("actor")}
              </TableHead>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort("started")}
              >
                Started{sortIndicator("started")}
              </TableHead>
              <TableHead>Duration</TableHead>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort("events")}
              >
                Events{sortIndicator("events")}
              </TableHead>
              <TableHead className="cursor-pointer select-none" onClick={() => handleSort("type")}>
                Types{sortIndicator("type")}
              </TableHead>
              <TableHead className="hidden md:table-cell">Preview</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                  No audit sessions found.
                </TableCell>
              </TableRow>
            ) : (
              sorted.map((sess) => (
                <TableRow
                  key={sess.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedSessionId(sess.id)}
                >
                  <TableCell className="font-medium">{getActorDisplayName(sess.actor)}</TableCell>
                  <TableCell className="font-mono text-xs">{formatDate(sess.startTime)}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {formatDuration(sess.startTime, sess.endTime)}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{sess.eventCount}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {getEventTypes(sess).map((t) => (
                        <Badge key={t} variant="outline" className="text-[10px]">
                          {t}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="hidden max-w-[300px] truncate text-xs text-muted-foreground md:table-cell">
                    {sess.events[0]?.data
                      ? (() => {
                          try {
                            const d = JSON.parse(sess.events[0].data);
                            return d.block_id ?? d.stream ?? d.item_name ?? sess.events[0].data;
                          } catch {
                            return sess.events[0].data;
                          }
                        })()
                      : "--"}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {selectedSession && (
        <SessionDetail session={selectedSession} onClose={() => setSelectedSessionId(null)} />
      )}
    </>
  );
}
