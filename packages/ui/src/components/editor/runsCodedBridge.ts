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
 *   placeholder, type 'sub'  ↔  SubRun            ([equiv], ref in sub_type)
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

import { projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";

import {
  CHIP_SEGMENTS,
  parseCodedSegments,
  segmentsToCodedText,
  type CodedSegment,
} from "./codedText";
import type { SpanInfo } from "../../types/span";

/**
 * Parse `(codedText, spans)` into a Run sequence. Every coded
 * marker becomes one typed Run (PcOpen / PcClose / Placeholder / Sub)
 * carrying the corresponding span's `equiv_text` as the run's
 * `equiv` field — that's what `flattenRuns` reads when producing
 * the inline form for ICU strings or `__t`/`__tx` fallbacks.
 */
export function codedToRuns(codedText: string, spans: SpanInfo[]): Run[] {
  const segments = parseCodedSegments(codedText, spans);
  return segmentsToRuns(segments);
}

/**
 * The flat editor's reading of a run: every inline code is a chip, and a
 * plural or select wrapper is refused, because coded text is a single-form
 * representation. Callers split a plural into per-form sequences first.
 */
const FLAT_SEGMENTS: ModelRunSpec<CodedSegment> = {
  ...CHIP_SEGMENTS,
  plural: () => {
    throw new Error(
      "runsToCoded: plural/select runs cannot be flattened to coded text. " +
        "Call runsToCoded on each form's content individually.",
    );
  },
  select: () => {
    throw new Error(
      "runsToCoded: plural/select runs cannot be flattened to coded text. " +
        "Call runsToCoded on each form's content individually.",
    );
  },
};

/**
 * Inverse of `codedToRuns`. Projects a Run sequence through
 * {@link FLAT_SEGMENTS} into a coded text string + parallel spans array.
 * Plural / select wrapper runs throw.
 */
export function runsToCoded(runs: readonly Run[]): { codedText: string; spans: SpanInfo[] } {
  return segmentsToCodedText(projectRuns(runs, FLAT_SEGMENTS));
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
        if (span.type === "sub") {
          // A subblock reference chip carries the referenced block in
          // `sub_type` (see CHIP_SEGMENTS), so it returns as the sub run it was.
          out.push({ sub: { id: span.id, ref: span.sub_type ?? "", equiv } });
          break;
        }
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
