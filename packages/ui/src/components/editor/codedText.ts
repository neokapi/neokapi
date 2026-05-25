import { type Run, flattenRuns } from "@neokapi/kapi-format";

import type { SpanInfo } from "../../types/span";
import { semanticLabel } from "./tagSemantics";

// Unicode markers matching the Go model constants
const MARKER_OPENING = "\uE001";
const MARKER_CLOSING = "\uE002";
const MARKER_PLACEHOLDER = "\uE003";

function isMarker(ch: string): boolean {
  const code = ch.charCodeAt(0);
  return code >= 0xe001 && code <= 0xe003;
}

/** A parsed segment of coded text — either plain text or a tag reference. */
export type CodedSegment = { type: "text"; value: string } | { type: "tag"; spanInfo: SpanInfo };

/**
 * Parse coded text + spans into an array of segments for read-only rendering.
 * No Lexical dependency — used by SourceCellDisplay.
 */
export function parseCodedSegments(codedText: string, spans: SpanInfo[]): CodedSegment[] {
  const segments: CodedSegment[] = [];
  let textBuf = "";
  let spanIdx = 0;

  for (const ch of codedText) {
    if (isMarker(ch)) {
      if (textBuf) {
        segments.push({ type: "text", value: textBuf });
        textBuf = "";
      }
      if (spanIdx < spans.length) {
        segments.push({ type: "tag", spanInfo: spans[spanIdx] });
        spanIdx++;
      }
    } else {
      textBuf += ch;
    }
  }

  if (textBuf) {
    segments.push({ type: "text", value: textBuf });
  }

  return segments;
}

/**
 * Serialize an array of CodedSegments back to coded text + spans.
 * Used after Lexical editor state is walked.
 */
export function segmentsToCodedText(segments: CodedSegment[]): {
  codedText: string;
  spans: SpanInfo[];
} {
  let codedText = "";
  const spans: SpanInfo[] = [];

  for (const seg of segments) {
    if (seg.type === "text") {
      codedText += seg.value;
    } else {
      const marker =
        seg.spanInfo.span_type === "opening"
          ? MARKER_OPENING
          : seg.spanInfo.span_type === "closing"
            ? MARKER_CLOSING
            : MARKER_PLACEHOLDER;
      codedText += marker;
      spans.push(seg.spanInfo);
    }
  }

  return { codedText, spans };
}

/**
 * Returns the tag display label for a span.
 * Delegates to tagSemantics for semantic-aware labels.
 */
export function spanLabel(span: SpanInfo): string {
  return semanticLabel(span);
}

/**
 * Convert an RFC 0001 Run sequence into read-only display segments.
 * Text runs become text segments; ph / pcOpen / pcClose / sub runs
 * become tag segments rendered as chips. Structured plural / select
 * runs are flattened to their ICU text form (read-only display has
 * no per-form chip affordance), keeping the display lossless for the
 * common inline-code cases while staying robust for the rest.
 *
 * Mirrors the span mapping in `runsCodedBridge.runsToCoded`, but
 * tolerates plural / select instead of throwing.
 */
export function runsToSegments(runs: readonly Run[]): CodedSegment[] {
  const segments: CodedSegment[] = [];
  for (const r of runs) {
    if ("text" in r) {
      if (r.text) segments.push({ type: "text", value: r.text });
      continue;
    }
    if ("ph" in r) {
      segments.push({
        type: "tag",
        spanInfo: {
          span_type: "placeholder",
          type: r.ph.type,
          sub_type: r.ph.subType,
          id: r.ph.id,
          data: r.ph.data,
          equiv_text: r.ph.equiv,
          display_text: r.ph.disp,
        },
      });
      continue;
    }
    if ("pcOpen" in r) {
      segments.push({
        type: "tag",
        spanInfo: {
          span_type: "opening",
          type: r.pcOpen.type,
          sub_type: r.pcOpen.subType,
          id: r.pcOpen.id,
          data: r.pcOpen.data,
          equiv_text: r.pcOpen.equiv,
          display_text: r.pcOpen.disp,
        },
      });
      continue;
    }
    if ("pcClose" in r) {
      segments.push({
        type: "tag",
        spanInfo: {
          span_type: "closing",
          type: r.pcClose.type,
          sub_type: r.pcClose.subType,
          id: r.pcClose.id,
          data: r.pcClose.data,
          equiv_text: r.pcClose.equiv,
        },
      });
      continue;
    }
    if ("sub" in r) {
      segments.push({
        type: "tag",
        spanInfo: {
          span_type: "placeholder",
          type: "sub",
          id: r.sub.id,
          data: `[${r.sub.equiv}]`,
          equiv_text: r.sub.equiv,
        },
      });
      continue;
    }
    // plural / select: flatten to their ICU text form for display.
    segments.push({ type: "text", value: flattenRuns([r]) });
  }
  return segments;
}
