/**
 * Change sets (kapi.change/v1) a review client sends: a decision on a
 * translation, and a correction approved in the same set. Every operation names
 * the revision the reviewer read in `if_match`, so the server refuses it when
 * the translation moved after the card was loaded.
 */

import {
  CHANGE_SCHEMA_ID,
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
  type SetContentOp,
} from "@neokapi/contract-types";
import { projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";

import type { DeckCard, FixReason } from "./types.ts";

/** The translation a card shows: its block's edition in the card's locale. */
export function cardRef(card: Pick<DeckCard, "item" | "blockId" | "locale">): ChangeRef {
  return { doc: card.item, block: card.blockId, edition: card.locale };
}

/**
 * Runs as a change set carries them: each code by its id, type and attributes,
 * without its native data, which the format keeps. Every kind maps one to one.
 */
export function toChangeRuns(runs: readonly Run[]): ChangeRun[] {
  return projectRuns(runs, CHANGE_RUNS);
}

const CHANGE_RUNS: ModelRunSpec<ChangeRun> = {
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
    // A run the client cannot name would leave the correction short, so the
    // change is not built at all.
    throw new Error(`a "${kind}" run cannot be sent in a change: ${why}`);
  },
};

/** A decision on the translation a card shows. */
export function decideOp(
  card: DeckCard,
  outcome: DecideOutcome,
  reasons?: readonly string[],
): DecideOp {
  const op: DecideOp = {
    op: "decide",
    at: cardRef(card),
    if_match: card.targetRevision,
    outcome,
  };
  if (card.sourceRevision) (op as DecideOp & { basis?: string }).basis = card.sourceRevision;
  if (reasons && reasons.length > 0) op.reasons = [...reasons];
  return op;
}

/** Replace the translation a card shows with corrected runs. */
export function setContentOp(card: DeckCard, runs: readonly Run[]): SetContentOp {
  const op: SetContentOp = {
    op: "set_content",
    at: cardRef(card),
    if_match: card.targetRevision,
    runs: toChangeRuns(runs),
  };
  if (card.sourceRevision) op.basis = card.sourceRevision;
  return op;
}

/**
 * A change set of ops. When the cards carry the source revision, the set asks
 * the server to check each operation's `basis`, so a decision on a source that
 * moved after the card was read is refused as stale.
 */
function changeSet(ops: ChangeOp[], card: DeckCard, note?: string): ChangeSet {
  const set: ChangeSet = { schema: CHANGE_SCHEMA_ID, ops };
  if (card.sourceRevision) set.require_basis = true;
  if (note) set.note = note;
  return set;
}

/** Approve the translation as the card shows it. */
export function approveSet(card: DeckCard): ChangeSet {
  return changeSet([decideOp(card, "establish")], card);
}

/** Reject the translation, with the reasons and an optional note the next draft reads. */
export function rejectSet(card: DeckCard, reasons: readonly FixReason[], note?: string): ChangeSet {
  return changeSet([decideOp(card, "reject", reasons)], card, note);
}

/**
 * Correct the translation and approve the correction in one set. Both
 * operations name the revision the card showed: the server checks each against
 * the edition as the set found it.
 */
export function fixSet(
  card: DeckCard,
  runs: readonly Run[],
  reasons: readonly FixReason[],
  note?: string,
): ChangeSet {
  return changeSet([setContentOp(card, runs), decideOp(card, "establish", reasons)], card, note);
}

/**
 * Withdraw a decision that already landed, on the revision the decision left:
 * the undo of a sent approval or rejection. The translation returns to review.
 */
export function withdrawSet(card: DeckCard, revision: string): ChangeSet {
  const op: DecideOp = { op: "decide", at: cardRef(card), if_match: revision, outcome: "withdraw" };
  return { schema: CHANGE_SCHEMA_ID, ops: [op] };
}

/** What became of a change set. */
export type ChangeOutcome =
  /** It landed. `revision` is the revision the last operation left. */
  | { status: "applied"; result: ChangeResult; revision?: string }
  /**
   * Part of it landed: under a `block` separation-of-duties policy a
   * correction is kept and its approval refused, so the block waits for a
   * second reviewer.
   */
  | { status: "held"; result: ChangeResult; error?: ChangeError }
  /** The translation or its source moved after the card was read. */
  | {
      status: "stale";
      result: ChangeResult;
      op: OpResult;
      current?: CurrentEdition;
      field?: string;
    }
  | { status: "refused"; result: ChangeResult; error: ChangeError };

/** Read a change set's result into the outcome a review surface acts on. */
export function readOutcome(result: ChangeResult): ChangeOutcome {
  const refused = result.ops.filter((op) => op.status === "refused");
  if (result.status === "partial") {
    return { status: "held", result, error: refused.find((op) => op.error)?.error };
  }
  if (result.status !== "refused") {
    const last = result.ops.length > 0 ? result.ops[result.ops.length - 1] : undefined;
    return { status: "applied", result, revision: last?.after };
  }
  const stale = refused.find((op) => op.error?.code === "stale");
  if (stale) {
    return {
      status: "stale",
      result,
      op: stale,
      current: stale.current,
      field: stale.error?.field,
    };
  }
  const error = result.error ??
    refused.find((op) => op.error)?.error ?? { code: "invalid", message: "the change was refused" };
  return { status: "refused", result, error };
}
