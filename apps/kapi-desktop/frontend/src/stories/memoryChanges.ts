/**
 * An in-memory ChangeClient, for Storybook stories and component tests: a few
 * blocks held as a read shows them, a revision per edition computed from its
 * content, and change sets applied with the contract's precondition, so a
 * stale revision is refused with the edition as it stands. Term rules stand in
 * for the commit check: a set_content whose text holds a rule's term is
 * refused gate_failed with a finding, and lands with it under gate report. It
 * is a stand-in for the change service the desktop reaches through its
 * bindings, which the Go tests drive for real.
 */

import type {
  BlockRead,
  ChangeFinding,
  ChangeOp,
  ChangeOpKind,
  ChangeRef,
  ChangeResult,
  ChangeSet,
  CodeRead,
  EditionHistory,
  FormatDescription,
  HistoryEntry,
  OpResult,
  ReadPage,
  StructureRead,
} from "@neokapi/contract-types";

import type { ChangeClient } from "../lib/changes";
import type { ReviewItem } from "../types/api";

export interface MemoryBlock {
  /** The document the block is read from (a translation's own file reads its edition). */
  doc: string;
  block: string;
  /** The edition as the change service names it; {doc, block} when absent. */
  ref?: ChangeRef;
  text: string;
  codes?: Record<string, CodeRead>;
  structures?: StructureRead[];
  /** Other editions the block holds, by key, with their own plurals and selects. */
  editions?: Record<
    string,
    {
      text: string;
      status?: string;
      codes?: Record<string, CodeRead>;
      structures?: StructureRead[];
    }
  >;
  ops?: ChangeOpKind[];
}

/** A word the project's checks fail, and what to say instead. */
export interface MemoryTermRule {
  term: string;
  replacement: string;
}

/** The content of one edition as the client holds it. */
interface MemoryEdition {
  text: string;
  structures?: StructureRead[];
}

/** A 16-hex-digit FNV-1a hash, the shape of a revision. */
function revOf(content: string): string {
  let h1 = 0x811c9dc5;
  let h2 = 0x01000193;
  for (let i = 0; i < content.length; i++) {
    const c = content.charCodeAt(i);
    h1 = Math.imul(h1 ^ c, 0x01000193) >>> 0;
    h2 = Math.imul(h2 ^ c, 0x811c9dc5) >>> 0;
  }
  return `r:${h1.toString(16).padStart(8, "0")}${h2.toString(16).padStart(8, "0")}`;
}

const DEFAULT_OPS: ChangeOpKind[] = ["set_content", "replace_text", "annotate", "unannotate"];

function sameRef(a: ChangeRef, b: { doc: string; block?: string; edition?: string }): boolean {
  return a.doc === b.doc && a.block === b.block && (a.edition ?? "") === (b.edition ?? "");
}

export class MemoryChanges implements ChangeClient {
  /** Every change set applied, in order, refused ones included. */
  readonly sets: ChangeSet[] = [];
  private blocks: MemoryBlock[];
  private log = new Map<string, HistoryEntry[]>();
  private seq = 0;

  private rules: MemoryTermRule[];

  constructor(
    blocks: MemoryBlock[],
    history: Record<string, HistoryEntry[]> = {},
    rules: MemoryTermRule[] = [],
  ) {
    this.blocks = blocks.map((b) => ({ ...b }));
    for (const [k, v] of Object.entries(history)) this.log.set(k, [...v]);
    this.rules = rules;
  }

  /** What the term rules find in the text an operation writes. */
  private findingsOf(op: ChangeOp): ChangeFinding[] {
    if (op.op !== "set_content" || op.text === undefined) return [];
    const text = op.text;
    return this.rules
      .filter((r) => text.includes(r.term))
      .map((r) => ({
        rule: "terms.vocabulary",
        message: `Use \u201c${r.replacement}\u201d instead of \u201c${r.term}\u201d`,
        fails: true,
      }));
  }

  /** The edition's revision now. */
  rev(b: MemoryEdition): string {
    return revOf(JSON.stringify({ text: b.text, structures: b.structures ?? [] }));
  }

  /**
   * The edition an operation names: a block read from its own document, or
   * one listed among a block's editions, addressed by the block's reference
   * and the edition's key.
   */
  private editionAt(at: ChangeRef): MemoryEdition | undefined {
    const own = this.blocks.find((x) => sameRef(this.refOf(x), at));
    if (own) return own;
    if (!at.edition) return undefined;
    const holder = this.blocks.find((x) =>
      sameRef(this.refOf(x), { doc: at.doc, block: at.block }),
    );
    return holder?.editions?.[at.edition];
  }

  private refOf(b: MemoryBlock): ChangeRef {
    return b.ref ?? { doc: b.doc, block: b.block };
  }

  private toRead(b: MemoryBlock): BlockRead {
    const editions: BlockRead["editions"] = {};
    for (const [key, e] of Object.entries(b.editions ?? {})) {
      editions[key] = {
        rev: this.rev(e),
        text: e.text,
        status: e.status,
        stale: false,
        ...(e.codes ? { codes: e.codes } : {}),
        ...(e.structures ? { structures: e.structures } : {}),
      };
    }
    return {
      ref: this.refOf(b),
      rev: this.rev(b),
      text: b.text,
      ...(b.codes ? { codes: b.codes } : {}),
      ...(b.structures ? { structures: b.structures } : {}),
      ...(b.editions ? { editions } : {}),
      ops: b.ops ?? DEFAULT_OPS,
    };
  }

  /** Change a block as another writer would, behind every reader's back. */
  touch(doc: string, block: string, text: string): void {
    const b = this.blocks.find((x) => x.doc === doc && x.block === block);
    if (b) b.text = text;
  }

  async read(request: { doc: string; blocks?: string[] }): Promise<ReadPage> {
    const blocks = this.blocks
      .filter((b) => b.doc === request.doc)
      .filter((b) => !request.blocks?.length || request.blocks.includes(b.block))
      .map((b) => this.toRead(b));
    return { doc: request.doc, home: "memory", format: "memory", head: "h", blocks };
  }

  async describe(): Promise<FormatDescription | null> {
    return null;
  }

  async history(request: { ref: ChangeRef }): Promise<EditionHistory> {
    const b = this.blocks.find((x) => sameRef(this.refOf(x), request.ref));
    return {
      ref: request.ref,
      rev: b ? this.rev(b) : "absent",
      entries: [...(this.log.get(JSON.stringify(request.ref)) ?? [])],
    };
  }

  async apply(set: ChangeSet): Promise<ChangeResult> {
    this.sets.push(set);
    const results: OpResult[] = [];
    const plans: Array<() => void> = [];
    let refused = -1;
    set.ops.forEach((op, i) => {
      const at = "at" in op ? op.at : undefined;
      const b = at ? this.editionAt(at) : undefined;
      const res: OpResult = { i, op: op.op, status: "applied", ...(at ? { at } : {}) };
      results.push(res);
      if (!b) {
        res.status = "refused";
        res.error = { code: "not_found", message: "no such block" };
        if (refused < 0) refused = i;
        return;
      }
      const rev = this.rev(b);
      const ifMatch = "if_match" in op ? op.if_match : undefined;
      if (typeof ifMatch === "string" && ifMatch !== "*" && ifMatch !== rev) {
        res.status = "refused";
        res.error = {
          code: "stale",
          field: "if_match",
          message: `the edition is at ${rev}, not ${ifMatch}`,
        };
        res.current = { rev, text: b.text };
        if (refused < 0) refused = i;
        return;
      }
      res.before = rev;
      const findings = this.findingsOf(op);
      if (findings.length > 0) {
        res.findings = findings;
        if (set.gate !== "report") {
          res.status = "refused";
          res.error = {
            code: "gate_failed",
            message: `the edit introduces ${findings.length} failing finding(s): ${findings[0].message}`,
          };
          if (refused < 0) refused = i;
          return;
        }
      }
      plans.push(() => this.write(b, at!, op, res));
    });
    if (refused >= 0) {
      for (const r of results) {
        if (r.status !== "refused") {
          r.status = "not_applied";
          r.blocked_by = refused;
          delete r.findings;
        }
      }
      return {
        schema: "kapi.change-result/v1",
        status: "refused",
        record: null,
        docs: [],
        ops: results,
      };
    }
    for (const p of plans) p();
    return {
      schema: "kapi.change-result/v1",
      status: "applied",
      record: `op-${this.seq}`,
      docs: [],
      ops: results,
    };
  }

  private write(b: MemoryEdition, ref: ChangeRef, op: ChangeOp, res: OpResult): void {
    if (op.op !== "set_content") {
      res.after = this.rev(b);
      return;
    }
    const before = this.rev(b);
    if (op.path && b.structures) {
      const at = op.path.length - 1;
      const step = op.path[at];
      const prefix = JSON.stringify(op.path.slice(0, at));
      b.structures = b.structures.map((s) => {
        if (JSON.stringify(s.path) !== prefix || typeof step === "number") return s;
        const named = step as { plural?: string; select?: string };
        const name = named.plural ?? named.select ?? "";
        return { ...s, branches: { ...s.branches, [name]: op.text ?? "" } };
      });
    } else if (op.text !== undefined) {
      b.text = op.text;
    }
    const after = this.rev(b);
    res.after = after;
    // A change to the source leaves every translation on an older basis.
    const editions = "editions" in b ? (b as MemoryBlock).editions : undefined;
    if (!ref.edition && editions) {
      res.invalidates = Object.keys(editions)
        .sort()
        .map((edition) => ({ edition, reason: "basis_moved" }));
    }
    this.seq++;
    // A block read from its own document records under the reference its
    // read gives; an edition listed in a block under the one the change named.
    const key = JSON.stringify("doc" in b ? this.refOf(b as MemoryBlock) : ref);
    const entries = this.log.get(key) ?? [];
    entries.unshift({
      record: `op-${this.seq}`,
      before,
      after,
      actor: { kind: "person" },
      origin: "desktop",
      at: new Date(Date.UTC(2026, 9, 3, 9, this.seq)).toISOString(),
    });
    this.log.set(key, entries);
  }
}

/**
 * A change service holding the units of a review queue, each read from the
 * file the queue names: a translation from its own file, naming the edition of
 * its source, and a source unit with the translations the queue lists for it.
 * `extra` adds to or replaces a block, by `${locale}:${key}`, and `history`
 * seeds the recorded changes, by the JSON of an edition's reference.
 */
export function queueChanges(
  items: ReviewItem[],
  extra: Record<string, Partial<MemoryBlock>> = {},
  history: Record<string, HistoryEntry[]> = {},
): MemoryChanges {
  const blocks: MemoryBlock[] = items.map((it) => {
    const sourceLocale = it.sourceLocale ?? "en-US";
    const sourceDoc = it.relative ?? `locales/${sourceLocale}.json`;
    const base: MemoryBlock = it.isSource
      ? {
          doc: it.file,
          block: it.key,
          ref: { doc: it.file, block: it.key },
          text: it.source,
          editions: Object.fromEntries(
            items
              .filter((o) => !o.isSource && o.key === it.key)
              .map((o) => [o.locale, { text: o.target ?? "" }]),
          ),
        }
      : {
          doc: it.file,
          block: it.key,
          ref: { doc: sourceDoc, block: it.key, edition: it.locale },
          text: it.target ?? "",
          editions: { [sourceLocale]: { text: it.source } },
        };
    return { ...base, ...extra[`${it.locale}:${it.key}`] };
  });
  return new MemoryChanges(blocks, history);
}

/** The decide operations a client was sent, in order, with the note each set carried. */
export function decisionsSent(
  changes: MemoryChanges,
): Array<{ outcome: string; at: ChangeRef; note?: string }> {
  const out: Array<{ outcome: string; at: ChangeRef; note?: string }> = [];
  for (const set of changes.sets) {
    for (const op of set.ops) {
      if (op.op === "decide") out.push({ outcome: op.outcome, at: op.at, note: set.note });
    }
  }
  return out;
}
