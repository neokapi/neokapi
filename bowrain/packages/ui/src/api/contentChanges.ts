/**
 * Content changes: what an editor surface sends when a person saves a
 * translation, decides on one, writes a note or marks an entity. Each is an
 * operation of a change set (kapi.change/v1, typed by @neokapi/contract-types)
 * sent to the stream's changes route. An operation on a translation names the
 * revision the surface rendered (`if_match`), so the server refuses it when
 * someone changed the translation in the meantime, and the refusal carries the
 * translation as it now stands.
 */

import {
  CHANGE_ERROR_HTTP_STATUS,
  CHANGE_SCHEMA_ID,
  type AnnotateOp,
  type ChangeAnchor,
  type ChangeError,
  type ChangeOp,
  type ChangeRef,
  type ChangeResult,
  type ChangeRun,
  type ChangeSet,
  type CurrentEdition,
  type DecideOp,
  type DecideOutcome,
  type OpResult,
  type RunPos,
  type SetContentOp,
  type UnannotateOp,
} from "@neokapi/contract-types";
import { otherBranch, projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";
import type { BlockInfo, ReviewRung } from "../types/api";

/** A change set of content operations (kapi.change/v1). */
export type ContentChangeSet = ChangeSet;
export type { ChangeResult };

/** The revision an edition holds when it does not exist. */
export const ABSENT_REVISION = "absent";

/**
 * The revision of a block's translation in one language as the surface rendered
 * it: what `if_match` names. A block served without its revisions has none to
 * name for a translation it holds, and an edit then has nothing to guard it, so
 * that is an error rather than a guess.
 */
export function renderedRevision(block: BlockInfo, locale: string): string {
  const rev = block.target_revisions?.[locale];
  if (rev) return rev;
  if (block.targets?.[locale] == null) return ABSENT_REVISION;
  throw new Error(
    `block ${block.id} was read without the revision of its ${locale} translation; reload it before changing it`,
  );
}

/** A block's translation in one language, in an item. */
export function translationRef(item: string, blockId: string, locale: string): ChangeRef {
  return { doc: item, block: blockId, edition: locale };
}

/** A block's source, in an item: the block's own edition. */
export function sourceRef(item: string, blockId: string): ChangeRef {
  return { doc: item, block: blockId };
}

/**
 * The runs a change set carries: the block's runs with every code named by its
 * id and type and none of its native data, which the format keeps and spells
 * again from the type when it writes the document.
 */
export function toChangeRuns(runs: readonly Run[]): ChangeRun[] {
  return projectRuns(runs, CHANGE_RUN_SPEC);
}

const CHANGE_RUN_SPEC: ModelRunSpec<ChangeRun> = {
  text: (r) => (r.noTranslate ? { text: r.text, noTranslate: true } : { text: r.text }),
  ph: ({ ph }) => ({
    ph: {
      id: ph.id,
      type: ph.type,
      subType: ph.subType,
      equiv: ph.equiv,
      disp: ph.disp,
      attrs: ph.attrs,
      constraints: ph.constraints,
    },
  }),
  pcOpen: ({ pcOpen }) => ({
    pcOpen: {
      id: pcOpen.id,
      type: pcOpen.type,
      subType: pcOpen.subType,
      equiv: pcOpen.equiv,
      disp: pcOpen.disp,
      attrs: pcOpen.attrs,
      constraints: pcOpen.constraints,
    },
  }),
  pcClose: ({ pcClose }) => ({
    pcClose: { id: pcClose.id, type: pcClose.type, subType: pcClose.subType, equiv: pcClose.equiv },
  }),
  sub: ({ sub }) => ({ sub: { id: sub.id, ref: sub.ref, equiv: sub.equiv } }),
  plural: ({ plural }) => ({
    plural: {
      pivot: plural.pivot,
      forms: Object.fromEntries(
        Object.entries(plural.forms).map(([form, runs]) => [form, toChangeRuns(runs ?? [])]),
      ),
    },
  }),
  select: ({ select }) => ({
    select: {
      pivot: select.pivot,
      cases: Object.fromEntries(
        Object.entries(select.cases).map(([key, runs]) => [key, toChangeRuns(runs)]),
      ),
    },
  }),
  fallback: (kind, why) => {
    // A run this build cannot name would leave the change set without it: the
    // edit is not sent rather than sent short.
    throw new Error(`a "${kind}" run cannot be sent in a change: ${why}`);
  },
};

/** What a save carries: the runs the editor produced, or plain text. */
export type TranslationContent = { runs: readonly Run[] } | { text: string };

/** Replace a translation's content, on the revision the surface rendered. */
export function setTranslation(
  item: string,
  block: BlockInfo,
  locale: string,
  content: TranslationContent,
  ifMatch: string = renderedRevision(block, locale),
): SetContentOp {
  const at = translationRef(item, block.id, locale);
  return "runs" in content
    ? { op: "set_content", at, if_match: ifMatch, runs: toChangeRuns(content.runs) }
    : { op: "set_content", at, if_match: ifMatch, text: content.text };
}

/** A review decision on a translation, on the revision the reviewer read. */
export function decideTranslation(
  item: string,
  block: BlockInfo,
  locale: string,
  outcome: DecideOutcome,
  ifMatch: string = renderedRevision(block, locale),
): DecideOp {
  return { op: "decide", at: translationRef(item, block.id, locale), if_match: ifMatch, outcome };
}

/**
 * The decision a review control makes: an approval establishes the
 * translation; a clearing call withdraws it to translated, or rejects it to
 * draft (`rung` "draft"), so it re-enters the work queue.
 */
export function decisionOutcome(
  reviewed: boolean,
  rung?: ReviewRung,
): Exclude<DecideOutcome, "advise"> {
  if (reviewed) return "establish";
  return rung === "draft" ? "reject" : "withdraw";
}

/** The annotation type a person's note on a block is. */
export const NOTE_ANNOTATION = "note";

/** A note on a block. Notes sit on the source; the server stamps the author. */
export function addNote(item: string, blockId: string, text: string): AnnotateOp {
  return { op: "annotate", at: sourceRef(item, blockId), type: NOTE_ANNOTATION, value: { text } };
}

/** Remove a note from a block. */
export function removeNote(item: string, blockId: string, noteId: string): UnannotateOp {
  return { op: "unannotate", at: sourceRef(item, blockId), type: NOTE_ANNOTATION, id: noteId };
}

/** The annotation type an entity marked on a block's source is. */
export const ENTITY_ANNOTATION = "entity";

/** An entity a person marks in a block's source. */
export interface EntityMark {
  text: string;
  type: string;
  /** Where the entity starts and ends in the block's source text (`block.source`). */
  start: number;
  end: number;
  dnt: boolean;
  source?: string;
  locale?: string;
}

/**
 * Mark an entity in a block's source. The value is the entity annotation the
 * engine registers, field for field, so the server stores it as one.
 */
export function markEntity(item: string, block: BlockInfo, entity: EntityMark): AnnotateOp {
  return {
    op: "annotate",
    at: sourceRef(item, block.id),
    type: ENTITY_ANNOTATION,
    anchor: textRangeAnchor(sourceRuns(block), block.source, entity.start, entity.end),
    value: {
      Text: entity.text,
      Type: entity.type,
      Locale: entity.locale ?? "",
      DNT: entity.dnt,
      Source: entity.source ?? "manual",
    },
  };
}

/** A block's source as runs: the typed runs, or its text as one run. */
function sourceRuns(block: BlockInfo): readonly Run[] {
  if (block.source_runs && block.source_runs.length > 0) return block.source_runs as Run[];
  return [{ text: block.source }];
}

/**
 * The anchor of a span of a block's source text, from offsets into the text a
 * surface shows (`block.source`, string indexes) to run positions: a run index
 * and a code-point offset into that run, counted the way the engine flattens a
 * run sequence (model.RangeAnchor), so the anchor resolves to the same words.
 */
export function textRangeAnchor(
  runs: readonly Run[],
  text: string,
  start: number,
  end: number,
): ChangeAnchor {
  const codePoints = (s: string) => Array.from(s).length;
  const lengths = projectRuns(runs, FLAT_LENGTH_SPEC);
  const position = (offset: number): RunPos => {
    if (offset <= 0) return { run: 0, offset: 0 };
    let pos = 0;
    for (let i = 0; i < lengths.length; i++) {
      const l = lengths[i];
      if (l === 0) continue;
      if (offset < pos + l) return { run: i, offset: offset - pos };
      if (offset === pos + l) return { run: i + 1, offset: 0 };
      pos += l;
    }
    return { run: lengths.length, offset: 0 };
  };
  return {
    kind: "range",
    start: position(codePoints(text.slice(0, start))),
    end: position(codePoints(text.slice(0, end))),
  };
}

const flatLength = (runs: readonly Run[]): number =>
  projectRuns(runs, FLAT_LENGTH_SPEC).reduce((a, b) => a + b, 0);

/**
 * How much of the flattened text each run is, in code points. A code holds no
 * text; a plural or select reads as its `other` branch, as the engine reads it.
 * Every kind answers with a number, so the lengths stay aligned with the runs.
 */
const FLAT_LENGTH_SPEC: ModelRunSpec<number> = {
  text: (r) => Array.from(r.text).length,
  ph: () => 0,
  pcOpen: () => 0,
  pcClose: () => 0,
  sub: () => 0,
  plural: ({ plural }) => flatLength(otherBranch(plural.forms as Record<string, Run[]>)),
  select: ({ select }) => flatLength(otherBranch(select.cases)),
  fallback: () => 0,
};

/**
 * Runs in the placeholder form a read shows (the form a stale refusal's
 * `current.text` comes in): text as it is, and each inline code as an
 * `<x id="…"/>` placeholder. A plural or select reads as its `other` branch.
 */
export function placeholderText(runs: readonly Run[]): string {
  return projectRuns(runs, PLACEHOLDER_SPEC).join("");
}

const PLACEHOLDER_SPEC: ModelRunSpec<string> = {
  text: (r) => r.text,
  ph: ({ ph }) => `<x id="${ph.id}/"/>`,
  pcOpen: ({ pcOpen }) => `<x id="${pcOpen.id}"/>`,
  pcClose: ({ pcClose }) => `<x id="/${pcClose.id}"/>`,
  sub: ({ sub }) => `<x id="sub:${sub.id}"/>`,
  plural: ({ plural }) => placeholderText(otherBranch(plural.forms as Record<string, Run[]>)),
  select: ({ select }) => placeholderText(otherBranch(select.cases)),
  fallback: () => "",
};

/** A change set of ops, with an optional note a person reads in history. */
export function contentChangeSet(ops: ChangeOp[], note?: string): ContentChangeSet {
  return note ? { schema: CHANGE_SCHEMA_ID, note, ops } : { schema: CHANGE_SCHEMA_ID, ops };
}

/** What became of a change set, read from its result. */
export type ChangeOutcome =
  | { status: "applied"; result: ChangeResult }
  | { status: "stale"; result: ChangeResult; op: OpResult; current: CurrentEdition }
  | { status: "refused"; result: ChangeResult; error: ChangeError };

/**
 * Read a change set's result: applied (or previewed), stale on an operation
 * whose edition moved (with the edition as it stands), or refused for any
 * other reason (with the first refusal).
 */
export function readOutcome(result: ChangeResult): ChangeOutcome {
  if (result.status !== "refused") return { status: "applied", result };
  if (result.error) return { status: "refused", result, error: result.error };
  const refused = result.ops.filter((op) => op.status === "refused");
  const stale = refused.find((op) => op.error?.code === "stale" && op.current);
  if (stale?.current) return { status: "stale", result, op: stale, current: stale.current };
  const error = refused.find((op) => op.error)?.error ?? {
    code: "invalid",
    message: "the change was refused",
  };
  return { status: "refused", result, error };
}

/** The revision an operation left its edition at, when it says. */
export function revisionAfter(result: ChangeResult, index = 0): string | undefined {
  return result.ops.find((op) => op.i === index)?.after;
}

/**
 * A change set the server refused for a reason other than a stale revision. Its
 * message is the refusal's, which a surface shows as the error's cause, and its
 * status is the one the refusal's code is answered with.
 */
export class ChangeRefusedError extends Error {
  readonly result: ChangeResult;
  readonly error: ChangeError;
  readonly status: number;

  constructor(result: ChangeResult, error: ChangeError) {
    super(error.message);
    this.name = "ChangeRefusedError";
    this.result = result;
    this.error = error;
    this.status = CHANGE_ERROR_HTTP_STATUS[error.code];
  }
}
