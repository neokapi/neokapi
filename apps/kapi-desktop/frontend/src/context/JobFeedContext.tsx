import { createContext, useContext, useState, useCallback, useRef, useEffect } from "react";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { api } from "../hooks/useApi";

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
  type: "state" | "progress" | "trace" | "error" | "complete" | "pipeline_metrics";
  flow_id: string;
  message?: string;
  file_index?: number;
  file_count?: number;
  file_path?: string;
  duration_ms?: number;
  files_processed?: number;
  steps?: StepSnapshot[];
}

export interface Job {
  id: string;
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
  error?: string;
}

interface JobFeedContextValue {
  jobs: Job[];
  activeJob: Job | null;
  selectedJobId: string | null;
  selectedJob: Job | null;
  hasActive: boolean;
  /** Pre-create a job with full context before the backend emits "running". */
  startJob: (
    flowName: string,
    projectName?: string,
    targetLangs?: string[],
    fileCount?: number,
  ) => void;
  selectJob: (id: string | null) => void;
  clearJob: (id: string) => void;
  clearAll: () => void;
}

const JobFeedContext = createContext<JobFeedContextValue>({
  jobs: [],
  activeJob: null,
  selectedJobId: null,
  selectedJob: null,
  hasActive: false,
  startJob: () => {},
  selectJob: () => {},
  clearJob: () => {},
  clearAll: () => {},
});

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

const MAX_JOBS = 20;

export function JobFeedProvider({ children }: { children: React.ReactNode }) {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);
  // Tracks the ID of the job that is currently waiting for backend events.
  const activeIdRef = useRef<string | null>(null);

  // startJob is called from RunnerPage BEFORE api.runFlow — pre-creates
  // the job with project name and context.
  const startJob = useCallback(
    (flowName: string, projectName?: string, targetLangs?: string[], fileCount?: number) => {
      const id = `${flowName}-${Date.now()}`;
      activeIdRef.current = id;
      const job: Job = {
        id,
        flowName,
        projectName,
        targetLangs,
        fileCount,
        status: "running",
        events: [],
        progress: { current: 0, total: 0 },
        stepSnapshots: [],
        startTime: Date.now(),
      };
      setJobs((prev) => [job, ...prev].slice(0, MAX_JOBS));
      setSelectedJobId(id);
    },
    [],
  );

  // Global event listener — always mounted, persists across navigation.
  // All events update the active job (the one created by startJob).
  // If no active job exists and a "running" event arrives (e.g. reconnect),
  // a new job is created.
  useWailsEvent("flow:event", (data) => {
    const e = data as RunEvent;

    setJobs((prev) => {
      const activeId = activeIdRef.current;

      // If we have an active job, route ALL events to it.
      if (activeId) {
        return prev.map((job) => {
          if (job.id !== activeId) return job;
          const events = [...job.events, e];

          switch (e.type) {
            case "progress":
              return {
                ...job,
                events,
                progress: {
                  current: (e.file_index ?? job.progress.current) + 1,
                  total: e.file_count ?? job.progress.total,
                },
              };
            case "complete":
              activeIdRef.current = null;
              return {
                ...job,
                events,
                status: "complete" as const,
                durationMs: e.duration_ms,
                progress: { ...job.progress, current: job.progress.total },
              };
            case "error": {
              activeIdRef.current = null;
              const rawMsg = e.message ?? "Flow execution failed";
              const isCanceled =
                rawMsg.includes("context canceled") || rawMsg.includes("context cancelled");
              return {
                ...job,
                events,
                status: isCanceled ? ("canceled" as const) : ("error" as const),
                error: isCanceled ? "Flow canceled" : rawMsg,
              };
            }
            case "pipeline_metrics":
              return { ...job, events, stepSnapshots: e.steps ?? job.stepSnapshots };
            default:
              return { ...job, events };
          }
        });
      }

      // No active job — if this is a "running" event, create a new job
      // (reconnect scenario: app started while backend was already running).
      if (e.type === "state" && e.message === "running") {
        const id = `${e.flow_id}-${Date.now()}`;
        activeIdRef.current = id;
        const job: Job = {
          id,
          flowName: e.flow_id,
          status: "running",
          events: [e],
          progress: { current: 0, total: 0 },
          stepSnapshots: [],
          startTime: Date.now(),
        };
        setSelectedJobId(id);
        return [job, ...prev].slice(0, MAX_JOBS);
      }

      // No active job and not a "running" event — ignore (stale event).
      return prev;
    });
  });

  // Safety-net reconciliation, independent of live event delivery. While a
  // job is running we poll the backend run state on a fixed interval; once
  // the run reaches a terminal state the live "flow:event" stream didn't
  // deliver — a dropped or late terminal event, or a stream that simply went
  // quiet after the last progress event (progress is emitted *before* the
  // final file finishes, so the UI already reads 100%) — we pull the
  // authoritative event log and settle the job. Polling, rather than
  // re-arming off each incoming event, is what guarantees a finished job
  // can't hang at 100% just because the event stream stopped. Keyed on the
  // running job's id: starts when a run begins, stops the moment the job
  // settles (via either the live path above or this reconciliation).
  const runningJobId = jobs.find((j) => j.status === "running")?.id ?? null;
  useEffect(() => {
    if (!runningJobId) return;
    let stopped = false;

    const reconcile = async () => {
      // Cheap string probe first — avoid pulling the whole event buffer on
      // every tick of a long, genuinely-running flow.
      const state = await api.getRunState();
      if (stopped || (state !== "complete" && state !== "error" && state !== "canceled")) {
        return;
      }

      const events = (await api.getRunEvents()) as RunEvent[] | null;
      if (stopped || !events || events.length === 0) return;

      setJobs((prev) => {
        const job = prev.find((j) => j.id === runningJobId);
        if (!job || job.status !== "running") return prev; // already settled

        // Fold the authoritative log into a fresh snapshot.
        let updated: Job = { ...job, events };
        let terminal = false;
        for (const e of events) {
          if (e.type === "progress") {
            updated = {
              ...updated,
              progress: {
                current: (e.file_index ?? 0) + 1,
                total: e.file_count ?? updated.progress.total,
              },
            };
          } else if (e.type === "complete") {
            terminal = true;
            updated = {
              ...updated,
              status: "complete",
              durationMs: e.duration_ms,
              progress: { ...updated.progress, current: updated.progress.total },
            };
          } else if (e.type === "pipeline_metrics") {
            updated = { ...updated, stepSnapshots: e.steps ?? updated.stepSnapshots };
          } else if (e.type === "error") {
            terminal = true;
            const rawMsg = e.message ?? "Flow execution failed";
            const isCanceled =
              rawMsg.includes("context canceled") || rawMsg.includes("context cancelled");
            updated = {
              ...updated,
              status: isCanceled ? "canceled" : "error",
              error: isCanceled ? "Flow canceled" : rawMsg,
            };
          }
        }

        // Backend reports terminal but the log hasn't recorded a terminal
        // event yet — leave it running and let the next tick retry.
        if (!terminal) return prev;
        if (activeIdRef.current === runningJobId) activeIdRef.current = null;
        return prev.map((j) => (j.id === runningJobId ? updated : j));
      });
    };

    // Probe immediately (catches a flow that finished before this effect
    // mounted) then keep polling until the job settles.
    void reconcile();
    const interval = setInterval(() => void reconcile(), 750);
    return () => {
      stopped = true;
      clearInterval(interval);
    };
  }, [runningJobId]);

  const activeJob = jobs.find((j) => j.status === "running") ?? null;
  const selectedJob = jobs.find((j) => j.id === selectedJobId) ?? null;
  const hasActive = activeJob !== null;

  const selectJob = useCallback((id: string | null) => {
    setSelectedJobId(id);
  }, []);

  const clearJob = useCallback((id: string) => {
    setJobs((prev) => prev.filter((j) => j.id !== id));
    setSelectedJobId((prev) => (prev === id ? null : prev));
  }, []);

  const clearAll = useCallback(() => {
    setJobs((prev) => prev.filter((j) => j.status === "running"));
    setSelectedJobId(null);
  }, []);

  return (
    <JobFeedContext.Provider
      value={{
        jobs,
        activeJob,
        selectedJobId,
        selectedJob,
        hasActive,
        startJob,
        selectJob,
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
