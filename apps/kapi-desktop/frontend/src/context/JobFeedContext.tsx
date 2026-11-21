import { createContext, useContext, useState, useCallback, useRef, useEffect } from "react";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { parseAppError } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { captureEvent, durationBucket } from "../analytics";
import type { RunError } from "../types/api";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type JobStatus = "running" | "complete" | "error" | "canceled";

export interface StepSnapshot {
  name: string;
  parts_in: number;
  parts_out: number;
}

export interface RunEvent {
  type:
    | "state"
    | "progress"
    | "trace"
    | "error"
    | "complete"
    | "pipeline_metrics"
    | "converge_event";
  flow_id: string;
  tab_id?: string;
  message?: string;
  /** The locale pass of a state, progress or trace event; absent on a source-only pass. */
  locale?: string;
  file_index?: number;
  file_count?: number;
  file_path?: string;
  duration_ms?: number;
  files_processed?: number;
  steps?: StepSnapshot[];
  /**
   * One typed progress event of a convergence run (type == "converge_event") —
   * the per-pass/per-locale stream the live run view reduces into locale rows.
   */
  converge_event?: import("../types/api").ConvergeEvent;
  /** Final structured convergence result (on a convergence run's "complete"). */
  converge_result?: import("../types/api").ConvergeOutput;
  /**
   * The failure as structure (type == "error"): headline + remediation actions +
   * affected file/locale + the raw chain. `message` keeps the raw text.
   */
  error?: import("../types/api").RunError;
}

export interface Job {
  id: string;
  tabID?: string;
  converge?: boolean;
  flowName: string;
  projectName?: string;
  targetLangs?: string[];
  fileCount?: number;
  status: JobStatus;
  events: RunEvent[];
  progress: { current: number; total: number };
  stepSnapshots: StepSnapshot[];
  startTime: number;
  durationMs?: number;
  /** Friendly one-liner for the feed row (the classified headline when there is one). */
  error?: string;
  /**
   * The classified failure, when the backend produced one — headline,
   * remediation actions, affected file/locale, raw chain. The feed renders this
   * in preference to `error` so a run failure arrives as structure, not as a
   * wrapped Go error chain.
   */
  runError?: RunError;
}

type JobDetails = Pick<
  Job,
  "flowName" | "projectName" | "targetLangs" | "fileCount" | "converge"
> & { tabID: string };

interface JobFeedContextValue {
  jobs: Job[];
  activeJob: Job | null;
  hasActive: boolean;
  launchJob: (details: JobDetails, launch: () => Promise<unknown>) => Promise<void>;
  clearJob: (id: string) => void;
  clearAll: () => void;
}

const JobFeedContext = createContext<JobFeedContextValue>({
  jobs: [],
  activeJob: null,
  hasActive: false,
  launchJob: async (_, launch) => {
    await launch();
  },
  clearJob: () => {},
  clearAll: () => {},
});

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

const MAX_JOBS = 20;

function newJob(details: Omit<JobDetails, "tabID"> & { tabID?: string }): Job {
  return {
    ...details,
    id: crypto.randomUUID(),
    status: "running",
    events: [],
    progress: { current: 0, total: 0 },
    stepSnapshots: [],
    startTime: Date.now(),
  };
}

function applyEvent(job: Job, event: RunEvent): Job {
  switch (event.type) {
    case "progress":
      return {
        ...job,
        progress: {
          current: (event.file_index ?? 0) + 1,
          total: event.file_count ?? job.progress.total,
        },
      };
    case "complete":
      return {
        ...job,
        status: "complete",
        durationMs: event.duration_ms,
        progress: { ...job.progress, current: job.progress.total },
      };
    case "error": {
      const message = event.message ?? "Flow execution failed";
      const canceled = event.error?.kind === "canceled" || /context cancel(?:l)?ed/.test(message);
      return {
        ...job,
        status: canceled ? "canceled" : "error",
        error: canceled ? "Flow canceled" : (event.error?.headline ?? message),
        runError: canceled ? undefined : event.error,
      };
    }
    case "converge_event":
      return { ...job, converge: true };
    case "pipeline_metrics":
      return { ...job, stepSnapshots: event.steps ?? job.stepSnapshots };
    default:
      return job;
  }
}

export function JobFeedProvider({ children }: { children: React.ReactNode }) {
  const [jobs, setJobs] = useState<Job[]>([]);
  const active = useRef<Job | null>(null);
  const [confirmed, setConfirmed] = useState<Job | null>(null);

  const add = useCallback((job: Job) => {
    if (active.current) throw new Error("A flow is already running.");
    active.current = job;
    setJobs((prev) => [job, ...prev].slice(0, MAX_JOBS));
  }, []);

  const finish = useCallback((job: Job) => {
    if (active.current?.id !== job.id) return;
    active.current = null;
    setConfirmed(null);
    captureEvent("flow_run_completed", {
      flow: job.flowName,
      outcome:
        job.status === "complete" ? "completed" : job.status === "canceled" ? "canceled" : "failed",
      duration_bucket: durationBucket(job.durationMs ?? Date.now() - job.startTime),
    });
  }, []);

  const launchJob = useCallback(
    async (details: JobDetails, launch: () => Promise<unknown>) => {
      const job = newJob(details);
      add(job);
      captureEvent("flow_run_started", { flow: job.flowName, file_count: job.fileCount });
      try {
        await launch();
        if (active.current?.id === job.id) setConfirmed(job);
      } catch (err) {
        const failed: Job = { ...job, status: "error", error: parseAppError(err).title };
        finish(failed);
        setJobs((prev) => prev.map((item) => (item.id === job.id ? failed : item)));
        throw err;
      }
    },
    [add, finish],
  );

  useWailsEvent("flow:event", (data) => {
    const event = data as RunEvent;
    let job = active.current;
    if (!job) {
      if (event.type !== "state" || event.message !== "running") return;
      job = newJob({ flowName: event.flow_id, tabID: event.tab_id });
      add(job);
      setConfirmed(job);
    }
    if (event.tab_id !== job.tabID) return;
    const id = job.id;
    if (event.type === "complete" || event.type === "error") finish(applyEvent(job, event));
    setJobs((prev) =>
      prev.map((item) =>
        item.id === id ? { ...applyEvent(item, event), events: [...item.events, event] } : item,
      ),
    );
  });

  useEffect(() => {
    if (!confirmed) return;
    let stopped = false;
    const reconcile = async () => {
      const state = await api.getRunState();
      if (stopped || (state !== "complete" && state !== "error" && state !== "canceled")) return;
      const events = (await api.getRunEvents()) as RunEvent[] | null;
      if (stopped || active.current?.id !== confirmed.id || !events?.length) return;
      if (events.some((e) => e.tab_id !== confirmed.tabID)) return;
      const updated = events.reduce(applyEvent, { ...confirmed, events });
      if (updated.status === "running") return;
      finish(updated);
      setJobs((prev) => prev.map((item) => (item.id === updated.id ? updated : item)));
    };
    const poll = () => {
      void reconcile().catch(() => {});
    };
    poll();
    const interval = setInterval(poll, 750);
    return () => {
      stopped = true;
      clearInterval(interval);
    };
  }, [confirmed, finish]);

  const activeJob = jobs.find((j) => j.status === "running") ?? null;
  const hasActive = activeJob !== null;

  const clearJob = useCallback((id: string) => {
    setJobs((prev) => prev.filter((j) => j.status === "running" || j.id !== id));
  }, []);

  const clearAll = useCallback(() => {
    setJobs((prev) => prev.filter((j) => j.status === "running"));
  }, []);

  return (
    <JobFeedContext.Provider
      value={{
        jobs,
        activeJob,
        hasActive,
        launchJob,
        clearJob,
        clearAll,
      }}
    >
      {children}
    </JobFeedContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useJobFeed() {
  return useContext(JobFeedContext);
}
