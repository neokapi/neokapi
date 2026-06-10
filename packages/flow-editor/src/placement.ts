// Transformer placement diagnostics (AD-006) — the client-side mirror of the
// Go placement pass (core/flow/placement.go), so the editor flags an unsafe
// transformer position inline while the user composes, before the build gate
// rejects it. Rules:
//
//  - Error: a transformer after a step that produces a committed target
//    orphans the targets (which anchor to the source it rewrites). A
//    transformer that itself produces the target port (unredact rewrites both
//    sides coherently) is exempt.
//
//  - Error: a recoverable transformer (redact) after a step that egresses
//    source to a remote sink leaks unprotected content. The step(s) producing
//    the inputs its config-resolved contract REQUIRES are exempt (the AD-020
//    detection trade-off: a cloud NER feeding entity-driven redaction).
//
//  - Warning: a transformer placed later than its earliest valid slot forces
//    the overlays produced in between to be rebased across its rewrite.
//
// The Go pass refines contracts per node config via registry contract
// resolvers; the client mirrors the two resolvers that matter here — redact's
// entity requirement and the AI tools' local-provider egress downgrade.

import type { FlowSpec, FlowStep, IOPort, ToolInfo } from "./types";

export type PlacementSeverity = "error" | "warning";

/** Placement rule identifiers — kept in sync with core/flow/placement.go. */
export const RULE_TRANSFORMER_AFTER_TARGET = "transformer-after-target";
export const RULE_TRANSFORMER_AFTER_EGRESS = "transformer-after-remote-egress";
export const RULE_TRANSFORMER_LATE = "transformer-late-placement";

export interface PlacementDiagnostic {
  severity: PlacementSeverity;
  rule: string;
  /** Index of the offending transformer in `spec.steps`. */
  stepIndex: number;
  tool: string;
  message: string;
}

const REMOTE_SOURCE_EGRESS = "remote-source-egress";
const PORT_TARGET = "target";
const PORT_SOURCE = "source";

/** Providers that keep content on the machine (mirrors aiprovider.IsLocalProvider). */
const LOCAL_PROVIDERS = new Set(["ollama", "demo"]);

const portKey = (p: IOPort) => `${p.type}@${p.side ?? "source"}`;

interface ResolvedStep {
  tool: string;
  consumes: IOPort[];
  produces: IOPort[];
  sideEffects: string[];
  isTransformer: boolean;
  recoverable: boolean;
}

/**
 * Resolve a step's contract from its tool info + config, mirroring the Go
 * contract resolvers: redact's entity port becomes required when its config
 * enables entity detection; an AI tool configured with a local provider drops
 * the remote-source-egress effect.
 */
function resolveStep(step: FlowStep, toolMap: Map<string, ToolInfo>): ResolvedStep | null {
  const info = toolMap.get(step.tool);
  if (!info) return null;

  let consumes = info.consumes ?? [];
  let sideEffects = info.side_effects ?? [];
  const config = step.config ?? {};

  if (step.tool === "redact") {
    const detectors = config.detectors as string[] | undefined;
    const entityTypes = config.entityTypes as string[] | undefined;
    const entitiesOn = (detectors?.includes("entities") ?? false) || (entityTypes?.length ?? 0) > 0;
    if (entitiesOn) {
      consumes = consumes.map((c) =>
        c.type === "entity" && (c.side ?? "source") === "source" ? { ...c, optional: false } : c,
      );
    }
  }

  const provider = config.provider;
  if (typeof provider === "string" && LOCAL_PROVIDERS.has(provider)) {
    sideEffects = sideEffects.filter((e) => e !== REMOTE_SOURCE_EGRESS);
  }

  const producesSource = (info.produces ?? []).some(
    (p) => p.type === PORT_SOURCE && (p.side ?? "source") === "source",
  );

  return {
    tool: step.tool,
    consumes,
    produces: info.produces ?? [],
    sideEffects,
    isTransformer: !!info.isSourceTransform || producesSource,
    recoverable: !!info.recoverable,
  };
}

/** Union resolution for a parallel group: branches merged into one contract. */
function resolveGroup(steps: FlowStep[], toolMap: Map<string, ToolInfo>): ResolvedStep | null {
  const resolved = steps
    .map((s) => resolveStep(s, toolMap))
    .filter((r): r is ResolvedStep => r !== null);
  if (resolved.length === 0) return null;
  return {
    tool: resolved.map((r) => r.tool).join("+"),
    consumes: resolved.flatMap((r) => r.consumes),
    produces: resolved.flatMap((r) => r.produces),
    sideEffects: resolved.flatMap((r) => r.sideEffects),
    isTransformer: resolved.some((r) => r.isTransformer),
    recoverable: resolved.some((r) => r.recoverable),
  };
}

const producesTarget = (r: ResolvedStep) =>
  r.produces.some((p) => p.type === PORT_TARGET && (p.side ?? "source") === "target");

const feedsAny = (up: ResolvedStep, keys: Set<string>) =>
  up.produces.some((p) => keys.has(portKey(p)));

const consumedKeys = (r: ResolvedStep, includeOptional: boolean) =>
  new Set(r.consumes.filter((c) => includeOptional || !c.optional).map(portKey));

/**
 * Run the placement pass over a FlowSpec, returning diagnostics indexed by
 * step. Steps whose tools are unknown to the tool map are skipped.
 */
export function computePlacement(
  spec: FlowSpec,
  toolMap: Map<string, ToolInfo>,
): PlacementDiagnostic[] {
  const resolved: (ResolvedStep | null)[] = spec.steps.map((step) =>
    step.parallel && step.parallel.length > 0
      ? resolveGroup(step.parallel, toolMap)
      : resolveStep(step, toolMap),
  );

  const diags: PlacementDiagnostic[] = [];
  resolved.forEach((info, i) => {
    if (!info?.isTransformer) return;

    const consumed = consumedKeys(info, true);
    const required = consumedKeys(info, false);
    const selfProducesTarget = producesTarget(info);

    let lastInputIdx = -1;
    for (let j = 0; j < i; j++) {
      const up = resolved[j];
      if (!up) continue;
      if (feedsAny(up, consumed)) lastInputIdx = j;

      if (!selfProducesTarget && producesTarget(up)) {
        diags.push({
          severity: "error",
          rule: RULE_TRANSFORMER_AFTER_TARGET,
          stepIndex: i,
          tool: info.tool,
          message: `"${info.tool}" follows "${up.tool}", which produces targets: rewriting source orphans the targets that anchor to it — move it before any target-producing step`,
        });
      }

      if (
        info.recoverable &&
        up.sideEffects.includes(REMOTE_SOURCE_EGRESS) &&
        !feedsAny(up, required)
      ) {
        diags.push({
          severity: "error",
          rule: RULE_TRANSFORMER_AFTER_EGRESS,
          stepIndex: i,
          tool: info.tool,
          message: `"${info.tool}" must run before "${up.tool}", which sends source to a remote sink: unprotected content leaks before redaction applies — move it earlier, or use a local provider`,
        });
      }
    }

    const rebased: string[] = [];
    for (let j = lastInputIdx + 1; j < i; j++) {
      const up = resolved[j];
      if (!up) continue;
      for (const p of up.produces) {
        const side = p.side ?? "source";
        if (
          side === "source" &&
          p.type !== PORT_TARGET &&
          p.type !== PORT_SOURCE &&
          !consumed.has(portKey(p))
        ) {
          rebased.push(`${p.type} (from "${up.tool}")`);
        }
      }
    }
    if (rebased.length > 0) {
      diags.push({
        severity: "warning",
        rule: RULE_TRANSFORMER_LATE,
        stepIndex: i,
        tool: info.tool,
        message: `"${info.tool}" is placed later than needed: the ${rebased.join(", ")} overlay(s) produced before it must be rebased across its rewrite — move it right after its last required input`,
      });
    }
  });
  return diags;
}

/** Group diagnostics by step index for per-node rendering. */
export function placementByStep(diags: PlacementDiagnostic[]): Map<number, PlacementDiagnostic[]> {
  const byStep = new Map<number, PlacementDiagnostic[]>();
  for (const d of diags) {
    const list = byStep.get(d.stepIndex);
    if (list) list.push(d);
    else byStep.set(d.stepIndex, [d]);
  }
  return byStep;
}
