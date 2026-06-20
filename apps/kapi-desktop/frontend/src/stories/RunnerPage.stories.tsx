import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, useEffect, useCallback } from "react";
import { Play, Square, CheckCircle2, XCircle, Loader2 } from "lucide-react";
import type { FlowSpec } from "../types/api";
import type { StepSnapshot } from "../context/JobFeedContext";
import { Button, Card, ScrollArea } from "@neokapi/ui-primitives";
import { PipelineProgress } from "../components/PipelineProgress";

// Simulated RunnerPage that demonstrates the full execution lifecycle
// without needing a real Wails backend.
function SimulatedRunner({
  flowName,
  flow,
  autoRun = false,
  simulateError = false,
  fileCount = 3,
  stepDurationMs = 800,
}: {
  flowName: string;
  flow: FlowSpec;
  autoRun?: boolean;
  simulateError?: boolean;
  fileCount?: number;
  stepDurationMs?: number;
}) {
  type RunState = "idle" | "running" | "complete" | "error" | "canceled";

  const [state, setState] = useState<RunState>("idle");
  const [progress, setProgress] = useState({ current: 0, total: fileCount });
  const [currentFile, setCurrentFile] = useState("");
  const [stepSnapshots, setStepSnapshots] = useState<StepSnapshot[]>(
    flow.steps.map((s) => ({ name: s.tool, parts_in: 0, parts_out: 0 })),
  );
  const [events, setEvents] = useState<Array<{ type: string; message: string; ts: number }>>([]);
  const [elapsed, setElapsed] = useState(0);

  const addEvent = useCallback((type: string, message: string) => {
    setEvents((prev) => [...prev, { type, message, ts: Date.now() }]);
  }, []);

  const runSimulation = useCallback(async () => {
    setState("running");
    setProgress({ current: 0, total: fileCount });
    setStepSnapshots(flow.steps.map((s) => ({ name: s.tool, parts_in: 0, parts_out: 0 })));
    setEvents([]);
    setElapsed(0);
    const start = Date.now();
    const partsPerFile = 30;

    const files = Array.from(
      { length: fileCount },
      (_, i) => `src/locales/en/messages-${i + 1}.json`,
    );

    addEvent("state", "Flow execution started");

    for (let fileIdx = 0; fileIdx < files.length; fileIdx++) {
      const file = files[fileIdx];
      setCurrentFile(file);
      setProgress({ current: fileIdx, total: files.length });
      addEvent("progress", `Processing ${file}`);

      // Simulate streaming metrics: parts flow through each step.
      for (let stepIdx = 0; stepIdx < flow.steps.length; stepIdx++) {
        addEvent("trace", `  ${flow.steps[stepIdx].tool}: processing ${file}`);

        // Simulate parts flowing in/out of this step.
        setStepSnapshots((prev) =>
          prev.map((s, i) => (i === stepIdx ? { ...s, parts_in: s.parts_in + partsPerFile } : s)),
        );

        await new Promise((r) => setTimeout(r, stepDurationMs));
        setElapsed(Date.now() - start);

        // Mark parts out after processing.
        setStepSnapshots((prev) =>
          prev.map((s, i) => (i === stepIdx ? { ...s, parts_out: s.parts_out + partsPerFile } : s)),
        );

        if (simulateError && fileIdx === 1 && stepIdx === 1) {
          addEvent(
            "error",
            `Error in ${flow.steps[stepIdx].tool}: connection timeout to AI provider`,
          );
          setState("error");
          return;
        }
      }

      setProgress({ current: fileIdx + 1, total: files.length });
      addEvent("complete", `Completed ${file}`);
    }

    setCurrentFile("");
    const duration = ((Date.now() - start) / 1000).toFixed(1);
    addEvent("complete", `Flow completed: ${files.length} files in ${duration}s`);
    setState("complete");
  }, [flow, fileCount, stepDurationMs, simulateError, addEvent]);

  const handleCancel = useCallback(() => {
    setState("canceled");
    addEvent("state", "Flow execution canceled by user");
  }, [addEvent]);

  useEffect(() => {
    if (autoRun) {
      const timer = setTimeout(runSimulation, 500);
      return () => clearTimeout(timer);
    }
  }, [autoRun, runSimulation]);

  const stateIcon = {
    idle: null,
    running: <Loader2 size={16} className="animate-spin text-primary" />,
    complete: <CheckCircle2 size={16} className="text-green-500" />,
    error: <XCircle size={16} className="text-destructive" />,
    canceled: <XCircle size={16} className="text-muted-foreground" />,
  };

  const progressPct =
    progress.total > 0 ? Math.round((progress.current / progress.total) * 100) : 0;

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center gap-3">
        <h2 className="text-lg font-medium">Run: {flowName}</h2>
        {stateIcon[state]}
        {state === "running" && elapsed > 0 && (
          <span className="text-xs text-muted-foreground">{(elapsed / 1000).toFixed(1)}s</span>
        )}
      </div>

      {/* Pipeline with active step highlighting */}
      <Card className="mb-4 p-3">
        <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
          Pipeline
        </h3>
        <PipelineProgress steps={flow.steps} snapshots={stepSnapshots} runState={state} />
      </Card>

      {/* Controls */}
      <div className="mb-4 flex gap-2">
        {state === "idle" && (
          <Button onClick={runSimulation} data-testid="run-button">
            <Play size={14} />
            Run Flow
          </Button>
        )}
        {state === "running" && (
          <Button
            variant="outline"
            onClick={handleCancel}
            className="border-destructive text-destructive hover:bg-destructive/10"
          >
            <Square size={14} />
            Cancel
          </Button>
        )}
        {(state === "complete" || state === "error" || state === "canceled") && (
          <Button
            variant="outline"
            onClick={() => {
              setState("idle");
              setEvents([]);
              setProgress({ current: 0, total: fileCount });
            }}
          >
            Reset
          </Button>
        )}
      </div>

      {/* Progress bar */}
      {(state === "running" || state === "complete") && (
        <div className="mb-4">
          <div className="mb-1 flex justify-between text-xs text-muted-foreground">
            <span>{currentFile || `${progress.current} / ${progress.total} files`}</span>
            <span>{progressPct}%</span>
          </div>
          <div
            className="h-2 overflow-hidden rounded-full bg-accent"
            role="progressbar"
            aria-valuenow={progress.current}
            aria-valuemin={0}
            aria-valuemax={progress.total}
          >
            <div
              className="h-full rounded-full bg-primary transition-all duration-500"
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>
      )}

      {/* Live event log */}
      {events.length > 0 && (
        <Card>
          <h3 className="border-b border-border px-3 py-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Output ({events.length} events)
          </h3>
          <ScrollArea className="max-h-48">
            <div className="p-3 font-mono text-xs">
              {events.map((event, i) => (
                <div
                  key={i}
                  className={`py-0.5 ${
                    event.type === "error"
                      ? "text-destructive"
                      : event.type === "complete"
                        ? "text-green-500"
                        : event.type === "trace"
                          ? "text-muted-foreground"
                          : "text-foreground"
                  }`}
                >
                  {event.message}
                </div>
              ))}
            </div>
          </ScrollArea>
        </Card>
      )}
    </div>
  );
}

const meta: Meta<typeof SimulatedRunner> = {
  title: "Interactions/Flow Execution",
  component: SimulatedRunner,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Demonstrates flow execution with live progress bars, step highlighting, event streaming, error handling, and cancellation.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof SimulatedRunner>;

export const IdleState: Story = {
  args: {
    flowName: "translate-and-qa",
    flow: {
      steps: [{ tool: "translate", config: { provider: "anthropic" } }, { tool: "qa" }],
    },
  },
};

export const AutoRunThreeFiles: Story = {
  name: "Auto-run (3 files, 2 steps)",
  args: {
    flowName: "translate-and-qa",
    flow: {
      steps: [{ tool: "translate" }, { tool: "qa" }],
    },
    autoRun: true,
    fileCount: 3,
    stepDurationMs: 600,
  },
};

export const LongPipeline: Story = {
  name: "Long pipeline (5 files, 4 steps)",
  args: {
    flowName: "full-pipeline",
    flow: {
      steps: [{ tool: "tm-leverage" }, { tool: "translate" }, { tool: "qa" }, { tool: "qa" }],
    },
    autoRun: true,
    fileCount: 5,
    stepDurationMs: 400,
  },
};

export const ErrorDuringExecution: Story = {
  name: "Error during execution",
  args: {
    flowName: "translate-and-qa",
    flow: {
      steps: [{ tool: "translate" }, { tool: "qa" }],
    },
    autoRun: true,
    simulateError: true,
    fileCount: 3,
    stepDurationMs: 500,
  },
};

export const SingleFileQuick: Story = {
  name: "Single file (fast)",
  args: {
    flowName: "pseudo",
    flow: {
      steps: [{ tool: "pseudo-translate" }],
    },
    autoRun: true,
    fileCount: 1,
    stepDurationMs: 300,
  },
};
