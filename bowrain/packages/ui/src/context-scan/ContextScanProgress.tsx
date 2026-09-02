import { Button, Card, Progress } from "@neokapi/ui-primitives";
import type { ContextScanJob } from "../types/api";
import { TEST_IDS } from "../test-ids";
import { AlertTriangle, Check, Loader2 } from "../components/icons";

export interface ContextScanProgressProps {
  job: ContextScanJob;
  /** Shown on failure; typically re-enqueues the same request. */
  onRetry?: () => void;
}

/** Worker milestones, in order. Progress values match the job pipeline. */
const STEPS: { at: number; label: string }[] = [
  { at: 10, label: "Reading your sources" },
  { at: 40, label: "Assembling the corpus" },
  { at: 60, label: "Drafting your voice profile" },
  { at: 80, label: "Extracting candidate terms" },
  { at: 100, label: "Preparing the review" },
];

/**
 * Progress view for a running context scan: a bar plus the worker's milestone
 * steps. Failures render the job error with an optional retry action.
 */
export function ContextScanProgress({ job, onRetry }: ContextScanProgressProps) {
  if (job.status === "failed") {
    return (
      <Card data-testid={TEST_IDS.contextScan.progress} className="p-6 space-y-4">
        <div className="flex items-center gap-2">
          <AlertTriangle className="w-4 h-4 text-destructive" />
          <h2 className="text-sm font-semibold">The scan could not complete</h2>
        </div>
        <p className="text-sm text-muted-foreground">
          {job.error || "The scan failed before a draft could be produced."}
        </p>
        {onRetry && (
          <Button variant="outline" onClick={onRetry}>
            Try again
          </Button>
        )}
      </Card>
    );
  }

  const progress = Math.max(0, Math.min(100, job.progress));
  // The step whose milestone the progress has not yet passed is active;
  // everything before it is done. At 100 every step is done.
  const activeIndex = progress >= 100 ? STEPS.length : STEPS.findIndex((s) => progress <= s.at);

  return (
    <Card data-testid={TEST_IDS.contextScan.progress} className="p-6 space-y-5">
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">Scanning your brand</h2>
          <span className="text-xs tabular-nums text-muted-foreground">{progress}%</span>
        </div>
        <Progress value={progress} />
      </div>

      <ul className="space-y-2.5">
        {STEPS.map((step, i) => {
          const done = i < activeIndex;
          const active = i === activeIndex;
          return (
            <li key={step.at} className="flex items-center gap-2.5 text-sm">
              <span
                className={`flex h-5 w-5 items-center justify-center rounded-full shrink-0 ${
                  done
                    ? "bg-primary/15 text-primary"
                    : active
                      ? "bg-muted text-foreground"
                      : "bg-muted/50 text-muted-foreground/50"
                }`}
              >
                {done ? (
                  <Check className="w-3 h-3" />
                ) : active ? (
                  <Loader2 className="w-3 h-3 animate-spin" />
                ) : (
                  <span className="w-1.5 h-1.5 rounded-full bg-current" />
                )}
              </span>
              <span
                className={
                  done
                    ? "text-muted-foreground"
                    : active
                      ? "text-foreground"
                      : "text-muted-foreground/60"
                }
              >
                {step.label}
              </span>
            </li>
          );
        })}
      </ul>

      <p className="text-xs text-muted-foreground">
        This usually takes under a minute. The draft opens for review when it is ready. The scan
        never writes a profile on its own.
      </p>
    </Card>
  );
}
