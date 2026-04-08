import { useState, useRef, useEffect } from "react";
import { Loader2, CheckCircle2, XCircle, ListTodo, X, Trash2 } from "lucide-react";
import { Button, ScrollArea } from "@neokapi/ui-primitives";
import { useJobFeed, type Job } from "../context/JobFeedContext";

/**
 * Persistent job feed button in the top-right header area.
 * Shows a badge when jobs are active, and a dropdown panel on click.
 */
export function JobFeedButton({ onViewJob }: { onViewJob?: (job: Job) => void }) {
  const { jobs, hasActive, clearJob, clearAll } = useJobFeed();
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  // Auto-open when a new job starts.
  useEffect(() => {
    if (hasActive) setOpen(true);
  }, [hasActive]);

  const statusIcon = (job: Job) => {
    switch (job.status) {
      case "running":
        return <Loader2 size={13} className="animate-spin text-primary shrink-0" />;
      case "complete":
        return <CheckCircle2 size={13} className="text-green-500 shrink-0" />;
      case "error":
        return <XCircle size={13} className="text-destructive shrink-0" />;
      default:
        return <XCircle size={13} className="text-muted-foreground shrink-0" />;
    }
  };

  const formatDuration = (ms?: number) => {
    if (!ms) return "";
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  return (
    <div className="relative" ref={panelRef}>
      {/* Button */}
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setOpen((v) => !v)}
        className="relative h-7 w-7"
        aria-label="Job feed"
      >
        {hasActive ? (
          <Loader2 size={15} className="animate-spin text-primary" />
        ) : (
          <ListTodo size={15} className="text-muted-foreground" />
        )}
        {/* Badge */}
        {jobs.length > 0 && (
          <span className="absolute -top-0.5 -right-0.5 flex h-3.5 w-3.5 items-center justify-center rounded-full bg-primary text-[8px] font-bold text-primary-foreground">
            {jobs.length}
          </span>
        )}
      </Button>

      {/* Dropdown panel */}
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 w-72 rounded-lg border border-border bg-card shadow-lg overflow-hidden">
          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b border-border bg-muted/30">
            <span className="text-[11px] font-semibold text-foreground">Jobs</span>
            <div className="flex items-center gap-1">
              {jobs.some((j) => j.status !== "running") && (
                <button
                  onClick={clearAll}
                  className="text-[10px] text-muted-foreground hover:text-foreground transition-colors"
                  title="Clear completed"
                >
                  <Trash2 size={11} />
                </button>
              )}
              <button
                onClick={() => setOpen(false)}
                className="p-0.5 rounded hover:bg-muted/60 transition-colors"
              >
                <X size={12} className="text-muted-foreground" />
              </button>
            </div>
          </div>

          {/* Job list */}
          {jobs.length === 0 ? (
            <div className="px-3 py-6 text-center text-xs text-muted-foreground">
              No recent jobs
            </div>
          ) : (
            <ScrollArea className="max-h-64">
              <div className="divide-y divide-border/30">
                {jobs.map((job) => (
                  <div
                    key={job.id}
                    className="flex items-start gap-2 px-3 py-2 hover:bg-muted/30 transition-colors cursor-pointer group"
                    onClick={() => {
                      onViewJob?.(job);
                      setOpen(false);
                    }}
                  >
                    <div className="mt-0.5">{statusIcon(job)}</div>
                    <div className="flex-1 min-w-0">
                      <div className="text-xs font-medium text-foreground truncate">
                        {job.flowName}
                      </div>
                      {job.status === "running" && job.progress.total > 0 && (
                        <div className="mt-1">
                          <div className="flex justify-between text-[10px] text-muted-foreground mb-0.5">
                            <span>
                              {job.progress.current}/{job.progress.total}
                            </span>
                            <span>
                              {Math.round((job.progress.current / job.progress.total) * 100)}%
                            </span>
                          </div>
                          <div className="h-1 rounded-full bg-accent overflow-hidden">
                            <div
                              className="h-full rounded-full bg-primary transition-all duration-300"
                              style={{
                                width: `${(job.progress.current / job.progress.total) * 100}%`,
                              }}
                            />
                          </div>
                        </div>
                      )}
                      {job.status === "complete" && (
                        <div className="text-[10px] text-muted-foreground mt-0.5">
                          {formatDuration(job.durationMs)}
                          {job.events.find((e) => e.files_processed) &&
                            ` — ${job.events.find((e) => e.files_processed)?.files_processed} files`}
                        </div>
                      )}
                      {job.status === "error" && (
                        <div className="text-[10px] text-destructive mt-0.5 truncate">
                          {job.error}
                        </div>
                      )}
                    </div>
                    {job.status !== "running" && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          clearJob(job.id);
                        }}
                        className="opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-muted/60 transition-all shrink-0"
                        title="Dismiss"
                      >
                        <X size={11} className="text-muted-foreground" />
                      </button>
                    )}
                  </div>
                ))}
              </div>
            </ScrollArea>
          )}
        </div>
      )}
    </div>
  );
}
