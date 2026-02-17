import type { SpanInfo } from "../../types/api";

// --- Semantic Categories ---

export type SemanticCategory =
  | "bold"
  | "italic"
  | "underline"
  | "strikethrough"
  | "link"
  | "image"
  | "code"
  | "subscript"
  | "superscript"
  | "break"
  | "generic";

const categoryMap: Record<string, SemanticCategory> = {
  b: "bold",
  strong: "bold",
  bold: "bold",
  i: "italic",
  em: "italic",
  italic: "italic",
  emphasis: "italic",
  u: "underline",
  underline: "underline",
  s: "strikethrough",
  del: "strikethrough",
  strike: "strikethrough",
  a: "link",
  link: "link",
  img: "image",
  image: "image",
  code: "code",
  tt: "code",
  kbd: "code",
  samp: "code",
  var: "code",
  sub: "subscript",
  sup: "superscript",
  br: "break",
  wbr: "break",
  hr: "break",
};

/** Extract tag name from raw markup data like "<b>", "</a>", "<br/>" */
export function tagNameFromData(data: string): string {
  const match = data.match(/<\/?(\w+)/);
  return match ? match[1] : "?";
}

/** Resolve a span to its semantic category. */
export function semanticCategory(span: SpanInfo): SemanticCategory {
  const name = (span.type || tagNameFromData(span.data)).toLowerCase();
  return categoryMap[name] || "generic";
}

// --- Color Palette ---

export interface TagColorScheme {
  bg: string;
  border: string;
  text: string;
}

const colorPalette: Record<SemanticCategory, TagColorScheme> = {
  bold: { bg: "rgba(59, 130, 246, 0.15)", border: "rgba(59, 130, 246, 0.5)", text: "rgb(59, 130, 246)" },
  italic: { bg: "rgba(168, 85, 247, 0.15)", border: "rgba(168, 85, 247, 0.5)", text: "rgb(168, 85, 247)" },
  underline: { bg: "rgba(14, 165, 233, 0.15)", border: "rgba(14, 165, 233, 0.5)", text: "rgb(14, 165, 233)" },
  strikethrough: { bg: "rgba(244, 63, 94, 0.15)", border: "rgba(244, 63, 94, 0.5)", text: "rgb(244, 63, 94)" },
  link: { bg: "rgba(34, 197, 94, 0.15)", border: "rgba(34, 197, 94, 0.5)", text: "rgb(34, 197, 94)" },
  image: { bg: "rgba(251, 146, 60, 0.15)", border: "rgba(251, 146, 60, 0.5)", text: "rgb(234, 88, 12)" },
  code: { bg: "rgba(100, 116, 139, 0.15)", border: "rgba(100, 116, 139, 0.5)", text: "rgb(100, 116, 139)" },
  subscript: { bg: "rgba(236, 72, 153, 0.15)", border: "rgba(236, 72, 153, 0.5)", text: "rgb(236, 72, 153)" },
  superscript: { bg: "rgba(217, 70, 239, 0.15)", border: "rgba(217, 70, 239, 0.5)", text: "rgb(217, 70, 239)" },
  break: { bg: "rgba(251, 146, 60, 0.15)", border: "rgba(251, 146, 60, 0.5)", text: "rgb(234, 88, 12)" },
  generic: { bg: "rgba(156, 163, 175, 0.15)", border: "rgba(156, 163, 175, 0.5)", text: "rgb(107, 114, 128)" },
};

/** Get the color scheme for a span based on its semantic category. */
export function tagColors(span: SpanInfo): TagColorScheme {
  return colorPalette[semanticCategory(span)];
}

// --- Labels & Tooltips ---

const categoryLabels: Record<SemanticCategory, string> = {
  bold: "B",
  italic: "I",
  underline: "U",
  strikethrough: "S",
  link: "a",
  image: "img",
  code: "code",
  subscript: "sub",
  superscript: "sup",
  break: "br",
  generic: "",
};

const categoryNames: Record<SemanticCategory, string> = {
  bold: "Bold",
  italic: "Italic",
  underline: "Underline",
  strikethrough: "Strikethrough",
  link: "Link",
  image: "Image",
  code: "Code",
  subscript: "Subscript",
  superscript: "Superscript",
  break: "Break",
  generic: "Tag",
};

/** Rich chip label: "B>" for bold opening, "/I" for italic closing, "br" for break placeholder. */
export function semanticLabel(span: SpanInfo): string {
  const cat = semanticCategory(span);
  const label = categoryLabels[cat] || span.type || tagNameFromData(span.data);
  switch (span.span_type) {
    case "opening":
      return `${label}>`;
    case "closing":
      return `/${label}`;
    case "placeholder":
      return label;
    default:
      return label;
  }
}

/** Tooltip with semantic name and raw data: e.g. "Bold open (b) — <b>" */
export function semanticTooltip(span: SpanInfo): string {
  const cat = semanticCategory(span);
  const name = categoryNames[cat];
  const tagName = span.type || tagNameFromData(span.data);
  const spanTypeLabel = span.span_type === "opening" ? "open" : span.span_type === "closing" ? "close" : "placeholder";
  return `${name} ${spanTypeLabel} (${tagName}) — ${span.data}`;
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
 * Returns a map from span array index to pair info.
 */
export function buildPairs(spans: SpanInfo[]): Map<number, SpanPairInfo> {
  const result = new Map<number, SpanPairInfo>();
  // Stack per type: key = lowercase type name, value = array of (spanIndex, pairIndex)
  const stacks = new Map<string, number[]>();
  let nextPair = 1;

  for (let i = 0; i < spans.length; i++) {
    const span = spans[i];
    const typeName = (span.type || tagNameFromData(span.data)).toLowerCase();

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
  type: "missing_tag" | "extra_tag" | "unpaired";
  message: string;
}

/** Count tags by fingerprint (type + span_type). */
function tagFingerprints(spans: SpanInfo[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const span of spans) {
    const key = `${(span.type || tagNameFromData(span.data)).toLowerCase()}:${span.span_type}`;
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return counts;
}

/** Validate target spans against source spans. */
export function validateTags(sourceSpans: SpanInfo[], targetSpans: SpanInfo[]): TagValidationResult {
  const errors: TagValidationIssue[] = [];
  const warnings: TagValidationIssue[] = [];

  const sourceCounts = tagFingerprints(sourceSpans);
  const targetCounts = tagFingerprints(targetSpans);

  // Check for missing tags
  for (const [key, sourceCount] of sourceCounts) {
    const targetCount = targetCounts.get(key) || 0;
    if (targetCount < sourceCount) {
      const [tagType, spanType] = key.split(":");
      const missing = sourceCount - targetCount;
      errors.push({
        type: "missing_tag",
        message: `Missing ${missing} ${spanType} "${tagType}" tag${missing > 1 ? "s" : ""}`,
      });
    }
  }

  // Check for extra tags
  for (const [key, targetCount] of targetCounts) {
    const sourceCount = sourceCounts.get(key) || 0;
    if (targetCount > sourceCount) {
      const [tagType, spanType] = key.split(":");
      const extra = targetCount - sourceCount;
      warnings.push({
        type: "extra_tag",
        message: `Extra ${extra} ${spanType} "${tagType}" tag${extra > 1 ? "s" : ""}`,
      });
    }
  }

  // Check for unpaired tags in target
  const targetStacks = new Map<string, number>();
  for (const span of targetSpans) {
    const typeName = (span.type || tagNameFromData(span.data)).toLowerCase();
    if (span.span_type === "opening") {
      targetStacks.set(typeName, (targetStacks.get(typeName) || 0) + 1);
    } else if (span.span_type === "closing") {
      const count = targetStacks.get(typeName) || 0;
      if (count > 0) {
        targetStacks.set(typeName, count - 1);
      } else {
        errors.push({
          type: "unpaired",
          message: `Closing "${typeName}" without matching opening tag`,
        });
      }
    }
  }
  for (const [typeName, count] of targetStacks) {
    if (count > 0) {
      errors.push({
        type: "unpaired",
        message: `${count} opening "${typeName}" tag${count > 1 ? "s" : ""} without matching closing tag`,
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

/** HTML tag mapping per category (whitelist only). */
const htmlTagMap: Record<SemanticCategory, { open: string; close: string } | null> = {
  bold: { open: "<b>", close: "</b>" },
  italic: { open: "<i>", close: "</i>" },
  underline: { open: "<u>", close: "</u>" },
  strikethrough: { open: "<s>", close: "</s>" },
  link: { open: '<a style="color:inherit;text-decoration:underline">', close: "</a>" },
  code: { open: '<code style="background:rgba(100,116,139,0.15);padding:0 3px;border-radius:2px">', close: "</code>" },
  subscript: { open: "<sub>", close: "</sub>" },
  superscript: { open: "<sup>", close: "</sup>" },
  image: null,
  break: null,
  generic: null,
};

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/**
 * Convert coded text + spans into safe preview HTML.
 * Only whitelisted tags are emitted; everything else is escaped.
 */
export function codedTextToHtml(codedText: string, spans: SpanInfo[]): string {
  let html = "";
  let spanIdx = 0;

  for (const ch of codedText) {
    const code = ch.charCodeAt(0);
    if (code >= 0xe001 && code <= 0xe003) {
      if (spanIdx < spans.length) {
        const span = spans[spanIdx];
        spanIdx++;
        const cat = semanticCategory(span);
        const mapping = htmlTagMap[cat];

        if (span.span_type === "placeholder") {
          if (cat === "break") {
            html += "<br/>";
          } else if (cat === "image") {
            html += '<span style="opacity:0.5">[img]</span>';
          } else {
            html += `<span style="opacity:0.5">[${escapeHtml(span.type || "?")}]</span>`;
          }
        } else if (mapping) {
          html += span.span_type === "opening" ? mapping.open : mapping.close;
        }
        // For generic/unknown, we simply skip the marker (text flows through)
      }
    } else {
      html += escapeHtml(ch);
    }
  }

  return html;
}
