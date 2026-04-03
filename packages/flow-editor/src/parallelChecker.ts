// Parallel checker — analyzes sequential flows and suggests tools that could
// run in parallel. Tools are parallelizable if they:
// 1. Are adjacent in the pipeline
// 2. Don't have data dependencies (both read blocks, neither requires the other's output)
// 3. Are from independent categories (e.g., "validate" + "enrich" can run together)

import type { FlowSpec, FlowStep, ToolInfo } from "./types";

export interface ParallelSuggestion {
  /** Indices of the steps that could be parallelized (in the flat steps array). */
  stepIndices: number[];
  /** Tool names in the suggested parallel group. */
  toolNames: string[];
  /** Human-readable explanation. */
  reason: string;
}

// Categories that are known to be "read-only" on blocks (don't modify the block content).
// Tools in these categories can safely run in parallel with each other.
const READ_ONLY_CATEGORIES = new Set(["validate", "enrich"]);

// Categories whose tools modify blocks and thus create ordering dependencies.
const _MUTATING_CATEGORIES = new Set(["translate", "transform", "convert"]);

/**
 * Analyze a sequential flow and suggest groups of adjacent steps that
 * could safely run in parallel.
 *
 * Rules:
 * - Two adjacent non-mutating tools (validate, enrich) can run in parallel.
 * - A mutating tool (translate, transform) cannot be parallelized with others.
 * - Tools that are already in a parallel group are skipped.
 * - Only suggests groups of 2+ tools.
 */
export function suggestParallelGroups(
  spec: FlowSpec,
  toolMap: Map<string, ToolInfo>,
): ParallelSuggestion[] {
  const suggestions: ParallelSuggestion[] = [];
  const steps = spec.steps;

  // Find runs of adjacent non-mutating sequential tools.
  let runStart = -1;

  for (let i = 0; i < steps.length; i++) {
    const step = steps[i];

    // Skip parallel groups.
    if (step.parallel && step.parallel.length > 0) {
      flushRun(steps, runStart, i - 1, toolMap, suggestions);
      runStart = -1;
      continue;
    }

    const info = toolMap.get(step.tool);
    const category = info?.category || "pipeline";

    if (canParallelize(category)) {
      if (runStart === -1) runStart = i;
    } else {
      flushRun(steps, runStart, i - 1, toolMap, suggestions);
      runStart = -1;
    }
  }

  // Flush any trailing run.
  flushRun(steps, runStart, steps.length - 1, toolMap, suggestions);

  return suggestions;
}

function canParallelize(category: string): boolean {
  return READ_ONLY_CATEGORIES.has(category) || category === "pipeline";
}

function flushRun(
  steps: FlowStep[],
  start: number,
  end: number,
  toolMap: Map<string, ToolInfo>,
  out: ParallelSuggestion[],
) {
  if (start === -1 || end < start || end - start < 1) return;

  const indices: number[] = [];
  const names: string[] = [];
  const categories: string[] = [];

  for (let i = start; i <= end; i++) {
    indices.push(i);
    names.push(steps[i].tool);
    const info = toolMap.get(steps[i].tool);
    categories.push(info?.category || "pipeline");
  }

  const catLabels = [...new Set(categories)].join(" + ");
  out.push({
    stepIndices: indices,
    toolNames: names,
    reason: `These ${names.length} ${catLabels} tools don't modify content and can run simultaneously for faster processing.`,
  });
}

/**
 * Check whether a specific tool category is considered non-mutating
 * (safe to run in parallel with other non-mutating tools).
 */
export function isCategoryParallelizable(category: string): boolean {
  return canParallelize(category);
}
