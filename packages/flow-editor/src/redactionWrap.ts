// Guided redaction: redact and unredact are a pair. `redact` runs in the
// source-transform stage (replacing sensitive spans with placeholders before
// the main tools see them) and `unredact` runs last (restoring the originals
// after processing). These helpers add/remove that wrap as one action and detect
// an incomplete wrap (redact without a matching unredact — secrets never
// restored).

import type { FlowSpec } from "./types";

const REDACT = "redact";
const UNREDACT = "unredact";

const inStage = (spec: FlowSpec, tool: string) =>
  (spec.sourceTransforms ?? []).some((s) => s.tool === tool) ||
  spec.steps.some((s) => s.tool === tool);

/** True when the flow has both a `redact` source-transform and a trailing `unredact`. */
export function hasRedactionWrap(spec: FlowSpec): boolean {
  return (
    (spec.sourceTransforms ?? []).some((s) => s.tool === REDACT) &&
    spec.steps.some((s) => s.tool === UNREDACT)
  );
}

/** redact is present but nothing restores it — the secrets are never put back. */
export function redactionIncomplete(spec: FlowSpec): boolean {
  return inStage(spec, REDACT) && !inStage(spec, UNREDACT);
}

/** Add the redaction wrap: `redact` first (source-transform), `unredact` last. */
export function wrapWithRedaction(spec: FlowSpec): FlowSpec {
  const st = spec.sourceTransforms ?? [];
  const sourceTransforms = st.some((s) => s.tool === REDACT) ? st : [{ tool: REDACT }, ...st];
  const steps = spec.steps.some((s) => s.tool === UNREDACT)
    ? spec.steps
    : [...spec.steps, { tool: UNREDACT }];
  return { ...spec, sourceTransforms, steps };
}

/** Remove the redaction wrap (both redact and unredact, wherever they sit). */
export function unwrapRedaction(spec: FlowSpec): FlowSpec {
  const sourceTransforms = (spec.sourceTransforms ?? []).filter(
    (s) => s.tool !== REDACT && s.tool !== UNREDACT,
  );
  const steps = spec.steps.filter((s) => s.tool !== REDACT && s.tool !== UNREDACT);
  const next: FlowSpec = { ...spec, steps };
  if (sourceTransforms.length > 0) next.sourceTransforms = sourceTransforms;
  else delete next.sourceTransforms;
  return next;
}
