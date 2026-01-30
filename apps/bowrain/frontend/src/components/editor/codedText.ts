import type { SpanInfo } from "../../types/api";

// Unicode markers matching the Go model constants
const MARKER_OPENING = "\uE001";
const MARKER_CLOSING = "\uE002";
const MARKER_PLACEHOLDER = "\uE003";

function isMarker(ch: string): boolean {
  const code = ch.charCodeAt(0);
  return code >= 0xe001 && code <= 0xe003;
}

/** A parsed segment of coded text — either plain text or a tag reference. */
export type CodedSegment =
  | { type: "text"; value: string }
  | { type: "tag"; spanInfo: SpanInfo };

/**
 * Parse coded text + spans into an array of segments for read-only rendering.
 * No Lexical dependency — used by SourceCellDisplay.
 */
export function parseCodedSegments(
  codedText: string,
  spans: SpanInfo[],
): CodedSegment[] {
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
 * Opening: "b>" or "a>", Closing: "/b" or "/a", Placeholder: "br" or "img"
 */
export function spanLabel(span: SpanInfo): string {
  const name = span.type || tagNameFromData(span.data);
  switch (span.span_type) {
    case "opening":
      return `${name}>`;
    case "closing":
      return `/${name}`;
    case "placeholder":
      return name;
    default:
      return name;
  }
}

/**
 * Extract the tag name from raw markup data like "<b>", "</a>", "<br/>"
 */
function tagNameFromData(data: string): string {
  const match = data.match(/<\/?(\w+)/);
  return match ? match[1] : "?";
}
