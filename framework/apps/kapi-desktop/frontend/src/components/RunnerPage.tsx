import { useState, useCallback } from "react";
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
import type { FlowSpec } from "../types/api";
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

interface RunnerPageProps {
  flowName: string;
  flow: FlowSpec;
  onClose: () => void;
}

export function RunnerPage({ flowName, flow, onClose }: RunnerPageProps) {
  const [state, setState] = useState<RunState>("idle");
  const [events, setEvents] = useState<RunEvent[]>([]);
  const [inputFiles] = useState<string[]>([]);
  const [targetLang, setTargetLang] = useState("");
  const [progress, setProgress] = useState({ current: 0, total: 0 });
  const [error, setError] = useState<string | null>(null);
  // Listen for flow events from the backend.
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

  const handleRun = useCallback(async () => {
    if (!targetLang || inputFiles.length === 0) return;
    setState("running");
    setEvents([]);
    setError(null);
    setProgress({ current: 0, total: inputFiles.length });

    try {
      await api.runFlow(flowName, inputFiles, targetLang);
      // Completion is signaled via events, not the return value.
    } catch (e) {
      setState("error");
      setError(String(e));
    }
  }, [flowName, targetLang, inputFiles]);

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
            <Button variant="outline" size="sm" onClick={onClose} aria-label="Back to flows">
              Back to Flows
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

      {/* Configuration (only in idle state) */}
      {state === "idle" && (
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
        {state === "idle" && (
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
            <ScrollArea className="max-h-64">
              <div>
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
              </div>
            </ScrollArea>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
