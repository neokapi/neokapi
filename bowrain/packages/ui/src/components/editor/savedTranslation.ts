import { flattenRuns, projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";
import { placeholderText } from "../../api/contentChanges";
import type { BlockInfo, TargetStatus } from "../../types/api";
import type { UnifiedSaveResult } from "../UnifiedTargetEditor";
import { getTargetStatus, getTargetText, withTargetEntry, withTargetRevision } from "./blockStatus";

/**
 * A translation a person saves, in each shape a surface needs: the runs the
 * change carries (codes and plural forms kept), the text a block shows until
 * it is read back, and the wording the stale prompt shows as theirs.
 */
export interface SavedTranslation {
  runs: Run[];
  text: string;
  mine: string;
}

/**
 * The text a block shows for saved runs until it is read back: the text runs,
 * each inline code contributing nothing, and a plural or select as its ICU
 * spelling, the one flat form that keeps every branch.
 */
const SHOWN_TEXT: ModelRunSpec<string> = {
  text: (r) => r.text,
  ph: { dropped: "the text a block shows carries no inline codes; the runs keep them" },
  pcOpen: { dropped: "the text a block shows carries no inline codes; the runs keep them" },
  pcClose: { dropped: "the text a block shows carries no inline codes; the runs keep them" },
  sub: { dropped: "the text a block shows carries no inline codes; the runs keep them" },
  plural: (r) => flattenRuns([r]),
  select: (r) => flattenRuns([r]),
  fallback: () => "",
};

/** What the target editor's result saves. */
export function savedTranslation(result: UnifiedSaveResult): SavedTranslation {
  return savedRuns(result.runs);
}

/** What a run sequence saves, from the editor or built outside it (a match, a term insert). */
export function savedRuns(runs: Run[]): SavedTranslation {
  return {
    runs,
    text: projectRuns(runs, SHOWN_TEXT).join(""),
    mine: placeholderText(runs),
  };
}

/**
 * The per-locale status a target carries after a person saves it for `locale`.
 * A person who changes a translation makes it translated, whatever rung it
 * stood on: a review decision judged the wording it replaced, and a draft a
 * person has rewritten is their translation (change.Consequences). Re-saving
 * identical content changes nothing and keeps the status. The optimistic write
 * mirrors that so the chip matches what a reload fetches.
 *
 * Two translations are the same when their runs read the same: the text and
 * each code by its id, in order. An edit that only moves or retypes a code is
 * seen as the change it is; a block served without runs compares its text.
 */
export function statusAfterEdit(block: BlockInfo, locale: string, saved: SavedTranslation) {
  const prev = getTargetStatus(block, locale);
  const prevRuns = block.targets_runs?.[locale];
  const changed =
    prevRuns && prevRuns.length > 0
      ? placeholderText(prevRuns as Run[]) !== placeholderText(saved.runs)
      : saved.text !== getTargetText(block, locale);
  return (changed ? "translated" : prev) satisfies TargetStatus;
}

/**
 * The block as a reload would fetch it after the save landed at revision
 * `after`: the translation's text and runs, and the status the change service
 * gives a person's edit (see `statusAfterEdit`).
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
      status: statusAfterEdit(block, locale, saved),
    }),
    locale,
    after,
  );
  return {
    ...next,
    targets_runs: {
      ...block.targets_runs,
      [locale]: saved.runs as NonNullable<BlockInfo["targets_runs"]>[string],
    },
  };
}
