import { useState, useEffect, useCallback } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type {
  AutomationRun,
  AutomationStep,
  AutomationLogEntry,
  RunStatus,
  StepStatus,
} from "../types/api";
import { Card } from "@neokapi/ui-primitives/components/ui/card";
import { Badge } from "@neokapi/ui-primitives/components/ui/badge";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { cn } from "@neokapi/ui-primitives";

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

const runStatusColor: Record<RunStatus, string> = {
  pending: "bg-muted text-muted-foreground",
  running: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  completed: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  failed: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
  partial: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
};

const stepStatusIcon: Record<StepStatus, string> = {
  pending: "\u25CB", // ○
  running: "\u25CF", // ●
  completed: "\u2713", // ✓
  failed: "\u2717", // ✗
  skipped: "\u2014", // —
};

const stepStatusColor: Record<StepStatus, string> = {
  pending: "text-muted-foreground",
  running: "text-blue-500 animate-pulse",
  completed: "text-green-500",
  failed: "text-red-500",
  skipped: "text-muted-foreground",
};

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function duration(startedAt: string, endedAt?: string): string {
  const start = new Date(startedAt).getTime();
  const end = endedAt ? new Date(endedAt).getTime() : Date.now();
  const ms = end - start;
  if (ms < 1000) return `${ms}ms`;
  const secs = Math.floor(ms / 1000);
  if (secs < 60) return `${secs}s`;
  return `${Math.floor(secs / 60)}m ${secs % 60}s`;
}

function triggerLabel(triggerType: string): string {
  const labels: Record<string, string> = {
    "connector.push.completed": "Content pushed",
    "push.automations.completed": "Automations completed",
    "source.review.completed": "Source review completed",
    "project.updated": "Project updated",
  };
  return labels[triggerType] || triggerType;
}

function actionLabel(actionType: string): string {
  const labels: Record<string, string> = {
    auto_translate: "AI Translation",
    auto_extract: "Entity Extraction",
    create_review_tasks: "Create Review Tasks",
    create_source_review: "Source Review",
    notify: "Notification",
    auto_translate_new_locale: "Translate New Locale",
  };
  return labels[actionType] || actionType;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface AutomationRunsPageProps {
  projectId: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AutomationRunsPage({ projectId }: AutomationRunsPageProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [runs, setRuns] = useState<AutomationRun[]>([]);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [steps, setSteps] = useState<AutomationStep[]>([]);
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);
  const [logs, setLogs] = useState<AutomationLogEntry[]>([]);
  const [loading, setLoading] = useState(true);

  // Fetch runs.
  const loadRuns = useCallback(async () => {
    if (!ws || !projectId) return;
    try {
      const result = await api.listAutomationRuns(ws, projectId, undefined, 20);
      setRuns(result);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [api, ws, projectId]);

  useEffect(() => {
    void loadRuns();
    // Poll every 5s for live updates.
    const interval = setInterval(loadRuns, 5000);
    return () => clearInterval(interval);
  }, [loadRuns]);

  // Fetch run detail when selected.
  useEffect(() => {
    if (!selectedRunId || !ws) return;
    let cancelled = false;
    const load = async () => {
      try {
        const { steps: s } = await api.getAutomationRun(ws, projectId, selectedRunId);
        if (!cancelled) setSteps(s);
      } catch {
        /* ignore */
      }
    };
    void load();
    const interval = setInterval(load, 3000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [api, ws, projectId, selectedRunId]);

  // Fetch logs when step selected.
  useEffect(() => {
    if (!selectedStepId || !selectedRunId || !ws) return;
    let cancelled = false;
    const load = async () => {
      try {
        const result = await api.listStepLogs(ws, projectId, selectedRunId, selectedStepId);
        if (!cancelled) setLogs(result);
      } catch {
        /* ignore */
      }
    };
    void load();
    const interval = setInterval(load, 3000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [api, ws, projectId, selectedRunId, selectedStepId]);

  if (loading) {
    return <div className="text-sm text-muted-foreground p-4">Loading automation runs...</div>;
  }

  if (runs.length === 0) {
    return (
      <div className="text-sm text-muted-foreground p-4">
        No automation runs yet. Push content to trigger automations.
      </div>
    );
  }

  const selectedRun = runs.find((r) => r.id === selectedRunId);

  return (
    <div className="flex gap-4 h-full min-h-0">
      {/* Run list */}
      <div className="w-[320px] shrink-0 overflow-y-auto space-y-2">
        <h3 className="text-sm font-semibold mb-2">Automation Runs</h3>
        {runs.map((run) => (
          <Card
            key={run.id}
            className={cn(
              "p-3 cursor-pointer transition-colors hover:bg-accent/50",
              selectedRunId === run.id && "ring-1 ring-primary bg-accent/30",
            )}
            onClick={() => {
              setSelectedRunId(run.id);
              setSelectedStepId(null);
              setLogs([]);
            }}
          >
            <div className="flex items-center justify-between mb-1">
              <Badge className={cn("text-[10px]", runStatusColor[run.status])}>{run.status}</Badge>
              <span className="text-[10px] text-muted-foreground">{timeAgo(run.started_at)}</span>
            </div>
            <p className="text-xs font-medium truncate">{triggerLabel(run.trigger_type)}</p>
            <p className="text-[10px] text-muted-foreground mt-0.5">
              {run.done_count}/{run.step_count} steps
              {run.trigger_data?.items && ` \u00B7 ${run.trigger_data.items}`}
            </p>
            {run.status !== "pending" && (
              <p className="text-[10px] text-muted-foreground">
                {duration(run.started_at, run.ended_at)}
              </p>
            )}
          </Card>
        ))}
      </div>

      {/* Run detail + steps */}
      <div className="flex-1 min-w-0 overflow-y-auto">
        {selectedRun ? (
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-semibold">{triggerLabel(selectedRun.trigger_type)}</h3>
                <p className="text-[11px] text-muted-foreground">
                  Run {selectedRun.id.slice(0, 8)} &middot;{" "}
                  {duration(selectedRun.started_at, selectedRun.ended_at)}
                </p>
              </div>
              {selectedRun.status === "running" && (
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={async () => {
                    await api.cancelAutomationRun(ws, projectId, selectedRun.id);
                    void loadRuns();
                  }}
                >
                  Cancel
                </Button>
              )}
            </div>

            {/* Steps */}
            <div className="space-y-2">
              {steps.map((step) => (
                <Card
                  key={step.id}
                  className={cn(
                    "p-3 cursor-pointer transition-colors hover:bg-accent/50",
                    selectedStepId === step.id && "ring-1 ring-primary",
                  )}
                  onClick={() => setSelectedStepId(step.id)}
                >
                  <div className="flex items-center gap-2">
                    <span className={cn("text-base", stepStatusColor[step.status])}>
                      {stepStatusIcon[step.status]}
                    </span>
                    <div className="flex-1 min-w-0">
                      <p className="text-xs font-medium">{actionLabel(step.action_type)}</p>
                      <p className="text-[10px] text-muted-foreground">{step.rule_name}</p>
                    </div>
                    <div className="text-right">
                      {step.total_jobs > 0 && (
                        <p className="text-[10px] text-muted-foreground">
                          {step.done_jobs}/{step.total_jobs} jobs
                        </p>
                      )}
                      {step.task_ids && step.task_ids.length > 0 && (
                        <p className="text-[10px] text-muted-foreground">
                          {step.task_ids.length} tasks
                        </p>
                      )}
                      <p className="text-[10px] text-muted-foreground">
                        {duration(step.started_at, step.ended_at)}
                      </p>
                    </div>
                  </div>
                  {step.total_jobs > 0 && step.status === "running" && (
                    <div className="mt-2 h-1.5 bg-muted rounded-full overflow-hidden">
                      <div
                        className="h-full bg-blue-500 rounded-full transition-all"
                        style={{
                          width: `${Math.round((step.done_jobs / step.total_jobs) * 100)}%`,
                        }}
                      />
                    </div>
                  )}
                  {step.error && <p className="text-[10px] text-destructive mt-1">{step.error}</p>}
                </Card>
              ))}
            </div>

            {/* Logs */}
            {selectedStepId && logs.length > 0 && (
              <div className="mt-4">
                <h4 className="text-xs font-semibold mb-2">Logs</h4>
                <div className="bg-muted/50 rounded-md p-2 max-h-[300px] overflow-y-auto font-mono text-[11px] space-y-0.5">
                  {logs.map((log) => (
                    <div key={log.id} className="flex gap-2">
                      <span className="text-muted-foreground shrink-0 w-[60px]">
                        {new Date(log.timestamp).toLocaleTimeString()}
                      </span>
                      <span
                        className={cn(
                          "shrink-0 w-[40px]",
                          log.level === "error" && "text-red-500",
                          log.level === "warn" && "text-yellow-500",
                          log.level === "info" && "text-muted-foreground",
                        )}
                      >
                        {log.level}
                      </span>
                      <span className="break-all">{log.message}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        ) : (
          <div className="text-sm text-muted-foreground p-4">Select a run to view details.</div>
        )}
      </div>
    </div>
  );
}
