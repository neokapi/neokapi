// Guided redaction: redact and unredact are a pair. `redact` runs first
// (replacing sensitive spans with placeholders before later steps — and any
// remote provider — see them) and `unredact` runs last (restoring the
// originals after processing). These helpers add/remove that wrap as one
// action and detect an incomplete wrap (redact without a matching unredact —
// secrets never restored).

import type { FlowSpec } from "./types";

const REDACT = "redact";
const UNREDACT = "unredact";

const hasTool = (spec: FlowSpec, tool: string) => spec.steps.some((s) => s.tool === tool);

/** True when the flow has both a leading `redact` and a trailing `unredact`. */
export function hasRedactionWrap(spec: FlowSpec): boolean {
  return hasTool(spec, REDACT) && hasTool(spec, UNREDACT);
}

/** redact is present but nothing restores it — the secrets are never put back. */
export function redactionIncomplete(spec: FlowSpec): boolean {
  return hasTool(spec, REDACT) && !hasTool(spec, UNREDACT);
}

/** Add the redaction wrap: `redact` as the first step, `unredact` as the last. */
export function wrapWithRedaction(spec: FlowSpec): FlowSpec {
  let steps = spec.steps;
  if (!hasTool(spec, REDACT)) steps = [{ tool: REDACT }, ...steps];
  if (!steps.some((s) => s.tool === UNREDACT)) steps = [...steps, { tool: UNREDACT }];
  return { ...spec, steps };
}

/** Remove the redaction wrap (both redact and unredact, wherever they sit). */
export function unwrapRedaction(spec: FlowSpec): FlowSpec {
  return { ...spec, steps: spec.steps.filter((s) => s.tool !== REDACT && s.tool !== UNREDACT) };
}
