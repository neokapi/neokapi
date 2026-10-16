import type { Run } from "@neokapi/kapi-format";
import {
  codedToRuns,
  runsToSegments,
  segmentsToCodedText,
  type SpanInfo,
} from "@neokapi/ui-primitives";

/**
 * The one seam between a block's runs and the inline-code editor.
 *
 * The editor's contenteditable holds text with one marker per chip, beside the
 * chips in order; that is the shape `InlineCodeEditor` takes and gives back.
 * `toEditorText` projects runs into it through the primitives' declared chip
 * projection (`runsToSegments`, a RunSpec over every run kind), and
 * `fromEditorText` reads an edit back as runs. Nothing else in this package
 * holds coded text: a surface keeps `Run[]` and crosses here at the editor.
 *
 * A plural or select inside a flat sequence reads as its ICU spelling and comes
 * back as text; a translation that is one plural never comes through here,
 * because `UnifiedTargetEditor` opens one editor per form.
 */
export interface EditorText {
  codedText: string;
  spans: SpanInfo[];
}

/** The editor's text and chips for a run sequence. */
export function toEditorText(runs: readonly Run[] | undefined): EditorText {
  return segmentsToCodedText(runsToSegments(runs ?? []));
}

/** The runs an edited editor text stands for. */
export function fromEditorText(codedText: string, spans: readonly SpanInfo[]): Run[] {
  return codedToRuns(codedText, [...spans]);
}

/**
 * The chips of a run sequence, in order: what the tag palette offers, the
 * pair map joins and the tag validator counts.
 */
export function codeSpans(runs: readonly Run[] | undefined): SpanInfo[] {
  return toEditorText(runs).spans;
}
