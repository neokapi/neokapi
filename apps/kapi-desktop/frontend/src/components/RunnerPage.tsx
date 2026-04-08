import { useState, useCallback, useEffect, useRef } from "react";
import { Play, Square, CheckCircle2, XCircle, Loader2, FileText, AlertTriangle } from "lucide-react";
import {
  Button,
  Badge,
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  Label,
  Input,
  ScrollArea,
  PageHeader,
} from "@neokapi/ui-primitives";
import type { FlowSpec, KapiProject } from "../types/api";
import { api } from "../hooks/useApi";
import { useJobFeed, type RunEvent } from "../context/JobFeedContext";

type RunState = "idle" | "running" | "complete" | "error" | "canceled";

export { type RunEvent };

export interface RunnerPageProps {
  tabID: string;
  flowName: string;
  flow: FlowSpec;
  onClose: () => void;
  /** When set, the project is used to resolve content and target languages. */
  project?: KapiProject;
  /** When true, automatically resolve content and run for all target languages on mount. */
  autoRun?: boolean;
}

export function RunnerPage({ tabID, flowName, flow, onClose, project, autoRun }: RunnerPageProps) {
  const { activeJob, selectedJob, jobs, startJob, hasActive } = useJobFeed();

  // Show selected job, or active job for this flow, or most recent matching.
  const job =
    selectedJob ??
    (activeJob?.flowName === flowName ? activeJob : null) ??
    jobs.find((j) => j.flowName === flowName) ??
    activeJob;

  // Derive state from job feed.
  const state: RunState = job?.status ?? "idle";
  const events: RunEvent[] = job?.events ?? [];
  const progress = job?.progress ?? { current: 0, total: 0 };
  const error = job?.error ?? null;

  const [inputFiles, setInputFiles] = useState<string[]>([]);
  const [targetLang, setTargetLang] = useState("");
  const autoRunStarted = useRef(false);

  const projectName = project?.name || undefined;

  const [launchError, setLaunchError] = useState<string | null>(null);

  // Helper: check if a flow is already running and cancel it before starting.
  const launchFlow = useCallback(
    async (paths: string[], targets: string[]) => {
      setLaunchError(null);

      // If another flow is running, cancel it first.
      const runState = await api.getRunState();
      if (runState === "running") {
        await api.cancelRun();
        // Wait briefly for the goroutine to stop.
        await new Promise((r) => setTimeout(r, 300));
      }

      startJob(flowName, projectName, undefined, paths.length);
      try {
        await api.runFlow(tabID, flowName, paths, targets);
      } catch (err) {
        setLaunchError(String(err));
      }
    },
    [tabID, flowName, projectName, startJob],
  );

  // Auto-run: resolve content and execute for all target languages.
  useEffect(() => {
    if (!autoRun || !project || autoRunStarted.current) return;
    autoRunStarted.current = true;

    const targets = project.defaults?.target_languages ?? [];
    if (targets.length === 0) return;

    (async () => {
      const matches = await api.matchContent(tabID);
      const paths = matches?.map((m) => m.path) ?? [];
      if (paths.length === 0) return;

      setInputFiles(paths);
      await launchFlow(paths, targets);
    })();
  }, [autoRun, project, tabID, launchFlow]);

  // Manual run (single language).
  const handleRun = useCallback(async () => {
    if (!targetLang || inputFiles.length === 0) return;
    await launchFlow(inputFiles, [targetLang]);
  }, [inputFiles, targetLang, launchFlow]);

  const handleCancel = useCallback(async () => {
    try {
      await api.cancelRun();
    } catch {
      // ignore
    }
  }, []);

  const stateIcon = {
    idle: null,
    running: <Loader2 size={16} className="animate-spin text-primary" />,
    complete: <CheckCircle2 size={16} className="text-green-500" />,
    error: <XCircle size={16} className="text-destructive" />,
    canceled: <XCircle size={16} className="text-muted-foreground" />,
  };

  return (
    <div className="p-6">
      <PageHeader
        title={`Run: ${flowName}`}
        className="mb-4"
        actions={
          <div className="flex items-center gap-2">
            {stateIcon[state]}
            <Button variant="outline" size="sm" onClick={onClose} aria-label="Back">
              {state === "complete" || state === "error" ? "Done" : "Back"}
            </Button>
          </div>
        }
      />

      {/* Pipeline preview */}
      <Card className="mb-4">
        <CardHeader className="px-4">
          <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Pipeline
          </CardTitle>
        </CardHeader>
        <CardContent className="px-4">
          <div className="flex items-center gap-2">
            {flow.steps.map((step, i) => (
              <div key={i} className="flex items-center gap-2">
                {i > 0 && <span className="text-muted-foreground">&rarr;</span>}
                <Badge variant="secondary">{step.tool}</Badge>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Manual configuration (only in idle state, non-autoRun) */}
      {state === "idle" && !autoRun && (
        <div className="mb-4 space-y-3">
          <div>
            <Label htmlFor="runner-files" className="mb-1 block">
              Input Files
            </Label>
            <Button
              id="runner-files"
              variant="outline"
              className="flex items-center gap-2 border-dashed text-muted-foreground hover:border-primary hover:text-primary"
              aria-label="Select input files for flow"
            >
              <FileText size={16} />
              {inputFiles.length > 0 ? `${inputFiles.length} file(s) selected` : "Select files..."}
            </Button>
          </div>
          <div>
            <Label htmlFor="runner-target-lang" className="mb-1 block">
              Target Language
            </Label>
            <Input
              id="runner-target-lang"
              type="text"
              value={targetLang}
              onChange={(e) => setTargetLang(e.target.value)}
              placeholder="e.g. fr-FR"
              className="w-48"
            />
          </div>
        </div>
      )}

      {/* Controls */}
      <div className="mb-6 flex gap-2">
        {state === "idle" && !autoRun && (
          <Button
            onClick={handleRun}
            disabled={!targetLang || inputFiles.length === 0 || hasActive}
            aria-label="Run flow"
          >
            {hasActive ? (
              <>
                <Loader2 size={14} className="animate-spin" />
                Running...
              </>
            ) : (
              <>
                <Play size={14} />
                Run Flow
              </>
            )}
          </Button>
        )}
        {state === "running" && (
          <Button
            variant="destructive"
            onClick={handleCancel}
            className="border border-destructive bg-transparent text-destructive hover:bg-destructive/10"
            aria-label="Cancel flow execution"
          >
            <Square size={14} />
            Cancel
          </Button>
        )}
      </div>

      {/* Errors */}
      {launchError && (
        <div className="mb-4 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive" role="alert">
          <AlertTriangle size={14} className="shrink-0" />
          {launchError}
        </div>
      )}
      {error && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          {error}
        </p>
      )}

      {/* Progress bar */}
      {(state === "running" || state === "complete") && progress.total > 0 && (
        <div className="mb-4">
          <div className="mb-1 flex justify-between text-xs text-muted-foreground">
            <span>
              {progress.current} / {progress.total} files
            </span>
            <span>{Math.round((progress.current / progress.total) * 100)}%</span>
          </div>
          <div
            className="h-2 overflow-hidden rounded-full bg-accent"
            role="progressbar"
            aria-valuenow={progress.current}
            aria-valuemin={0}
            aria-valuemax={progress.total}
          >
            <div
              className="h-full rounded-full bg-primary transition-all duration-300"
              style={{
                width: `${(progress.current / progress.total) * 100}%`,
              }}
            />
          </div>
        </div>
      )}

      {/* Event log */}
      {events.length > 0 && (
        <Card>
          <CardHeader className="px-4">
            <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Output
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <div className="h-64">
              <ScrollArea className="h-full">
                {events.map((event, i) => (
                  <div
                    key={i}
                    className={`py-0.5 font-mono text-xs ${
                      event.type === "error"
                        ? "text-destructive"
                        : event.type === "complete"
                          ? "text-green-500"
                          : "text-muted-foreground"
                    }`}
                  >
                    {event.message || event.file_path || event.type}
                  </div>
                ))}
              </ScrollArea>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
