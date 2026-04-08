import { useState, useRef, useEffect } from "react";
import {
  Loader2,
  CheckCircle2,
  XCircle,
  Ban,
  ListTodo,
  X,
  Trash2,
  ExternalLink,
} from "lucide-react";
import { Button, ScrollArea } from "@neokapi/ui-primitives";
import { useJobFeed, type Job } from "../context/JobFeedContext";

/**
 * Persistent job feed button in the top-right header area.
 * Shows a badge when jobs are active, and a dropdown panel on click.
 */
export function JobFeedButton({ onViewJob }: { onViewJob?: (job: Job) => void }) {
  const { jobs, hasActive, selectJob, clearJob, clearAll } = useJobFeed();
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
        return <Loader2 size={14} className="animate-spin text-primary shrink-0" />;
      case "complete":
        return <CheckCircle2 size={14} className="text-green-500 shrink-0" />;
      case "canceled":
        return <Ban size={14} className="text-muted-foreground shrink-0" />;
      case "error":
        return <XCircle size={14} className="text-destructive shrink-0" />;
    }
  };

  const formatDuration = (ms?: number) => {
    if (!ms) return "";
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
    return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`;
  };

  const relativeTime = (ts: number) => {
    const diff = Date.now() - ts;
    if (diff < 5_000) return "just now";
    if (diff < 60_000) return `${Math.floor(diff / 1000)}s ago`;
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
    return `${Math.floor(diff / 3_600_000)}h ago`;
  };

  const jobTitle = (job: Job) => {
    if (job.projectName) return `${job.projectName} — ${job.flowName}`;
    return job.flowName;
  };

  const handleView = (job: Job) => {
    selectJob(job.id);
    setOpen(false);
    // Defer navigation to avoid updating AppInner while ViewSwitch is rendering.
    queueMicrotask(() => onViewJob?.(job));
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
        {hasActive && (
          <span className="absolute -top-0.5 -right-0.5 flex h-3.5 w-3.5 items-center justify-center rounded-full bg-primary text-[8px] font-bold text-primary-foreground">
            {jobs.filter((j) => j.status === "running").length}
          </span>
        )}
      </Button>

      {/* Dropdown panel */}
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 w-80 rounded-lg border border-border bg-card shadow-lg overflow-hidden">
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
            <ScrollArea className="max-h-72">
              <div className="divide-y divide-border/30">
                {jobs.map((job) => (
                  <div
                    key={job.id}
                    className="px-3 py-2.5 hover:bg-muted/30 transition-colors group"
                  >
                    {/* Title row */}
                    <div className="flex items-center gap-1.5 mb-1">
                      {statusIcon(job)}
                      <span className="text-xs font-medium text-foreground truncate flex-1">
                        {jobTitle(job)}
                      </span>
                      <span className="text-[10px] text-muted-foreground shrink-0">
                        {job.status === "complete" || job.status === "error"
                          ? relativeTime(job.startTime)
                          : ""}
                      </span>
                    </div>

                    {/* Details row */}
                    <div className="flex items-center gap-2 text-[10px] text-muted-foreground ml-5">
                      {job.targetLangs && job.targetLangs.length > 0 && (
                        <span>{job.targetLangs.join(", ")}</span>
                      )}
                      {job.fileCount != null && job.fileCount > 0 && (
                        <span>{job.fileCount} files</span>
                      )}
                      {job.status === "complete" && job.durationMs != null && (
                        <span>{formatDuration(job.durationMs)}</span>
                      )}
                    </div>

                    {/* Progress bar (running) */}
                    {job.status === "running" && job.progress.total > 0 && (
                      <div className="mt-1.5 ml-5">
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

                    {/* Error/canceled message */}
                    {job.status === "canceled" && (
                      <div className="text-[10px] text-muted-foreground mt-1 ml-5">
                        Flow canceled
                      </div>
                    )}
                    {job.status === "error" && job.error && (
                      <div className="text-[10px] text-destructive mt-1 ml-5 truncate">
                        {job.error}
                      </div>
                    )}

                    {/* Actions row */}
                    <div className="flex items-center gap-1 mt-1.5 ml-5">
                      <button
                        onClick={() => handleView(job)}
                        className="text-[10px] text-primary hover:text-primary/80 flex items-center gap-0.5 transition-colors"
                      >
                        <ExternalLink size={10} />
                        View
                      </button>
                      {job.status !== "running" && (
                        <button
                          onClick={() => clearJob(job.id)}
                          className="text-[10px] text-muted-foreground hover:text-foreground ml-auto transition-colors opacity-0 group-hover:opacity-100"
                        >
                          Dismiss
                        </button>
                      )}
                    </div>
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
