// Requirement analysis: for each step, which non-optional consumed ports does
// nothing upstream produce? Those are unmet requirements the editor flags on the
// node (and explains in the config panel) — e.g. a qa-check that needs a
// committed target but sits before any translation tool.
//
// The flow is linear: steps run in order. A parallel group's branches all see
// the same upstream (the steps before the group), and the group's produces
// become available to steps after it. The source binding always makes the
// source content available.

import type { FlowSpec, IOPort, ToolInfo } from "./types";

/** Unmet (non-optional, unproduced-upstream) consumed ports, per step. */
export interface UnmetReport {
  /** Unmet ports for each step (union across branches for a parallel group). */
  steps: string[][];
}

const portKey = (p: IOPort) => `${p.type}@${p.side ?? "source"}`;

export function computeUnmet(spec: FlowSpec, toolMap: Map<string, ToolInfo>): UnmetReport {
  // The source binding always supplies the source content.
  const produced = new Set<string>(["source@source"]);

  const unmetFor = (tool: string): string[] => {
    const info = toolMap.get(tool);
    if (!info?.consumes) return [];
    return info.consumes.filter((c) => !c.optional && !produced.has(portKey(c))).map(portKey);
  };
  const addProduces = (tool: string) => {
    const info = toolMap.get(tool);
    for (const p of info?.produces ?? []) produced.add(portKey(p));
  };

  const steps = spec.steps.map((step) => {
    if (step.parallel && step.parallel.length > 0) {
      // Branches share the upstream; the group's produces apply only afterwards.
      const unmet = new Set<string>();
      for (const b of step.parallel) for (const u of unmetFor(b.tool)) unmet.add(u);
      for (const b of step.parallel) addProduces(b.tool);
      return [...unmet];
    }
    const unmet = unmetFor(step.tool);
    addProduces(step.tool);
    return unmet;
  });

  return { steps };
}
