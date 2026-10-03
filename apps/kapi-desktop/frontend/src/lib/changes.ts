/**
 * The change service as Kapi Desktop reaches it: the four bindings (Read,
 * Apply, Describe, History) that take and return the change contract's JSON as
 * strings, typed with @neokapi/contract-types.
 *
 * Every edit the desktop makes is a change set sent with the revision the
 * surface rendered. A surface reads the block it shows, copies the read's `ref`
 * into an operation's `at` and its `rev` into `if_match`, and reads the result:
 * applied, or refused with a reason. A refusal for a revision that moved
 * (`stale`) carries the edition as it now stands, so the surface can show it
 * and ask before applying again.
 */

import type {
  BlockRead,
  ChangeOp,
  ChangeRef,
  ChangeResult,
  ChangeSet,
  CurrentEdition,
  DescribeRequest,
  EditionHistory,
  FormatDescription,
  HistoryRequest,
  OpResult,
  ReadPage,
  ReadRequest,
  RunPath,
} from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";
import { editTextToSegments } from "@neokapi/ui-primitives";

import { api } from "../hooks/useApi";

/** What a surface calls to read and change content. Storybook and tests pass a fake. */
export interface ChangeClient {
  read(request: ReadRequest): Promise<ReadPage | null>;
  apply(set: ChangeSet): Promise<ChangeResult | null>;
  describe(request: DescribeRequest): Promise<FormatDescription | null>;
  history(request: HistoryRequest): Promise<EditionHistory | null>;
}

/** Parse a binding's JSON answer, or null outside the app (Storybook, vitest). */
function parse<T>(raw: string | null): T | null {
  return raw ? (JSON.parse(raw) as T) : null;
}

/** The change service of the project a tab has open. */
export function tabChanges(tabID: string): ChangeClient {
  return {
    read: async (request) => parse<ReadPage>(await api.readBlocks(tabID, JSON.stringify(request))),
    apply: async (set) => parse<ChangeResult>(await api.applyChanges(tabID, JSON.stringify(set))),
    describe: async (request) =>
      parse<FormatDescription>(await api.describeFormat(tabID, JSON.stringify(request))),
    history: async (request) =>
      parse<EditionHistory>(await api.blockHistory(tabID, JSON.stringify(request))),
  };
}

/** A path as the change service names a document: project-relative, forward slashes. */
export function docPath(path: string): string {
  return path.replace(/\\/g, "/");
}

/**
 * Read one block of a document. `doc` may be the file of one edition, whose
 * read names that edition (a translation's file reads its translation).
 * `editions` names editions in files of their own to show beside it.
 */
export async function readBlock(
  client: ChangeClient,
  doc: string,
  block: string,
  editions?: string[],
): Promise<BlockRead | null> {
  const page = await client.read({
    doc: docPath(doc),
    blocks: [block],
    ...(editions && editions.length > 0 ? { editions } : {}),
  });
  return page?.blocks[0] ?? null;
}

/** The content of one edition as a surface edits it: its text in placeholder form, its codes, its plurals. */
export interface EditionContent {
  /** The edition, canonical, to copy into an operation's `at`. */
  ref: ChangeRef;
  /** The revision the surface rendered, to send as `if_match`. */
  rev: string;
  text: string;
  codes: BlockRead["codes"];
  structures: BlockRead["structures"];
}

/**
 * The content of an edition from a read: the edition the read was opened on,
 * or, named by `edition`, another edition the block holds, with its own
 * plurals and selects, and its own codes where the read lists them (a
 * translation otherwise keeps the codes of what it translates).
 */
export function editionContent(read: BlockRead, edition?: string): EditionContent | null {
  if (!edition || edition === read.ref.edition) {
    return {
      ref: read.ref,
      rev: read.rev,
      text: read.text,
      codes: read.codes,
      structures: read.structures,
    };
  }
  const other = read.editions?.[edition];
  if (!other) return null;
  return {
    ref: { ...read.ref, edition },
    rev: other.rev,
    text: other.text,
    codes: other.codes ?? read.codes,
    structures: other.structures,
  };
}

/** One change a person made in an editor: the whole edition, or one branch of a plural. */
export interface EditionEdit {
  /** The plural form or select case, by path; absent for the whole edition. */
  path?: RunPath;
  /** The new content in placeholder form. */
  text: string;
}

/** The set_content operations that write edits to an edition read at `rev`. */
export function setContentOps(
  content: Pick<EditionContent, "ref" | "rev">,
  edits: EditionEdit[],
): ChangeOp[] {
  return edits.map((e) => ({
    op: "set_content" as const,
    at: content.ref,
    if_match: content.rev,
    text: e.text,
    ...(e.path ? { path: e.path } : {}),
  }));
}

/**
 * A decide operation on the edition read at `rev`. An edition read with no
 * content in its home (a parked draft the project store holds) is named
 * `absent`: the decision binds to the draft while the home still holds
 * nothing for it, and is refused as stale, with the text, once it does.
 */
export function decideOp(ref: ChangeRef, rev: string, outcome: "establish" | "reject"): ChangeOp {
  return { op: "decide", at: ref, if_match: rev, outcome };
}

/** The operation results of a change set that did not land. */
export function refusals(res: ChangeResult): OpResult[] {
  return res.ops.filter((o) => o.status === "refused");
}

/** The first refusal for a revision that moved, with the edition as it now stands. */
export function staleRefusal(res: ChangeResult): { op: OpResult; current: CurrentEdition } | null {
  for (const o of res.ops) {
    if (o.status === "refused" && o.error?.code === "stale" && o.current) {
      return { op: o, current: o.current };
    }
  }
  return null;
}

/** Why a change set did not land, in a sentence a person reads. */
export function refusalMessage(res: ChangeResult): string {
  if (res.error) return res.error.message;
  const first = refusals(res)[0];
  if (first?.error) {
    switch (first.error.code) {
      case "gate_failed":
        return t("A rule in force fails on this wording: {message}", {
          message: first.error.message,
        });
      default:
        return first.error.message;
    }
  }
  if (res.status === "partial") return t("Only part of the change was written.");
  return t("The change was not applied.");
}

/** A change set that did not land. */
export class ChangeRefused extends Error {
  readonly result: ChangeResult;
  constructor(result: ChangeResult) {
    super(refusalMessage(result));
    this.name = "ChangeRefused";
    this.result = result;
  }
}

/**
 * Apply a change set and return its result. A result that applied (or changed
 * nothing because the content already said it) resolves; anything else throws
 * ChangeRefused, carrying the result for a caller that reads a stale refusal.
 * Outside the app the client answers nothing and the call resolves to null.
 */
export async function applyOrThrow(
  client: ChangeClient,
  set: ChangeSet,
): Promise<ChangeResult | null> {
  const res = await client.apply(set);
  if (!res) return null;
  if (res.status !== "applied") throw new ChangeRefused(res);
  return res;
}

/**
 * The same operations sent again over the edition as it now stands: each
 * operation on the edition the stale refusal names takes the current revision,
 * and `rebase` may rewrite an operation further (a check's fix finds its words
 * again rather than trusting a run range in text that moved).
 */
export function rebaseOps(
  ops: ChangeOp[],
  stale: { op: OpResult; current: CurrentEdition },
  rebase?: (op: ChangeOp) => ChangeOp,
): ChangeOp[] {
  const at = stale.op.at;
  return ops.map((op) => {
    if (!("at" in op) || !at || !sameEdition(op.at, at)) return op;
    const next = { ...op, if_match: stale.current.rev } as ChangeOp;
    return rebase ? rebase(next) : next;
  });
}

function sameEdition(
  a: { doc: string; block?: string; edition?: string },
  b: { doc: string; block?: string; edition?: string },
): boolean {
  return a.doc === b.doc && a.block === b.block && (a.edition ?? "") === (b.edition ?? "");
}

/** How many times words occur in text, matches not overlapping. */
function occurrences(text: string, words: string): number {
  let n = 0;
  for (let at = text.indexOf(words); at >= 0; at = text.indexOf(words, at + words.length)) n++;
  return n;
}

/**
 * Whether words occur in an edit text with no inline code among them, every
 * time they occur, as the change service finds them (codes have no width). A
 * check's fix replaces its words with plain text, which keeps a code only at
 * either end of them; words found again across a link or a bold span would
 * take it with them.
 */
export function wordsInPlainText(text: string, words: string): boolean {
  if (!words) return false;
  const segments = editTextToSegments(text);
  let flat = "";
  let inText = 0;
  for (const seg of segments) {
    if (seg.type !== "text") continue;
    flat += seg.value;
    inText += occurrences(seg.value, words);
  }
  return inText > 0 && inText === occurrences(flat, words);
}

/** The editions a source edit left on an older basis, as their keys. */
export function invalidatedEditions(res: ChangeResult): string[] {
  const out = new Set<string>();
  for (const o of res.ops) for (const inv of o.invalidates ?? []) out.add(inv.edition);
  return [...out].sort();
}
