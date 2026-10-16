import {
  otherBranch,
  parseICUPluralString,
  projectRuns,
  runKindOf,
  type ModelRunSpec,
  type PluralForm,
  type Run,
} from "@neokapi/kapi-format";
import type { BlockInfo } from "../../types/api";
import { getTargetText } from "./blockStatus";

/**
 * A block's content as runs, read one way by every surface.
 *
 * The server serves a block's source and each translation as RFC 0001 runs
 * (`source_runs`, `targets_runs`) beside their text. A surface reads the runs
 * through these helpers and projects them through a declared RunSpec; a block
 * served without runs (an older payload, a fixture) reads its text as one run.
 */

/** The block's source as runs. */
export function sourceRuns(block: BlockInfo): Run[] {
  if (block.source_runs && block.source_runs.length > 0) return block.source_runs as Run[];
  return block.source ? [{ text: block.source }] : [];
}

/** A locale's translation as runs; empty when the block holds none there. */
export function targetRuns(block: BlockInfo, locale: string): Run[] {
  const runs = block.targets_runs?.[locale];
  if (runs && runs.length > 0) return runs as Run[];
  const text = getTargetText(block, locale);
  return text ? [{ text }] : [];
}

export type PluralRun = Extract<Run, { plural: unknown }>;
type TextRun = Extract<Run, { text: string }>;

/** The plural run a sequence consists of, or null when it holds anything else. */
export function soloPlural(runs: readonly Run[] | undefined): PluralRun | null {
  if (!runs || runs.length !== 1) return null;
  return runKindOf(runs[0]) === "plural" ? (runs[0] as PluralRun) : null;
}

/**
 * The plural a sequence is: a plural run on its own, or one text run spelling
 * a plural in ICU, which a format that keeps ICU as text serves. Null for
 * anything else.
 */
export function pluralOf(runs: readonly Run[] | undefined): PluralRun | null {
  const solo = soloPlural(runs);
  if (solo) return solo;
  if (!runs || runs.length !== 1 || runKindOf(runs[0]) !== "text") return null;
  return soloPlural(parseICUPluralString((runs[0] as TextRun).text) ?? undefined);
}

/** The forms of a plural, each as its runs, in CLDR order. */
export function pluralForms(plural: PluralRun): Partial<Record<PluralForm, Run[]>> {
  return plural.plural.forms as Partial<Record<PluralForm, Run[]>>;
}

/** Whether each run holds an inline code, looking inside plural and select branches. */
const HAS_CODE: ModelRunSpec<boolean> = {
  text: () => false,
  ph: () => true,
  pcOpen: () => true,
  pcClose: () => true,
  sub: () => true,
  plural: {
    expand: (r) => Object.values(r.plural.forms).flatMap((form) => projectRuns(form, HAS_CODE)),
  },
  select: {
    expand: (r) => Object.values(r.select.cases).flatMap((c) => projectRuns(c, HAS_CODE)),
  },
  fallback: () => false,
};

/** Whether a run sequence carries an inline code anywhere. */
export function hasInlineCodes(runs: readonly Run[] | undefined): boolean {
  return projectRuns(runs, HAS_CODE).some(Boolean);
}

/** Whether a block's source carries an inline code: the server's flag, else read from its runs. */
export function blockHasInlineCodes(block: BlockInfo): boolean {
  return block.has_inline_codes ?? hasInlineCodes(block.source_runs as Run[] | undefined);
}

/**
 * The plain text of a run sequence as the engine reads it (`model.RunsText`):
 * the text runs, a code contributing nothing, a plural or select its `other`
 * branch. Entity and term positions are stated over this text.
 */
const PLAIN_TEXT: ModelRunSpec<string> = {
  text: (r) => r.text,
  ph: { dropped: "an inline code has no plain text" },
  pcOpen: { dropped: "an inline code has no plain text" },
  pcClose: { dropped: "an inline code has no plain text" },
  sub: { dropped: "a subblock reference has no plain text here" },
  plural: (r) => plainText(otherBranch(r.plural.forms as Record<string, Run[]>)),
  select: (r) => plainText(otherBranch(r.select.cases)),
  fallback: () => "",
};

/** The plain text of a run sequence. */
export function plainText(runs: readonly Run[] | undefined): string {
  return projectRuns(runs, PLAIN_TEXT).join("");
}
