import type { SpanInfo } from "../../types/api";
import { getDefaultRegistry } from "../../vocabularies";

/** Resolved editing constraints for a span, combining SpanInfo overrides with vocabulary defaults. */
export interface ResolvedConstraints {
  deletable: boolean;
  cloneable: boolean;
  reorderable: boolean;
}

/**
 * Resolve editing constraints for a span. SpanInfo fields override vocabulary
 * defaults; unknown types fall back to all-true (permissive).
 */
export function resolveConstraints(span: SpanInfo): ResolvedConstraints {
  const vocabInfo = getDefaultRegistry().lookupOrFallback(span.type);
  return {
    deletable: span.deletable ?? vocabInfo.constraints.deletable,
    cloneable: span.cloneable ?? vocabInfo.constraints.cloneable,
    reorderable: span.can_reorder ?? vocabInfo.constraints.reorderable,
  };
}

/** Whether the span can be deleted from the target. */
export function isDeletable(span: SpanInfo): boolean {
  return resolveConstraints(span).deletable;
}

/** Whether the span can be duplicated in the target. */
export function isCloneable(span: SpanInfo): boolean {
  return resolveConstraints(span).cloneable;
}
