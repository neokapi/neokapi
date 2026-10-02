// Flow steps run in order, one after another.
//
// Every host runs a flow as an ordered tool chain, and the runtime refuses a
// flow whose step holds a `parallel:` list (core/flow.CheckSequential). The
// editors create no such step. A flow loaded with one is shown as invalid with
// the refusal the runtime gives, and the editor offers to list the group's
// tools as ordered steps, which is the fix that refusal names.

import { t } from "@neokapi/i18n-react/runtime";

/** The fields of a step the check reads: any host's step type carries them. */
export interface SequenceStep {
  tool: string;
  label?: string;
  parallel?: SequenceStep[];
}

/** A step the runtime refuses, and the refusal it gives. */
export interface SequenceIssue {
  /** The step's position in the flow. */
  index: number;
  message: string;
}

/**
 * The runtime's refusal for the step at `index` when it holds a `parallel:`
 * list, or null when the step runs. Each branch is named by its tool, else its
 * label, as core/flow.CheckSequential names them.
 */
export function parallelStepError(step: SequenceStep, index: number): string | null {
  const branches = step.parallel ?? [];
  if (branches.length === 0) return null;
  const tools = branches.map((b) => b.tool || b.label || t("a step with no tool")).join(", ");
  return t(
    "step[{index}] holds a parallel: list ({tools}), and flow steps run in order, one after another: list those tools as ordered steps",
    { index, tools },
  );
}

/** Every step the runtime refuses, in flow order. The runtime reports the first. */
export function sequenceIssues(steps: readonly SequenceStep[]): SequenceIssue[] {
  const out: SequenceIssue[] = [];
  steps.forEach((step, index) => {
    const message = parallelStepError(step, index);
    if (message) out.push({ index, message });
  });
  return out;
}

/**
 * The steps with each `parallel:` group replaced by its branches as ordered
 * steps, in the order the group listed them. With `index`, only that step is
 * replaced. Each branch keeps its own options and label; the group step's own
 * label is dropped, and an empty group leaves no step.
 */
export function listBranchesInOrder<S extends SequenceStep>(
  steps: readonly S[],
  index?: number,
): S[] {
  const out: S[] = [];
  steps.forEach((step, i) => {
    if (step.parallel !== undefined && (index === undefined || index === i)) {
      out.push(...(step.parallel as S[]));
      return;
    }
    out.push(step);
  });
  return out;
}
