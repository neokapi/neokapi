// Requirement analysis: for each step, which non-optional consumed ports does
// nothing upstream produce? Those are unmet requirements the editor flags on the
// node (and explains in the config panel) — e.g. a qa that needs a
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

export const portKey = (p: IOPort) => `${p.type}@${p.side ?? "source"}`;

/** The source binding always supplies the source content at every slot. */
const SOURCE_PORT: IOPort = { type: "source", side: "source" };

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

/**
 * What data is available at a given insertion slot — the ports produced by every
 * step before it, plus the always-present source content. Drives the Add panel's
 * "available here" strip and per-tool compatibility marking: a tool whose
 * non-optional inputs are all in `has` reads cleanly at that slot.
 *
 * `slot` is an index into `spec.steps` (0 = before the first step, steps.length =
 * append at the end). A parallel group's branches share the upstream before the
 * group, so adding a branch uses the group's own step index as the slot.
 */
export interface SlotContext {
  /** Ports available at this slot (source + everything produced upstream), deduped. */
  available: IOPort[];
  /** Fast membership test, keyed `type@side` (matches `portKey`). */
  has: Set<string>;
}

export function slotContext(
  spec: FlowSpec,
  toolMap: Map<string, ToolInfo>,
  slot: number,
): SlotContext {
  const has = new Set<string>([portKey(SOURCE_PORT)]);
  const available: IOPort[] = [SOURCE_PORT];
  const add = (tool: string) => {
    const info = toolMap.get(tool);
    for (const p of info?.produces ?? []) {
      const k = portKey(p);
      if (has.has(k)) continue;
      has.add(k);
      available.push(p);
    }
  };

  const upto = Math.max(0, Math.min(slot, spec.steps.length));
  for (let i = 0; i < upto; i++) {
    const step = spec.steps[i];
    if (step.parallel && step.parallel.length > 0) {
      for (const b of step.parallel) add(b.tool);
    } else if (step.tool) {
      add(step.tool);
    }
  }
  return { available, has };
}

/** How well a tool fits a slot: its non-optional inputs not yet produced upstream. */
export interface ToolFit {
  /** True when every required input is available at the slot. */
  ready: boolean;
  /** The required (non-optional) input ports that nothing upstream produces. */
  unmet: IOPort[];
}

export function toolFit(info: ToolInfo, ctx: SlotContext): ToolFit {
  const unmet = (info.consumes ?? []).filter((c) => !c.optional && !ctx.has.has(portKey(c)));
  return { ready: unmet.length === 0, unmet };
}
