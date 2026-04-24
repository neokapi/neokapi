/**
 * Round-trip between the editor's `(codedText, SpanInfo[])` shape and
 * `@neokapi/kapi-format`'s typed `Run[]`. Sits in `@neokapi/ui-primitives`
 * so the conversion lives next to the code that produces both sides:
 * `codedText.ts` (parses coded text into segments) and the kapi-format
 * Run model that the runtime consumes.
 *
 * The mapping is one-to-one and lossless for the formats both sides
 * already understand:
 *
 *   span_type: 'opening'     ↔  PcOpenRun         ({=equiv})
 *   span_type: 'closing'     ↔  PcCloseRun        ({/=equiv})
 *   span_type: 'placeholder' ↔  PlaceholderRun    ({equiv})
 *   plain text segment       ↔  TextRun
 *
 * The flat-text marker form (`{=equiv}`) is what `flattenRuns` from
 * `@neokapi/kapi-format` already emits for `PcOpenRun`. That's the
 * same form ICU plurals carry inside their per-form content, so a
 * plural target authored chip-by-chip in each form's editor round-
 * trips byte-identically through `flattenRuns(plural).
 *
 * See AD #408 / #409.
 */

import type { Run } from "@neokapi/kapi-format";

import { parseCodedSegments, segmentsToCodedText, type CodedSegment } from "./codedText";
import type { SpanInfo } from "../../types/span";

/**
 * Parse `(codedText, spans)` into a Run sequence. Every coded
 * marker becomes one typed Run (PcOpen / PcClose / Placeholder)
 * carrying the corresponding span's `equiv_text` as the run's
 * `equiv` field — that's what `flattenRuns` reads when producing
 * the inline form for ICU strings or `__t`/`__tx` fallbacks.
 */
export function codedToRuns(codedText: string, spans: SpanInfo[]): Run[] {
  const segments = parseCodedSegments(codedText, spans);
  return segmentsToRuns(segments);
}

/**
 * Inverse of `codedToRuns`. Walks a Run sequence and produces a
 * coded text string + parallel spans array. Plural / select wrapper
 * runs are intentionally rejected (`throw`) — callers should split
 * those into per-form Run sequences before round-tripping, since
 * coded text is a flat single-form representation.
 */
export function runsToCoded(runs: readonly Run[]): { codedText: string; spans: SpanInfo[] } {
  const segments: CodedSegment[] = [];
  for (const r of runs) {
    if ("text" in r) {
      segments.push({ type: "text", value: r.text });
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
      // Subblock references aren't a chip-editor concept — they
      // represent embedded sub-documents (HTML inside a YAML field,
      // etc.) and stay opaque to the in-cell translator. Render as
      // a placeholder marker keyed on equiv so the coded round-trip
      // still survives.
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
    if ("plural" in r || "select" in r) {
      throw new Error(
        "runsToCoded: plural/select runs cannot be flattened to coded text. " +
          "Call runsToCoded on each form's content individually.",
      );
    }
  }
  return segmentsToCodedText(segments);
}

// ─── Internals ───────────────────────────────────────────────────

function segmentsToRuns(segments: readonly CodedSegment[]): Run[] {
  const out: Run[] = [];
  for (const seg of segments) {
    if (seg.type === "text") {
      out.push({ text: seg.value });
      continue;
    }
    const span = seg.spanInfo;
    const equiv = span.equiv_text || span.id;
    switch (span.span_type) {
      case "opening":
        out.push({
          pcOpen: {
            id: span.id,
            type: span.type,
            ...(span.sub_type ? { subType: span.sub_type } : {}),
            data: span.data,
            equiv,
            ...(span.display_text ? { disp: span.display_text } : {}),
          },
        });
        break;
      case "closing":
        out.push({
          pcClose: {
            id: span.id,
            type: span.type,
            ...(span.sub_type ? { subType: span.sub_type } : {}),
            data: span.data,
            equiv,
          },
        });
        break;
      case "placeholder":
      default:
        out.push({
          ph: {
            id: span.id,
            type: span.type,
            ...(span.sub_type ? { subType: span.sub_type } : {}),
            data: span.data,
            equiv,
            ...(span.display_text ? { disp: span.display_text } : {}),
          },
        });
    }
  }
  return out;
}
