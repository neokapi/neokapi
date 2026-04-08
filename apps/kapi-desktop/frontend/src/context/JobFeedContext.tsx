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
  hasActive: boolean;
  clearJob: (id: string) => void;
  clearAll: () => void;
}

const JobFeedContext = createContext<JobFeedContextValue>({
  jobs: [],
  activeJob: null,
  hasActive: false,
  clearJob: () => {},
  clearAll: () => {},
});

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

const MAX_JOBS = 20;

export function JobFeedProvider({ children }: { children: React.ReactNode }) {
  const [jobs, setJobs] = useState<Job[]>([]);
  const activeIdRef = useRef<string | null>(null);

  // Global event listener — always mounted, persists across navigation.
  useWailsEvent("flow:event", (data) => {
    const e = data as RunEvent;

    setJobs((prev) => {
      // "state" with "running" → start a new job.
      if (e.type === "state" && e.message === "running") {
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
          case "error":
            activeIdRef.current = null;
            return {
              ...job,
              events,
              status: "error" as const,
              error: e.message ?? "Flow execution failed",
            };
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
        // A flow is running but we have no job for it — create a placeholder.
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
      }
    })();
  }, []);

  const activeJob = jobs.find((j) => j.status === "running") ?? null;
  const hasActive = activeJob !== null;

  const clearJob = useCallback((id: string) => {
    setJobs((prev) => prev.filter((j) => j.id !== id));
  }, []);

  const clearAll = useCallback(() => {
    setJobs((prev) => prev.filter((j) => j.status === "running"));
  }, []);

  return (
    <JobFeedContext.Provider value={{ jobs, activeJob, hasActive, clearJob, clearAll }}>
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
