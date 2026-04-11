import { useState } from "react";
import type { OriginDTO } from "./types";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import { FileText, Wrench, Upload, User, GitCommit } from "lucide-react";
import { relativeTime } from "./utils";

interface OriginsPopoverProps {
  origins: OriginDTO[];
  note?: string;
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
 * translator-visible note if present.
 */
export function OriginsPopover({ origins, note }: OriginsPopoverProps) {
  const [open, setOpen] = useState(false);

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
            {origins.map((o, i) => (
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
                </div>
              </div>
            ))}
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
