// WalkthroughCard — the guided-lesson transport for a scenario's walkthrough.
// It shows one step's prose and Back / Next (or Run, when the step's primary
// action is running the flow); advancing a step applies its editor focus, so
// the card points INTO the workspace — selecting the node, opening the panel —
// instead of describing it from the outside.

import React from "react";
import { ArrowLeft, ArrowRight, Play, GraduationCap } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import type { LessonStep } from "./labScenarios";

export interface WalkthroughCardProps {
  steps: LessonStep[];
  /** Active step index. */
  index: number;
  onIndexChange: (index: number) => void;
  /** Run the flow (offered when the active step has `run: true`). */
  onRun: () => void;
  runDisabled?: boolean;
}

export default function WalkthroughCard({
  steps,
  index,
  onIndexChange,
  onRun,
  runDisabled,
}: WalkthroughCardProps): React.ReactElement | null {
  const step = steps[index];
  if (!step) return null;
  const last = index === steps.length - 1;

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-l-4 border-border border-l-primary bg-card px-3 py-2.5">
      <div className="flex items-center justify-between">
        <span className="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.12em] text-primary">
          <GraduationCap size={12} />
          Walkthrough
        </span>
        <span className="font-mono text-[10px] text-muted-foreground">
          {index + 1} / {steps.length}
        </span>
      </div>

      <p className="text-sm leading-relaxed text-foreground">{step.prose}</p>

      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          className="h-7 px-2 text-xs"
          disabled={index === 0}
          onClick={() => onIndexChange(index - 1)}
        >
          <ArrowLeft size={12} className="mr-1" />
          Back
        </Button>
        {step.run ? (
          <Button size="sm" className="h-7 px-3 text-xs" disabled={runDisabled} onClick={onRun}>
            <Play size={12} className="mr-1" />
            Run the flow
          </Button>
        ) : (
          !last && (
            <Button size="sm" className="h-7 px-3 text-xs" onClick={() => onIndexChange(index + 1)}>
              Next
              <ArrowRight size={12} className="ml-1" />
            </Button>
          )
        )}
        {last && !step.run && (
          <span className="text-[11px] italic text-muted-foreground">
            End of the walkthrough — keep exploring, or pick another scenario.
          </span>
        )}
      </div>
    </div>
  );
}
