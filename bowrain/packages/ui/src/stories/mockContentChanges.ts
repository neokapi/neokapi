/**
 * The mock adapter's change service: a change set (kapi.change/v1) applied to
 * the stories' in-memory blocks with the server's rules where a surface can see
 * them. Each translation has a revision derived from its content, an operation
 * on a translation is refused stale when its `if_match` names another, the
 * refusal carries the translation as it stands, and a change set lands whole
 * or not at all.
 */

import {
  CHANGE_RESULT_SCHEMA_ID,
  type ChangeFinding,
  type ChangeOp,
  type OpResult,
} from "@neokapi/contract-types";
import { projectRuns, type ModelRunSpec, type Run } from "@neokapi/kapi-format";
import { runsToCoded } from "@neokapi/ui-primitives";
import { ABSENT_REVISION, type ChangeResult, type ContentChangeSet } from "../api/contentChanges";
import { getTargetCoded, getTargetStatus, getTargetText } from "../components/editor/blockStatus";
import type { BlockInfo, BlockNote, TargetStatus } from "../types/api";

/** FNV-1a over a string, as 16 hex digits: a stand-in for a content revision. */
function fnv64(s: string): string {
  let h = 0xcbf29ce484222325n;
  for (const ch of s) {
    h ^= BigInt(ch.codePointAt(0) ?? 0);
    h = (h * 0x100000001b3n) & 0xffffffffffffffffn;
  }
  return h.toString(16).padStart(16, "0");
}

/** The revision a mock block's translation in one language holds. */
export function mockRevision(block: BlockInfo, locale: string): string {
  if (block.targets?.[locale] == null) return ABSENT_REVISION;
  return `r:${fnv64(`${getTargetText(block, locale)}\u0000${getTargetCoded(block, locale)}`)}`;
}

/** A block as the mock serves it: with each translation's revision. */
export function servedBlock(block: BlockInfo): BlockInfo {
  const target_revisions: Record<string, string> = {};
  for (const locale of Object.keys(block.targets ?? {})) {
    target_revisions[locale] = mockRevision(block, locale);
  }
  return { ...block, target_revisions };
}

/** Replace a mock block's translation, as a person's save leaves it. */
export function writeTranslation(
  block: BlockInfo,
  locale: string,
  text: string,
  coded: string,
  status: TargetStatus,
): void {
  block.targets = { ...block.targets, [locale]: { text, status } };
  block.targets_coded = { ...block.targets_coded, [locale]: coded };
}

/** The text a reload shows for runs: their text, without the codes. */
const PLAIN_SPEC: ModelRunSpec<string> = {
  text: (r) => r.text,
  ph: { dropped: "a code holds no text a reload shows" },
  pcOpen: { dropped: "a code holds no text a reload shows" },
  pcClose: { dropped: "a code holds no text a reload shows" },
  sub: { dropped: "a subblock reference holds no text a reload shows" },
  plural: ({ plural }) => projectRuns((plural.forms.other ?? []) as Run[], PLAIN_SPEC).join(""),
  select: ({ select }) => projectRuns(select.cases.other ?? [], PLAIN_SPEC).join(""),
  fallback: () => "",
};

const PLACEHOLDER = /<x id="[^"]*"\/>/g;

/** What the mock adapter's change service reads and writes. */
export interface MockChangeStore {
  blocks: BlockInfo[];
  notes: BlockNote[];
  /**
   * Findings the project's checks raise on any translation saved: under the
   * default gate a save is refused with them; under `report` it lands.
   */
  failingCheck?: ChangeFinding[];
}

/**
 * Apply a change set to the mock store. Every precondition is checked first,
 * against the blocks as they stood; one refusal writes nothing.
 */
export function applyMockChanges(store: MockChangeStore, set: ContentChangeSet): ChangeResult {
  const blockOf = (op: ChangeOp) =>
    "at" in op && op.at && "block" in op.at
      ? store.blocks.find((b) => b.id === op.at.block)
      : undefined;
  const results: OpResult[] = set.ops.map((op, i) => ({
    i,
    op: op.op,
    status: "applied" as const,
    ...("at" in op && op.at && "block" in op.at ? { at: { ...op.at } } : {}),
  }));

  let refused = -1;
  // What the checks found, by document: the result lists it on the document,
  // as the change service does.
  const found = new Map<string, ChangeFinding[]>();
  const docs = (written: boolean) =>
    [...found].map(([doc, findings]) => ({ doc, written, after: null, findings }));
  const refuse = (i: number, result: Partial<OpResult>) => {
    Object.assign(results[i], { status: "refused" }, result);
    if (refused < 0) refused = i;
  };

  set.ops.forEach((op, i) => {
    const block = blockOf(op);
    if (!block) {
      refuse(i, { error: { code: "not_found", message: "the item holds no such block" } });
      return;
    }
    if ((op.op === "set_content" || op.op === "decide") && op.at.edition) {
      const locale = op.at.edition;
      const rev = mockRevision(block, locale);
      if (op.if_match !== "*" && op.if_match !== rev) {
        refuse(i, {
          error: {
            code: "stale",
            field: "if_match",
            message: `edition ${locale} is at ${rev}, not ${op.if_match}`,
          },
          current: { rev, text: getTargetText(block, locale) },
        });
        return;
      }
      if (
        op.op === "decide" &&
        op.outcome === "establish" &&
        !getTargetText(block, locale).trim()
      ) {
        refuse(i, {
          error: {
            code: "unsupported",
            message: `block ${block.id} has no ${locale} translation to establish: translate it first`,
          },
        });
        return;
      }
      const findings = store.failingCheck;
      if (op.op === "set_content" && findings?.length) {
        found.set(op.at.doc, [...(found.get(op.at.doc) ?? []), ...findings]);
        if (set.gate === "report") return;
        refuse(i, {
          error: {
            code: "gate_failed",
            message: `the edit introduces ${findings.length} failing finding(s) in ${op.at.doc}: ${findings[0].message}`,
          },
        });
      }
    }
  });

  if (refused >= 0) {
    return {
      schema: CHANGE_RESULT_SCHEMA_ID,
      status: "refused",
      record: null,
      docs: docs(false),
      ops: results.map((r) =>
        r.status === "refused" ? r : { ...r, status: "not_applied", blocked_by: refused },
      ),
    };
  }

  set.ops.forEach((op, i) => {
    const block = blockOf(op);
    if (!block) return;
    const result = results[i];
    switch (op.op) {
      case "set_content": {
        const locale = op.at.edition;
        // Content arrives as placeholder text or as runs, never both.
        const { text: sent, runs } = op;
        const text =
          sent !== undefined
            ? sent.replace(PLACEHOLDER, "")
            : projectRuns((runs ?? []) as Run[], PLAIN_SPEC).join("");
        if (!locale) {
          block.source = text;
          break;
        }
        result.before = mockRevision(block, locale);
        const coded = runs !== undefined ? safeCoded(runs as Run[]) : text;
        if (runs !== undefined) {
          // The runs a change carries are the served runs without native code
          // data, which a mock block does without.
          const served = runs as unknown as NonNullable<BlockInfo["targets_runs"]>[string];
          block.targets_runs = { ...block.targets_runs, [locale]: served };
        } else if (block.targets_runs?.[locale]) {
          const { [locale]: _gone, ...rest } = block.targets_runs;
          block.targets_runs = rest;
        }
        const prevCoded = getTargetCoded(block, locale);
        const changed =
          prevCoded !== "" ? coded !== prevCoded : text !== getTargetText(block, locale);
        writeTranslation(
          block,
          locale,
          text,
          coded,
          changed ? "translated" : getTargetStatus(block, locale),
        );
        result.after = mockRevision(block, locale);
        if (!changed) result.status = "unchanged";
        break;
      }
      case "decide": {
        const locale = op.at.edition ?? "";
        const status: TargetStatus =
          op.outcome === "establish"
            ? "established"
            : op.outcome === "reject"
              ? "draft"
              : "translated";
        result.before = result.after = mockRevision(block, locale);
        block.targets = {
          ...block.targets,
          [locale]: { text: getTargetText(block, locale), status },
        };
        break;
      }
      case "annotate": {
        const id = op.id ?? `${op.type}-${Date.now()}-${i}`;
        result.id = id;
        if (op.type === "note") {
          const text = (op.value as { text?: string } | undefined)?.text ?? "";
          store.notes.push({
            id,
            blockId: block.id,
            author: "you@example.com",
            text,
            createdAt: new Date().toISOString(),
          });
        } else if (op.type === "entity") {
          const value = op.value as { Text: string; Type: string; DNT: boolean; Source: string };
          block.entities = [
            ...(block.entities ?? []),
            {
              key: id,
              text: value.Text,
              type: value.Type,
              start: op.anchor?.start?.offset ?? 0,
              end: op.anchor?.end?.offset ?? 0,
              dnt: value.DNT,
              source: value.Source,
            },
          ];
        }
        break;
      }
      case "unannotate": {
        if (op.type === "note") {
          store.notes.splice(0, store.notes.length, ...store.notes.filter((n) => n.id !== op.id));
        } else if (op.type === "entity") {
          block.entities = (block.entities ?? []).filter((e) => e.key !== op.id);
        }
        break;
      }
      default:
        result.status = "unchanged";
    }
  });

  return {
    schema: CHANGE_RESULT_SCHEMA_ID,
    status: "applied",
    record: "mock",
    docs: docs(true),
    ops: results,
  };
}

/** Coded text for runs, or "" for a plural the coded form cannot hold. */
function safeCoded(runs: Run[]): string {
  try {
    return runsToCoded(runs).codedText;
  } catch {
    return "";
  }
}
