import { createContext, useContext, useState, useCallback, useRef, useEffect } from "react";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { api } from "../hooks/useApi";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type JobStatus = "running" | "complete" | "error" | "canceled";

export interface RunEvent {
  type: "state" | "progress" | "trace" | "error" | "complete";
  flow_id: string;
  message?: string;
  file_index?: number;
  file_count?: number;
  file_path?: string;
  duration_ms?: number;
  files_processed?: number;
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
  const activeIdRef = useRef<string | null>(null);
  const pendingJobRef = useRef<Job | null>(null);

  // startJob is called from RunnerPage BEFORE api.runFlow — pre-creates
  // the job with project name and context so we don't show "Running flow".
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
        startTime: Date.now(),
      };
      pendingJobRef.current = job;
      setJobs((prev) => [job, ...prev].slice(0, MAX_JOBS));
      setSelectedJobId(id);
    },
    [],
  );

  // Global event listener — always mounted, persists across navigation.
  useWailsEvent("flow:event", (data) => {
    const e = data as RunEvent;

    setJobs((prev) => {
      // "state" with "running" → if we have a pending job, just append the event.
      // Otherwise create a new job (reconnection or unexpected start).
      if (e.type === "state" && e.message === "running") {
        if (pendingJobRef.current) {
          const pending = pendingJobRef.current;
          pendingJobRef.current = null;
          return prev.map((j) => (j.id === pending.id ? { ...j, events: [...j.events, e] } : j));
        }
        // No pending job — create one from the event.
        const id = `${e.flow_id}-${Date.now()}`;
        activeIdRef.current = id;
        const job: Job = {
          id,
          flowName: e.flow_id,
          status: "running",
          events: [e],
          progress: { current: 0, total: 0 },
          startTime: Date.now(),
        };
        setSelectedJobId(id);
        return [job, ...prev].slice(0, MAX_JOBS);
      }

      // All other events update the active job.
      const activeId = activeIdRef.current;
      if (!activeId) return prev;

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
          default:
            return { ...job, events };
        }
      });
    });
  });

  // On mount, check if a flow is already running (reconnect scenario).
  useEffect(() => {
    void (async () => {
      const state = await api.getRunState();
      if (state === "running" && activeIdRef.current === null) {
        const id = `reconnected-${Date.now()}`;
        activeIdRef.current = id;
        setJobs((prev) => [
          {
            id,
            flowName: "Running flow",
            status: "running",
            events: [],
            progress: { current: 0, total: 0 },
            startTime: Date.now(),
          },
          ...prev,
        ]);
        setSelectedJobId(id);
      }
    })();
  }, []);

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
