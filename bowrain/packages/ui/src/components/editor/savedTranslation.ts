import { flattenRuns, type Run } from "@neokapi/kapi-format";
import { codedToRuns, runsToCoded } from "@neokapi/ui-primitives";
import { placeholderText } from "../../api/contentChanges";
import type { BlockInfo } from "../../types/api";
import type { UnifiedSaveResult } from "../UnifiedTargetEditor";
import { statusAfterEdit, withTargetEntry, withTargetRevision } from "./blockStatus";

/**
 * A translation a person saves, in each shape a surface needs: the runs the
 * change carries (codes and plural forms kept), the text a block shows until
 * it is read back, the coded text of a flat translation, and the wording the
 * stale prompt shows as theirs.
 */
export interface SavedTranslation {
  runs: Run[];
  text: string;
  /** The editor's coded text; absent for a plural, which coded text cannot hold. */
  coded?: string;
  mine: string;
}

/** Inline-code markers in coded text: the plain text is the coded text without them. */
const CODE_MARKERS = /[-]/g;

/** What the target editor's result saves. */
export function savedTranslation(result: UnifiedSaveResult): SavedTranslation {
  if (result.kind === "plural") {
    return { runs: result.runs, text: result.text, mine: result.text };
  }
  const runs = codedToRuns(result.codedText, result.spans) as Run[];
  return {
    runs,
    text: result.codedText.replace(CODE_MARKERS, ""),
    coded: result.codedText,
    mine: placeholderText(runs),
  };
}

/** What a run sequence built outside the editor (a match, a term insert) saves. */
export function savedRuns(runs: Run[]): SavedTranslation {
  try {
    const { codedText } = runsToCoded(runs);
    return {
      runs,
      text: codedText.replace(CODE_MARKERS, ""),
      coded: codedText,
      mine: placeholderText(runs),
    };
  } catch {
    // A plural or select has no coded form; it shows as its ICU spelling.
    const text = flattenRuns(runs);
    return { runs, text, mine: text };
  }
}

/**
 * The block as a reload would fetch it after the save landed at revision
 * `after`: the translation's text, runs and coded form, and the status the
 * change service gives a person's edit (see `statusAfterEdit`).
 */
export function withSavedTranslation(
  block: BlockInfo,
  locale: string,
  saved: SavedTranslation,
  after: string | undefined,
): BlockInfo {
  const next = withTargetRevision(
    withTargetEntry(block, locale, {
      text: saved.text,
      status: statusAfterEdit(block, locale, saved.text, saved.coded),
    }),
    locale,
    after,
  );
  return {
    ...next,
    targets_coded: { ...block.targets_coded, [locale]: saved.coded ?? "" },
    targets_runs: {
      ...block.targets_runs,
      [locale]: saved.runs as NonNullable<BlockInfo["targets_runs"]>[string],
    },
  };
}
