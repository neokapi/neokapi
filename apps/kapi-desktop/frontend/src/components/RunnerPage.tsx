import { useState, useCallback, useEffect, useRef } from "react";
import { Play, Square, CheckCircle2, XCircle, Loader2, FileText } from "lucide-react";
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
import { useWailsEvent } from "../hooks/useWailsEvent";

type RunState = "idle" | "running" | "complete" | "error" | "canceled";

interface RunEvent {
  type: "state" | "progress" | "trace" | "error" | "complete";
  flow_id: string;
  message?: string;
  file_index?: number;
  file_count?: number;
  file_path?: string;
  duration_ms?: number;
  files_processed?: number;
}

/**
 * Returns a promise that resolves when the backend emits a flow:event with
 * type "complete" or rejects on "error". Used to serialize multi-language
 * runs so the next language waits for the previous one to finish.
 */
function waitForFlowComplete(): Promise<void> {
  return new Promise((resolve, reject) => {
    let cleanup: (() => void) | null = null;
    import("@wailsio/runtime")
      .then((mod) => {
        cleanup = mod.Events.On("flow:event", (e: { data: unknown }) => {
          const event = e.data as { type: string; message?: string };
          if (event.type === "complete") {
            cleanup?.();
            resolve();
          } else if (event.type === "error") {
            cleanup?.();
            reject(new Error(event.message ?? "Flow execution failed"));
          }
        });
      })
      .catch(() => {
        // No Wails runtime (Storybook/test) — resolve immediately.
        resolve();
      });
  });
}

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
  const [state, setState] = useState<RunState>("idle");
  const [events, setEvents] = useState<RunEvent[]>([]);
  const [inputFiles, setInputFiles] = useState<string[]>([]);
  const [targetLang, setTargetLang] = useState("");
  const [progress, setProgress] = useState({ current: 0, total: 0 });
  const [error, setError] = useState<string | null>(null);
  const [currentTarget, setCurrentTarget] = useState("");
  const autoRunStarted = useRef(false);

  useWailsEvent("flow:event", (data) => {
    const e = data as RunEvent;
    setEvents((prev) => [...prev, e]);

    switch (e.type) {
      case "progress":
        setProgress({
          current: (e.file_index ?? 0) + 1,
          total: e.file_count ?? 0,
        });
        break;
      case "complete":
        setState("complete");
        setProgress((prev) => ({ ...prev, current: prev.total }));
        break;
      case "error":
        setState("error");
        setError(e.message ?? "Flow execution failed");
        break;
    }
  });

  // Auto-run: resolve content and execute for all target languages.
  useEffect(() => {
    if (!autoRun || !project || autoRunStarted.current) return;
    autoRunStarted.current = true;

    const targets = project.defaults?.target_languages ?? [];
    if (targets.length === 0) {
      setError("No target languages configured in project.");
      setState("error");
      return;
    }

    (async () => {
      const matches = await api.matchContent(tabID);
      const paths = matches?.map((m) => m.path) ?? [];
      if (paths.length === 0) {
        setError("No content files matched the project patterns.");
        setState("error");
        return;
      }

      setInputFiles(paths);
      setState("running");
      setProgress({ current: 0, total: paths.length * targets.length });

      let filesDone = 0;
      for (const lang of targets) {
        setCurrentTarget(lang);
        setEvents((prev) => [
          ...prev,
          {
            type: "state",
            flow_id: flowName,
            message: `Running for ${lang} (${paths.length} files)...`,
          },
        ]);

        try {
          // Start the flow — returns immediately (backend runs in goroutine).
          await api.runFlow(tabID, flowName, paths, lang);

          // Wait for the backend to signal completion or error via Wails event.
          await waitForFlowComplete();
        } catch (e) {
          setState("error");
          setError(`Failed for ${lang}: ${String(e)}`);
          return;
        }
        filesDone += paths.length;
        setProgress({ current: filesDone, total: paths.length * targets.length });
      }

      setState("complete");
      setEvents((prev) => [
        ...prev,
        {
          type: "complete",
          flow_id: flowName,
          message: `Completed for ${targets.length} language${targets.length > 1 ? "s" : ""}: ${targets.join(", ")}`,
        },
      ]);
    })();
  }, [autoRun, project, tabID, flowName]);

  // Manual run (single language).
  const handleRun = useCallback(async () => {
    if (!targetLang || inputFiles.length === 0) return;
    setState("running");
    setEvents([]);
    setError(null);
    setProgress({ current: 0, total: inputFiles.length });

    try {
      await api.runFlow(tabID, flowName, inputFiles, targetLang);
    } catch (e) {
      setState("error");
      setError(String(e));
    }
  }, [tabID, flowName, targetLang, inputFiles]);

  const handleCancel = useCallback(async () => {
    try {
      await api.cancelRun();
      setState("canceled");
    } catch (e) {
      setError(String(e));
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
            {currentTarget && state === "running" && (
              <Badge variant="secondary" className="text-xs">
                {currentTarget}
              </Badge>
            )}
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
            disabled={!targetLang || inputFiles.length === 0}
            aria-label="Run flow"
          >
            <Play size={14} />
            Run Flow
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

      {/* Error */}
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
