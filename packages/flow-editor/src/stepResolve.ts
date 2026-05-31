// Node-identity-based resolution of the selected flow step.
//
// React Flow nodes built by `stepsToGraph` carry the step's *position* in the
// FlowSpec on `node.data` — `stIndex` for a source-transform, or `stepIndex`
// (+ optional `branchIndex` for a parallel branch) for a main step. Resolving
// selection/edit/delete by these indices instead of by tool name is what makes
// duplicate-tool nodes addressable: selecting the 2nd `ai-translate` node edits
// the 2nd step, and deleting it removes only that step (or branch), never every
// step that happens to use the same tool.

import type { FlowSpec, FlowStep } from "./types";

/** Resolves where a selected node lives in a FlowSpec, by identity. */
export interface StepLocation {
  /** True when the node belongs to the source-transform stage. */
  isSourceTransform: boolean;
  /** Index into `spec.sourceTransforms` (when isSourceTransform) or `spec.steps`. */
  index: number;
  /** Index into the parent step's `parallel` array, when the node is a branch. */
  branchIndex?: number;
}

/** The relevant fields a tool node carries on `node.data` for resolution. */
export interface NodeStepData {
  toolName?: unknown;
  stage?: unknown;
  stIndex?: unknown;
  stepIndex?: unknown;
  branchIndex?: unknown;
}

function asIndex(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : undefined;
}

/**
 * Resolve the FlowSpec location of a node from its `data`.
 *
 * Returns null when the node carries no resolvable index (e.g. a reader/writer
 * node, or stale data). Operate on the returned `index`/`branchIndex` — never
 * on tool name — so duplicate tools and parallel branches stay distinct.
 */
export function resolveStepLocation(data: NodeStepData | null | undefined): StepLocation | null {
  if (!data) return null;

  if (data.stage === "source-transform") {
    const index = asIndex(data.stIndex);
    if (index === undefined) return null;
    return { isSourceTransform: true, index };
  }

  const index = asIndex(data.stepIndex);
  if (index === undefined) return null;
  const branchIndex = asIndex(data.branchIndex);
  return branchIndex === undefined
    ? { isSourceTransform: false, index }
    : { isSourceTransform: false, index, branchIndex };
}

/** Resolve the actual FlowStep at a location (the branch itself for parallel). */
export function stepAtLocation(spec: FlowSpec, loc: StepLocation): FlowStep | null {
  if (loc.isSourceTransform) {
    return spec.sourceTransforms?.[loc.index] ?? null;
  }
  const step = spec.steps[loc.index];
  if (!step) return null;
  if (loc.branchIndex !== undefined) {
    return step.parallel?.[loc.branchIndex] ?? null;
  }
  return step;
}

/**
 * Return a new FlowSpec with the step at `loc` updated by `mutate`.
 *
 * For a parallel branch the mutation targets `step.parallel[branchIndex]`,
 * never the wrapper step — so config edits to a branch persist to the branch.
 */
export function updateStepAtLocation(
  spec: FlowSpec,
  loc: StepLocation,
  mutate: (step: FlowStep) => FlowStep,
): FlowSpec {
  if (loc.isSourceTransform) {
    return {
      ...spec,
      sourceTransforms: (spec.sourceTransforms ?? []).map((s, i) =>
        i === loc.index ? mutate(s) : s,
      ),
    };
  }
  return {
    ...spec,
    steps: spec.steps.map((s, i) => {
      if (i !== loc.index) return s;
      if (loc.branchIndex !== undefined && s.parallel) {
        return {
          ...s,
          parallel: s.parallel.map((p, b) => (b === loc.branchIndex ? mutate(p) : p)),
        };
      }
      return mutate(s);
    }),
  };
}

/**
 * Return a new FlowSpec with the step at `loc` removed.
 *
 * Removing one branch of a parallel group drops only that branch (collapsing
 * the group to a plain step when a single branch remains, or removing the
 * group entirely when none do); it never drops sibling branches. Removing a
 * plain step or source-transform removes just that entry.
 */
export function removeStepAtLocation(spec: FlowSpec, loc: StepLocation): FlowSpec {
  if (loc.isSourceTransform) {
    const sourceTransforms = (spec.sourceTransforms ?? []).filter((_, i) => i !== loc.index);
    const next: FlowSpec = { ...spec };
    if (sourceTransforms.length > 0) next.sourceTransforms = sourceTransforms;
    else delete next.sourceTransforms;
    return next;
  }

  if (loc.branchIndex !== undefined) {
    const steps = spec.steps.flatMap((s, i) => {
      if (i !== loc.index || !s.parallel) return [s];
      const remaining = s.parallel.filter((_, b) => b !== loc.branchIndex);
      if (remaining.length === 0) return [];
      if (remaining.length === 1) return [remaining[0]];
      return [{ ...s, parallel: remaining }];
    });
    return { ...spec, steps };
  }

  return { ...spec, steps: spec.steps.filter((_, i) => i !== loc.index) };
}
