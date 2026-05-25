/**
 * Bridge from the legacy coded-text + span representation (still
 * produced by the `@neokapi/ui` editor) to the RFC 0001 Run sequence
 * the Wails backend now consumes.
 *
 * The bowrain platform editor (`@neokapi/ui`) has not yet been
 * migrated to author runs directly, so the desktop adapter converts
 * at the wire boundary. The mapping mirrors `runsCodedBridge` in
 * `@neokapi/ui-primitives`:
 *
 *   span_type 'opening'     → pcOpen
 *   span_type 'closing'     → pcClose
 *   span_type 'placeholder' → ph
 *   plain text run          → text
 */
import type { Run } from "../types/api";

interface CodedSpan {
  span_type: "opening" | "closing" | "placeholder";
  type: string;
  sub_type?: string;
  id: string;
  data: string;
  display_text?: string;
  equiv_text?: string;
}

// Unicode private-use markers matching the Go model constants.
const MARKER_MIN = 0xe001;
const MARKER_MAX = 0xe003;

function isMarker(ch: string): boolean {
  const code = ch.charCodeAt(0);
  return code >= MARKER_MIN && code <= MARKER_MAX;
}

/**
 * Convert coded text + spans into a Run sequence. Every coded marker
 * consumes the next span in order; intervening characters accumulate
 * into text runs.
 */
export function codedToRuns(codedText: string, spans: readonly CodedSpan[]): Run[] {
  const runs: Run[] = [];
  let textBuf = "";
  let spanIdx = 0;

  const flushText = () => {
    if (textBuf) {
      runs.push({ text: { text: textBuf } });
      textBuf = "";
    }
  };

  for (const ch of codedText) {
    if (isMarker(ch)) {
      flushText();
      const span = spans[spanIdx];
      if (span) {
        runs.push(spanToRun(span));
        spanIdx++;
      }
    } else {
      textBuf += ch;
    }
  }
  flushText();
  return runs;
}

function spanToRun(span: CodedSpan): Run {
  const equiv = span.equiv_text || span.id;
  switch (span.span_type) {
    case "opening":
      return {
        pcOpen: {
          id: span.id,
          type: span.type,
          ...(span.sub_type ? { subType: span.sub_type } : {}),
          data: span.data,
          equiv,
          ...(span.display_text ? { disp: span.display_text } : {}),
        },
      };
    case "closing":
      return {
        pcClose: {
          id: span.id,
          type: span.type,
          ...(span.sub_type ? { subType: span.sub_type } : {}),
          data: span.data,
          equiv,
        },
      };
    case "placeholder":
    default:
      return {
        ph: {
          id: span.id,
          type: span.type,
          ...(span.sub_type ? { subType: span.sub_type } : {}),
          data: span.data,
          equiv,
          ...(span.display_text ? { disp: span.display_text } : {}),
        },
      };
  }
}
