import type { SpanInfo } from "../../types/api";
import { getDefaultRegistry, type ColorScheme } from "../../vocabularies";
import { isDeletable, isCloneable } from "./tagConstraints";

// Re-export ColorScheme as TagColorScheme for backward compatibility.
export type TagColorScheme = ColorScheme;

/** Semantic category string from the vocabulary (e.g., "formatting", "linking", "structure"). */
export type SemanticCategory = string;

/** Extract tag name from raw markup data like "<b>", "</a>", "<br/>" */
export function tagNameFromData(data: string): string {
  const match = data.match(/<\/?(\w+)/);
  return match ? match[1] : "?";
}

/** Resolve a span to its vocabulary category. */
export function semanticCategory(span: SpanInfo): SemanticCategory {
  return getDefaultRegistry().lookupOrFallback(span.type).category;
}

/** Get the color scheme for a span based on its vocabulary type. */
export function tagColors(span: SpanInfo): TagColorScheme {
  return getDefaultRegistry().chipColor(span);
}

/** Rich chip label from vocabulary: "B>" for bold opening, "/I" for italic closing, "br" for break. */
export function semanticLabel(span: SpanInfo): string {
  // Use display_text from the backend if available.
  if (span.display_text) {
    return span.display_text;
  }
  return getDefaultRegistry().chipLabel(span);
}

/** Tooltip with semantic name and raw data: e.g. "Bold open — <b>" */
export function semanticTooltip(span: SpanInfo): string {
  const registry = getDefaultRegistry();
  const info = registry.lookupOrFallback(span.type);
  const spanTypeLabel =
    span.span_type === "opening" ? "open" : span.span_type === "closing" ? "close" : "placeholder";
  return `${info.label} ${spanTypeLabel} — ${span.data}`;
}

// --- Pair Analysis ---

export interface SpanPairInfo {
  /** 0-based index of the span in the spans array */
  spanIndex: number;
  /** 1-based pair number (shared between opening and closing) */
  pairIndex: number;
}

/**
 * Stack-based sequential matching of opening/closing pairs.
 * Uses span.id for pairing when available; falls back to type-based stacks.
 */
export function buildPairs(spans: SpanInfo[]): Map<number, SpanPairInfo> {
  const result = new Map<number, SpanPairInfo>();
  // Stack per type: key = span type, value = array of span indices
  const stacks = new Map<string, number[]>();
  let nextPair = 1;

  for (let i = 0; i < spans.length; i++) {
    const span = spans[i];
    const typeName = span.type;

    if (span.span_type === "opening") {
      const pairIndex = nextPair++;
      result.set(i, { spanIndex: i, pairIndex });
      const stack = stacks.get(typeName) || [];
      stack.push(i);
      stacks.set(typeName, stack);
    } else if (span.span_type === "closing") {
      const stack = stacks.get(typeName);
      if (stack && stack.length > 0) {
        const openIdx = stack.pop()!;
        const openPair = result.get(openIdx)!;
        result.set(i, { spanIndex: i, pairIndex: openPair.pairIndex });
      } else {
        // Unmatched closing — give it its own pair
        result.set(i, { spanIndex: i, pairIndex: nextPair++ });
      }
    } else {
      // Placeholder — standalone pair
      result.set(i, { spanIndex: i, pairIndex: nextPair++ });
    }
  }

  return result;
}

// --- Validation ---

export interface TagValidationResult {
  valid: boolean;
  errors: TagValidationIssue[];
  warnings: TagValidationIssue[];
}

export interface TagValidationIssue {
  type: "missing_tag" | "extra_tag" | "unpaired" | "deleted_non_deletable" | "cloned_non_cloneable";
  message: string;
}

/** Count tags by fingerprint (type + span_type). Uses | separator since type may contain ':'. */
function tagFingerprints(spans: SpanInfo[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const span of spans) {
    const key = `${span.type}|${span.span_type}`;
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return counts;
}

/** Validate target spans against source spans. */
export function validateTags(
  sourceSpans: SpanInfo[],
  targetSpans: SpanInfo[],
): TagValidationResult {
  const errors: TagValidationIssue[] = [];
  const warnings: TagValidationIssue[] = [];
  const registry = getDefaultRegistry();

  const sourceCounts = tagFingerprints(sourceSpans);
  const targetCounts = tagFingerprints(targetSpans);

  // Build a map from fingerprint → first matching source SpanInfo (for constraint checks).
  const sourceSpanByKey = new Map<string, SpanInfo>();
  for (const s of sourceSpans) {
    const k = `${s.type}|${s.span_type}`;
    if (!sourceSpanByKey.has(k)) sourceSpanByKey.set(k, s);
  }

  // Check for missing tags
  for (const [key, sourceCount] of sourceCounts) {
    const targetCount = targetCounts.get(key) || 0;
    if (targetCount < sourceCount) {
      const sepIdx = key.lastIndexOf("|");
      const tagType = key.slice(0, sepIdx);
      const spanType = key.slice(sepIdx + 1);
      const info = registry.lookup(tagType);
      const label = info?.label ?? tagType;
      const missing = sourceCount - targetCount;
      const representative = sourceSpanByKey.get(key);
      if (representative && !isDeletable(representative)) {
        errors.push({
          type: "deleted_non_deletable",
          message: `Missing ${missing} non-deletable ${spanType} "${label}" tag${missing > 1 ? "s" : ""}`,
        });
      } else {
        errors.push({
          type: "missing_tag",
          message: `Missing ${missing} ${spanType} "${label}" tag${missing > 1 ? "s" : ""}`,
        });
      }
    }
  }

  // Check for extra tags
  for (const [key, targetCount] of targetCounts) {
    const sourceCount = sourceCounts.get(key) || 0;
    if (targetCount > sourceCount) {
      const sepIdx = key.lastIndexOf("|");
      const tagType = key.slice(0, sepIdx);
      const spanType = key.slice(sepIdx + 1);
      const info = registry.lookup(tagType);
      const label = info?.label ?? tagType;
      const extra = targetCount - sourceCount;
      const representative = sourceSpanByKey.get(key);
      if (representative && !isCloneable(representative)) {
        errors.push({
          type: "cloned_non_cloneable",
          message: `Duplicated ${extra} non-cloneable ${spanType} "${label}" tag${extra > 1 ? "s" : ""}`,
        });
      } else {
        warnings.push({
          type: "extra_tag",
          message: `Extra ${extra} ${spanType} "${label}" tag${extra > 1 ? "s" : ""}`,
        });
      }
    }
  }

  // Check for unpaired tags in target
  const targetStacks = new Map<string, number>();
  for (const span of targetSpans) {
    const typeName = span.type;
    if (span.span_type === "opening") {
      targetStacks.set(typeName, (targetStacks.get(typeName) || 0) + 1);
    } else if (span.span_type === "closing") {
      const count = targetStacks.get(typeName) || 0;
      if (count > 0) {
        targetStacks.set(typeName, count - 1);
      } else {
        const info = registry.lookup(typeName);
        const label = info?.label ?? typeName;
        errors.push({
          type: "unpaired",
          message: `Closing "${label}" without matching opening tag`,
        });
      }
    }
  }
  for (const [typeName, count] of targetStacks) {
    if (count > 0) {
      const info = registry.lookup(typeName);
      const label = info?.label ?? typeName;
      errors.push({
        type: "unpaired",
        message: `${count} opening "${label}" tag${count > 1 ? "s" : ""} without matching closing tag`,
      });
    }
  }

  return {
    valid: errors.length === 0,
    errors,
    warnings,
  };
}

// --- Preview HTML ---

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/**
 * Convert coded text + spans into safe preview HTML.
 * Uses vocabulary registry for HTML tag rendering.
 */
export function codedTextToHtml(codedText: string, spans: SpanInfo[]): string {
  const registry = getDefaultRegistry();
  let html = "";
  let spanIdx = 0;

  for (const ch of codedText) {
    const code = ch.charCodeAt(0);
    if (code >= 0xe001 && code <= 0xe003) {
      if (spanIdx < spans.length) {
        const span = spans[spanIdx];
        spanIdx++;

        if (span.span_type === "placeholder") {
          const info = registry.lookupOrFallback(span.type);
          if (info.equiv) {
            // Render text equivalent (e.g., "\n" for breaks).
            html += escapeHtml(info.equiv);
          } else {
            html += `<span style="opacity:0.5">[${escapeHtml(info.label)}]</span>`;
          }
        } else {
          const tag = registry.htmlTag(span);
          if (tag) {
            html += tag;
          }
          // For types where HTML tag is null/undefined, skip (text flows through).
        }
      }
    } else {
      html += escapeHtml(ch);
    }
  }

  return html;
}
