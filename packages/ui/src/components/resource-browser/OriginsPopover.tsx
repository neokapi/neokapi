import { useState, useCallback, useEffect, useRef } from "react";
import type { OriginDTO, ImportSessionDTO } from "./types";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { FileText, Wrench, Upload, User, GitCommit } from "lucide-react";
import { relativeTime } from "./utils";

interface OriginsPopoverProps {
  origins: OriginDTO[];
  note?: string;
  /**
   * Optional adapter for resolving import-session metadata. When provided
   * and an origin carries a session_id, this component fetches the session
   * lazily (on popover open) and renders tool + version + imported_at +
   * entry count alongside the origin entry.
   */
  getImportSession?: (id: string) => Promise<ImportSessionDTO | null>;
}

function sourceIcon(source: string) {
  switch (source) {
    case "file":
      return <FileText className="size-3 shrink-0" />;
    case "tool":
      return <Wrench className="size-3 shrink-0" />;
    case "import":
      return <Upload className="size-3 shrink-0" />;
    case "user":
      return <User className="size-3 shrink-0" />;
    default:
      return <GitCommit className="size-3 shrink-0" />;
  }
}

/**
 * Shows a count badge of provenance origins. Clicking opens a popover
 * with the full list (source, key, reference, when, who) and the
 * translator-visible note if present. When an origin carries a session_id
 * and an `getImportSession` fetcher is provided, the session metadata
 * (tool, version, imported_at, entry count) is fetched lazily and cached
 * per session_id.
 */
export function OriginsPopover({ origins, note, getImportSession }: OriginsPopoverProps) {
  const [open, setOpen] = useState(false);
  const [sessionCache, setSessionCache] = useState<Record<string, ImportSessionDTO | null>>({});
  const inflightRef = useRef<Record<string, Promise<void>>>({});

  const ensureSession = useCallback(
    (id: string) => {
      if (!getImportSession) return;
      if (id in sessionCache) return;
      if (id in inflightRef.current) return;
      inflightRef.current[id] = (async () => {
        try {
          const s = await getImportSession(id);
          setSessionCache((prev) => ({ ...prev, [id]: s }));
        } catch {
          setSessionCache((prev) => ({ ...prev, [id]: null }));
        } finally {
          delete inflightRef.current[id];
        }
      })();
    },
    [getImportSession, sessionCache],
  );

  useEffect(() => {
    if (!open || !getImportSession) return;
    for (const o of origins) {
      if (o.session_id) ensureSession(o.session_id);
    }
  }, [open, origins, getImportSession, ensureSession]);

  if ((!origins || origins.length === 0) && !note) return null;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          className="inline-flex items-center gap-1 px-1.5 py-px rounded bg-muted text-[10px] font-medium text-muted-foreground hover:bg-muted/80 hover:text-foreground transition-colors"
          onClick={(e) => e.stopPropagation()}
          title="Provenance"
        >
          {note && <FileText className="size-2.5" />}
          {origins && origins.length > 0 && (
            <>
              <GitCommit className="size-2.5" />
              <span className="tabular-nums">{origins.length}</span>
            </>
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-3" align="start" onClick={(e) => e.stopPropagation()}>
        {note && (
          <div className="mb-2 pb-2 border-b border-border">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-1">
              Note
            </div>
            <div className="text-[12px] text-foreground">{note}</div>
          </div>
        )}
        {origins && origins.length > 0 && (
          <div className="flex flex-col gap-2">
            <div className="text-[10px] uppercase tracking-wider text-muted-foreground">
              {origins.length} {origins.length === 1 ? "origin" : "origins"}
            </div>
            {origins.map((o, i) => {
              const session = o.session_id ? sessionCache[o.session_id] : undefined;
              return (
                <div key={i} className="flex items-start gap-2 text-[11px]">
                  {sourceIcon(o.source)}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-baseline gap-2">
                      <span className="font-medium text-foreground">{o.source}</span>
                      {o.added_by && <span className="text-muted-foreground">by {o.added_by}</span>}
                      <span className="text-muted-foreground ml-auto">
                        {relativeTime(o.added_at)}
                      </span>
                    </div>
                    {o.key && (
                      <div className="text-muted-foreground font-mono text-[10px] break-all mt-0.5">
                        {o.key}
                      </div>
                    )}
                    {o.reference && (
                      <div className="text-muted-foreground font-mono text-[10px] break-all">
                        {o.reference}
                      </div>
                    )}
                    {o.session_id && (
                      <div
                        className="mt-1 rounded bg-muted/50 px-2 py-1 text-[10px] text-muted-foreground"
                        data-testid={`origin-session-${o.session_id}`}
                      >
                        {session === undefined && <span>Loading session…</span>}
                        {session === null && <span>Session {o.session_id.slice(0, 8)}…</span>}
                        {session && (
                          <div className="flex flex-col gap-0.5">
                            <div className="flex items-baseline gap-1">
                              <span className="font-medium text-foreground">
                                {session.tool_name || "import"}
                              </span>
                              {session.tool_version && (
                                <span className="text-[9px]">{session.tool_version}</span>
                              )}
                              <span className="ml-auto tabular-nums">
                                {session.entry_count}{" "}
                                {session.entry_count === 1 ? "entry" : "entries"}
                              </span>
                            </div>
                            <div className="flex items-baseline gap-1 text-[9px]">
                              <span>{relativeTime(session.imported_at)}</span>
                              {session.file_key && (
                                <span className="truncate font-mono" title={session.file_key}>
                                  {session.file_key}
                                </span>
                              )}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
