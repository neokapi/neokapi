import { Badge, Button, Card } from "@neokapi/ui-primitives";
import type { BlockHistoryEntry } from "../types/api";
import { Clock, ArrowRight } from "./icons";

export interface BlockHistoryPanelProps {
  /** History entries, most-recent first (as returned by the API). */
  entries: BlockHistoryEntry[];
  loading?: boolean;
  /** Restore this version (passes the entry's seq). Enabled when canRollback. */
  onRollback?: (seq: number) => void;
  canRollback?: boolean;
}

function relTime(iso: string): string {
  const d = new Date(iso);
  const diff = Math.floor((Date.now() - d.getTime()) / 1000);
  if (diff < 60) return "just now";
  if (diff < 3600) return Math.floor(diff / 60) + "m ago";
  if (diff < 86400) return Math.floor(diff / 3600) + "h ago";
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function originLabel(origin: string): string {
  switch (origin) {
    case "mt":
      return "machine translation";
    case "ai":
      return "AI";
    case "tm":
      return "translation memory";
    case "human":
      return "human";
    default:
      return origin;
  }
}

/**
 * BlockHistoryPanel renders the version history of a block's target (who changed
 * it, with what role, why, and when) and lets a permitted user restore any prior
 * version (per-edit undo). The current version is the first entry; restoring an
 * earlier one is non-destructive (it appends a new entry).
 */
export function BlockHistoryPanel({
  entries,
  loading,
  onRollback,
  canRollback,
}: BlockHistoryPanelProps) {
  if (!loading && entries.length === 0) {
    return (
      <Card className="p-6 text-center text-sm text-muted-foreground">
        <Clock className="mx-auto mb-2 h-6 w-6 text-muted-foreground/40" />
        No history yet — edits will appear here.
      </Card>
    );
  }

  return (
    <Card className="overflow-hidden p-0">
      <div className="border-b border-border/40 px-4 py-2.5">
        <h3 className="text-sm font-semibold">Version history</h3>
        <p className="text-[12px] text-muted-foreground">
          Who changed this translation, and what it was before
        </p>
      </div>
      <ol className="divide-y divide-border/30">
        {entries.map((e, i) => {
          const isCurrent = i === 0;
          return (
            <li key={e.seq} className="flex items-start gap-3 px-4 py-3 hover:bg-accent/20">
              <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted/60">
                <span className="text-[11px] font-medium text-muted-foreground">
                  {entries.length - i}
                </span>
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  {e.author ? (
                    <span className="text-sm font-medium text-primary">{e.author}</span>
                  ) : (
                    <span className="text-sm text-muted-foreground">unknown</span>
                  )}
                  {e.actorRole && (
                    <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                      {e.actorRole}
                    </Badge>
                  )}
                  {isCurrent && (
                    <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                      current
                    </Badge>
                  )}
                  {e.origin && e.origin !== "human" && (
                    <span className="text-[11px] text-muted-foreground/70">
                      via {originLabel(e.origin)}
                    </span>
                  )}
                </div>
                <p className="mt-0.5 truncate text-sm text-foreground/80" title={e.text}>
                  {e.text || <span className="italic text-muted-foreground">empty</span>}
                </p>
                <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground/60">
                  <span>{relTime(e.timestamp)}</span>
                  <span>·</span>
                  <span>{e.changeType.replace(/_/g, " ")}</span>
                  {e.editReason && (
                    <>
                      <span>·</span>
                      <span className="italic">{e.editReason}</span>
                    </>
                  )}
                </div>
              </div>
              {!isCurrent && canRollback && onRollback && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="shrink-0 gap-1 text-muted-foreground"
                  onClick={() => onRollback(e.seq)}
                  title="Restore this version"
                >
                  <ArrowRight className="h-3.5 w-3.5" /> Restore
                </Button>
              )}
            </li>
          );
        })}
      </ol>
    </Card>
  );
}
