import { useState, useEffect } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Loader2, CheckCircle2, XCircle, ListTodo, X, Trash2 } from "lucide-react";
import { Button, ScrollArea } from "@neokapi/ui-primitives";

// ---------------------------------------------------------------------------
// Simulated job feed (mirrors JobFeedButton without needing real context)
// ---------------------------------------------------------------------------

type JobStatus = "running" | "complete" | "error";

interface SimJob {
  id: string;
  flowName: string;
  status: JobStatus;
  progress: { current: number; total: number };
  durationMs?: number;
  error?: string;
}

function SimulatedJobFeed({
  initialJobs = [],
  simulateProgress = false,
}: {
  initialJobs?: SimJob[];
  simulateProgress?: boolean;
}) {
  const [jobs, setJobs] = useState<SimJob[]>(initialJobs);
  const [open, setOpen] = useState(true);

  // Simulate progress on running jobs.
  useEffect(() => {
    if (!simulateProgress) return;
    const interval = setInterval(() => {
      setJobs((prev) =>
        prev.map((j) => {
          if (j.status !== "running") return j;
          const next = j.progress.current + 1;
          if (next >= j.progress.total) {
            return {
              ...j,
              status: "complete",
              durationMs: 4200,
              progress: { ...j.progress, current: j.progress.total },
            };
          }
          return { ...j, progress: { ...j.progress, current: next } };
        }),
      );
    }, 800);
    return () => clearInterval(interval);
  }, [simulateProgress]);

  const hasActive = jobs.some((j) => j.status === "running");

  const statusIcon = (job: SimJob) => {
    switch (job.status) {
      case "running":
        return <Loader2 size={13} className="animate-spin text-primary shrink-0" />;
      case "complete":
        return <CheckCircle2 size={13} className="text-green-500 shrink-0" />;
      case "error":
        return <XCircle size={13} className="text-destructive shrink-0" />;
    }
  };

  return (
    <div className="relative inline-block">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setOpen((v) => !v)}
        className="relative h-7 w-7"
      >
        {hasActive ? (
          <Loader2 size={15} className="animate-spin text-primary" />
        ) : (
          <ListTodo size={15} className="text-muted-foreground" />
        )}
        {jobs.length > 0 && (
          <span className="absolute -top-0.5 -right-0.5 flex h-3.5 w-3.5 items-center justify-center rounded-full bg-primary text-[8px] font-bold text-primary-foreground">
            {jobs.length}
          </span>
        )}
      </Button>

      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 w-72 rounded-lg border border-border bg-card shadow-lg overflow-hidden">
          <div className="flex items-center justify-between px-3 py-2 border-b border-border bg-muted/30">
            <span className="text-[11px] font-semibold text-foreground">Jobs</span>
            <div className="flex items-center gap-1">
              <button className="text-[10px] text-muted-foreground hover:text-foreground">
                <Trash2 size={11} />
              </button>
              <button onClick={() => setOpen(false)} className="p-0.5 rounded hover:bg-muted/60">
                <X size={12} className="text-muted-foreground" />
              </button>
            </div>
          </div>

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
                    className="flex items-start gap-2 px-3 py-2 hover:bg-muted/30 cursor-pointer group"
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
                          {job.durationMs ? `${(job.durationMs / 1000).toFixed(1)}s` : ""}
                        </div>
                      )}
                      {job.status === "error" && (
                        <div className="text-[10px] text-destructive mt-0.5 truncate">
                          {job.error}
                        </div>
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

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof SimulatedJobFeed> = {
  title: "App/JobFeedButton",
  component: SimulatedJobFeed,
  parameters: {
    layout: "padded",
    docs: {
      description: {
        component:
          "Persistent job feed button with dropdown panel. Shows active, completed, and errored flow executions.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimulatedJobFeed>;

export const Empty: Story = {
  args: { initialJobs: [] },
};

export const RunningJob: Story = {
  name: "Running job with progress",
  args: {
    initialJobs: [
      { id: "1", flowName: "translate", status: "running", progress: { current: 5, total: 16 } },
    ],
    simulateProgress: true,
  },
};

export const CompletedJob: Story = {
  args: {
    initialJobs: [
      {
        id: "1",
        flowName: "translate",
        status: "complete",
        progress: { current: 16, total: 16 },
        durationMs: 12400,
      },
    ],
  },
};

export const ErroredJob: Story = {
  args: {
    initialJobs: [
      {
        id: "1",
        flowName: "translate-and-qa",
        status: "error",
        progress: { current: 3, total: 16 },
        error: "API rate limit exceeded",
      },
    ],
  },
};

export const MultipleJobs: Story = {
  name: "Multiple jobs (mixed status)",
  args: {
    initialJobs: [
      {
        id: "1",
        flowName: "translate (de-DE)",
        status: "running",
        progress: { current: 8, total: 16 },
      },
      {
        id: "2",
        flowName: "translate (fr-FR)",
        status: "complete",
        progress: { current: 16, total: 16 },
        durationMs: 8200,
      },
      {
        id: "3",
        flowName: "qa",
        status: "error",
        progress: { current: 2, total: 10 },
        error: "Provider unavailable",
      },
    ],
    simulateProgress: true,
  },
};
