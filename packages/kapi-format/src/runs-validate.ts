/**
 * Run-sequence validation. Checks that paired inline-code runs and
 * standalone placeholder runs in a target translation respect the
 * structural contract of the source — well-formed pair nesting,
 * matching ids, and the deletable / duplicable constraints from the
 * source's `RunConstraints`.
 *
 * Invoked by translation tooling (compile step, AI / TM post-processing,
 * connectors) before a target Run[] is accepted into the runtime
 * dictionary. Diagnostics carry enough context for callers to surface
 * them in CLI output, the visual editor, or CI logs.
 */

import type { PcCloseRun, PcOpenRun, PlaceholderRun, Run } from "./block.ts";

export type DiagnosticKind =
  | "unbalanced-open" // pcOpen with no matching pcClose
  | "unbalanced-close" // pcClose with no matching pcOpen
  | "mismatched-id" // pcOpen and its matching pcClose disagree on id
  | "ill-nested" // close before its open in LIFO order
  | "dropped-pair" // source had paired pair (id), target has neither open nor close
  | "dropped-standalone" // source had standalone (equiv), target has none
  | "duplicated-pair" // target has more than one open with the same id
  | "duplicated-standalone" // target repeats a non-cloneable equiv
  | "empty-paired-inner"; // source's pair had inner content; target's is empty

export interface Diagnostic {
  kind: DiagnosticKind;
  /** Human-readable message; safe to show in CLI / editor diagnostics. */
  message: string;
  /** The pair id or placeholder equiv the diagnostic concerns. */
  ref: string;
}

interface SourceShape {
  pairIds: Set<string>;
  pairInnerNonEmpty: Set<string>;
  standaloneEquivs: Map<string, { duplicable: boolean }>;
}

/**
 * Validate `target` against `source`. The source is the authoritative
 * shape; the target is the candidate translation. Returns an empty
 * array when everything checks out.
 */
export function validatePairedMarkers(source: Run[], target: Run[]): Diagnostic[] {
  const out: Diagnostic[] = [];
  const shape = describeSource(source);

  // Walk the target with a LIFO open stack; check well-formedness as
  // we go and record what we saw.
  const stack: Array<{ id: string; index: number; innerSeen: boolean }> = [];
  const targetPairCounts = new Map<string, number>();
  const targetStandaloneCounts = new Map<string, number>();

  walkRuns(target, (run) => {
    if (isPcOpen(run)) {
      stack.push({ id: run.pcOpen.id, index: stack.length, innerSeen: false });
      targetPairCounts.set(run.pcOpen.id, (targetPairCounts.get(run.pcOpen.id) ?? 0) + 1);
      return;
    }
    if (isPcClose(run)) {
      const top = stack[stack.length - 1];
      if (!top) {
        out.push({
          kind: "unbalanced-close",
          message: `Close marker for id "${run.pcClose.id}" has no matching open.`,
          ref: run.pcClose.id,
        });
        return;
      }
      if (top.id !== run.pcClose.id) {
        // Search downward for a matching open — that means we ill-nested.
        const matchIdx = findOpenIndex(stack, run.pcClose.id);
        if (matchIdx === -1) {
          out.push({
            kind: "unbalanced-close",
            message: `Close marker for id "${run.pcClose.id}" has no matching open.`,
            ref: run.pcClose.id,
          });
          return;
        }
        out.push({
          kind: "ill-nested",
          message: `Close marker for id "${run.pcClose.id}" appears before its open in LIFO order.`,
          ref: run.pcClose.id,
        });
        // Pop the matched frame so subsequent matches stay aligned.
        stack.splice(matchIdx, 1);
        return;
      }
      // Matched. Record whether the pair had any inner content.
      if (top.innerSeen) {
        // Inner content seen — fine.
      } else if (shape.pairInnerNonEmpty.has(run.pcClose.id)) {
        out.push({
          kind: "empty-paired-inner",
          message: `Paired pair "${run.pcClose.id}" lost its inner content in translation.`,
          ref: run.pcClose.id,
        });
      }
      stack.pop();
      // Mark the parent (if any) as having seen inner content.
      const parent = stack[stack.length - 1];
      if (parent) parent.innerSeen = true;
      return;
    }
    if (isPh(run)) {
      const equiv = run.ph.equiv;
      targetStandaloneCounts.set(equiv, (targetStandaloneCounts.get(equiv) ?? 0) + 1);
    }
    // Any non-empty content inside an open frame counts as inner.
    const top = stack[stack.length - 1];
    if (top) top.innerSeen = true;
  });

  // Any opens left on the stack are unbalanced.
  for (const frame of stack) {
    out.push({
      kind: "unbalanced-open",
      message: `Open marker for id "${frame.id}" has no matching close.`,
      ref: frame.id,
    });
  }

  // Source pair ids that appear nowhere in the target.
  for (const id of shape.pairIds) {
    const seen = targetPairCounts.get(id) ?? 0;
    if (seen === 0) {
      out.push({
        kind: "dropped-pair",
        message: `Paired marker "${id}" required by source is missing from target.`,
        ref: id,
      });
    } else if (seen > 1) {
      out.push({
        kind: "duplicated-pair",
        message: `Paired marker "${id}" appears ${seen} times in target.`,
        ref: id,
      });
    }
  }

  // Source standalone equivs.
  for (const [equiv, info] of shape.standaloneEquivs) {
    const seen = targetStandaloneCounts.get(equiv) ?? 0;
    if (seen === 0) {
      out.push({
        kind: "dropped-standalone",
        message: `Standalone marker "${equiv}" required by source is missing from target.`,
        ref: equiv,
      });
    } else if (seen > 1 && !info.duplicable) {
      out.push({
        kind: "duplicated-standalone",
        message: `Standalone marker "${equiv}" appears ${seen} times in target.`,
        ref: equiv,
      });
    }
  }

  return out;
}

function describeSource(source: Run[]): SourceShape {
  const pairIds = new Set<string>();
  const pairInnerNonEmpty = new Set<string>();
  const standaloneEquivs = new Map<string, { duplicable: boolean }>();
  const innerStack: Array<{ id: string; nonEmpty: boolean }> = [];

  walkRuns(source, (run) => {
    if (isPcOpen(run)) {
      pairIds.add(run.pcOpen.id);
      innerStack.push({ id: run.pcOpen.id, nonEmpty: false });
      return;
    }
    if (isPcClose(run)) {
      const top = innerStack.pop();
      if (top && top.nonEmpty) pairInnerNonEmpty.add(top.id);
      const parent = innerStack[innerStack.length - 1];
      if (parent) parent.nonEmpty = true;
      return;
    }
    if (isPh(run)) {
      const equiv = run.ph.equiv;
      const cloneable = run.ph.constraints?.cloneable === true;
      // Don't shadow if the same equiv was seen with cloneable: true
      // earlier (be the most permissive).
      const existing = standaloneEquivs.get(equiv);
      standaloneEquivs.set(equiv, {
        duplicable: cloneable || (existing?.duplicable ?? false),
      });
    }
    const top = innerStack[innerStack.length - 1];
    if (top) top.nonEmpty = true;
  });

  return { pairIds, pairInnerNonEmpty, standaloneEquivs };
}

/**
 * Walk a Run sequence including nested plural / select forms. Plural
 * and select forms each carry their own scope; for cross-form
 * validation we still want to surface paired markers consistently, so
 * we visit every form in document order.
 */
function walkRuns(runs: Run[], visit: (run: Run) => void): void {
  for (const run of runs) {
    if ("plural" in run) {
      for (const form of Object.values(run.plural.forms)) {
        if (form) walkRuns(form, visit);
      }
      continue;
    }
    if ("select" in run) {
      for (const form of Object.values(run.select.cases)) {
        walkRuns(form, visit);
      }
      continue;
    }
    visit(run);
  }
}

function findOpenIndex(
  stack: Array<{ id: string; index: number; innerSeen: boolean }>,
  id: string,
): number {
  for (let i = stack.length - 1; i >= 0; i--) {
    if (stack[i].id === id) return i;
  }
  return -1;
}

function isPcOpen(run: Run): run is PcOpenRun {
  return "pcOpen" in run;
}

function isPcClose(run: Run): run is PcCloseRun {
  return "pcClose" in run;
}

function isPh(run: Run): run is PlaceholderRun {
  return "ph" in run;
}
