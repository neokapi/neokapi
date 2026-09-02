import { useCallback, useEffect, useRef, useState } from "react";
import { Sparkles, Trash2, X } from "lucide-react";
import { Button, LocalePill, ScrollArea, SimpleTooltip, When, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../hooks/useApi";
import type { AIActivityEntry, AIActivityResult } from "../types/api";
import { AIExchangeView } from "./AIExchangeView";

/**
 * The session's AI activity: every model call this app has made, newest first.
 *
 * A per-action disclosure answers "what produced this proposal". This answers
 * the other question, which only a log can: what has been sent on my behalf
 * while I was working, including by a convergence run that made thousands of
 * calls nobody watched. Both read the same record.
 *
 * The log is session-scoped and in memory. Closing the app ends it, which is
 * the honest scope for a diagnostic: it is not an audit trail, and presenting
 * it as one would be a promise the storage does not keep.
 */
export function AIActivityButton() {
  const [open, setOpen] = useState(false);
  const [log, setLog] = useState<AIActivityResult | null>(null);
  const [expanded, setExpanded] = useState<number | null>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  const refresh = useCallback(async () => {
    try {
      setLog(await api.getAIActivity(0));
    } catch {
      // A diagnostic that raises its own error dialog is worse than one that
      // shows nothing: the panel is opened while something else is wrong.
      setLog(null);
    }
  }, []);

  useEffect(() => {
    if (open) void refresh();
  }, [open, refresh]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  const entries = log?.entries ?? [];

  const clear = async () => {
    await api.clearAIActivity();
    setExpanded(null);
    void refresh();
  };

  return (
    <div className="relative" ref={panelRef}>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setOpen((v) => !v)}
        className="relative h-7 w-7"
        aria-label={t("AI activity")}
        data-slot="ai-activity-button"
      >
        <Sparkles size={15} className="text-muted-foreground" />
      </Button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-[32rem] overflow-hidden rounded-lg border border-border bg-card shadow-lg">
          <div className="flex items-center justify-between border-b border-border bg-muted/30 px-3 py-2">
            <span className="text-[11px] font-semibold text-foreground">{t("AI activity")}</span>
            <div className="flex items-center gap-1">
              {entries.length > 0 && (
                <SimpleTooltip content={t("Clear")}>
                  <button
                    onClick={() => void clear()}
                    className="text-muted-foreground transition-colors hover:text-foreground"
                    aria-label={t("Clear")}
                  >
                    <Trash2 size={11} />
                  </button>
                </SimpleTooltip>
              )}
              <button
                onClick={() => setOpen(false)}
                className="rounded p-0.5 transition-colors hover:bg-muted/60"
                aria-label={t("Close")}
              >
                <X size={12} className="text-muted-foreground" />
              </button>
            </div>
          </div>

          {entries.length === 0 ? (
            <div className="px-3 py-6 text-center text-xs text-muted-foreground">
              {t("Nothing has been sent to a model in this session.")}
            </div>
          ) : (
            <ScrollArea className="max-h-[28rem]">
              <div className="divide-y divide-border/30">
                {entries.map((e) => (
                  <ActivityRow
                    key={e.id}
                    entry={e}
                    open={expanded === e.id}
                    onToggle={() => setExpanded(expanded === e.id ? null : e.id)}
                  />
                ))}
              </div>
              {log && log.dropped > 0 && (
                <p className="px-3 py-2 text-[10px] text-muted-foreground">
                  {t("{count} older calls are no longer held (the log keeps the last {cap}).", {
                    count: log.dropped,
                    cap: log.cap,
                  })}
                </p>
              )}
            </ScrollArea>
          )}
        </div>
      )}
    </div>
  );
}

function surfaceLabel(surface: string, action?: string): string {
  switch (surface) {
    case "review":
      return action ? t("Review · {action}", { action }) : t("Review");
    case "pre-review":
      return t("AI pre-review");
    case "convergence":
      return t("Convergence run");
    case "flow":
      return action ? t("Flow · {action}", { action }) : t("Flow");
    default:
      return surface || t("Other");
  }
}

function ActivityRow({
  entry,
  open,
  onToggle,
}: {
  entry: AIActivityEntry;
  open: boolean;
  onToggle: () => void;
}) {
  return (
    <div className={cn(open && "bg-muted/20")}>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-muted/40"
      >
        <span className="min-w-0 flex-1 truncate text-[11px]">
          {surfaceLabel(entry.scope.surface, entry.scope.action)}
          {entry.scope.file && (
            <span className="ml-1.5 text-muted-foreground" translate="no">
              {entry.scope.file}
              {entry.scope.key ? `:${entry.scope.key}` : ""}
            </span>
          )}
        </span>
        {entry.scope.locale && <LocalePill locale={entry.scope.locale} />}
        {entry.error && <span className="text-[10px] text-destructive">{t("failed")}</span>}
        <When
          iso={entry.at}
          dateStyle="none"
          className="shrink-0 text-[10px] text-muted-foreground"
        />
      </button>
      {open && (
        <div className="px-3 pb-3">
          <AIExchangeView exchange={entry.exchange} />
        </div>
      )}
    </div>
  );
}
