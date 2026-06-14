// Pure, presentation-free helpers for change-set operations (AD-021). Turning
// the eleven op types into human-readable diff rows, classifying which ops are
// governed (so the UI can badge them), and grouping a draft's ops by the part
// of the graph they touch. Kept free of React so it is unit-tested directly.
import type {
  ChangeSetOp,
  OpType,
  TermStatus,
  ConceptCreatePayload,
  ConceptUpdatePayload,
  ConceptDeletePayload,
  TermAddPayload,
  TermUpdatePayload,
  TermRemovePayload,
  TermStatusPayload,
  RelationAddPayload,
  RelationRemovePayload,
  VoiceRuleAddPayload,
  VoiceRuleRemovePayload,
} from "../../types/brand-graph";

// ── Governed classification ──────────────────────────────────────────────────
// Mirrors knowledge.IsGovernedOp + termbase.IsGovernedTransition (Go) so the UI
// badges the same ops the server gates. A governed op only reaches the live
// graph through a reviewed change-set with an approval from a second person.

/** True when a term-status transition is governed (→ forbidden/preferred, or away from forbidden). */
export function isGovernedTermTransition(from: TermStatus, to: TermStatus): boolean {
  if (from === to) return false;
  if (to === "forbidden" || to === "preferred") return true;
  return from === "forbidden";
}

/** True when an op is governed (knowledge.IsGovernedOp parity). */
export function isGovernedOp(op: ChangeSetOp): boolean {
  switch (op.op) {
    case "term.status": {
      const p = op.payload as TermStatusPayload;
      return isGovernedTermTransition(p.from, p.to);
    }
    case "relation.add": {
      const p = op.payload as RelationAddPayload;
      return p.relation.relation_type === "REPLACED_BY";
    }
    case "concept.delete":
    case "voice.rule.add":
    case "voice.rule.remove":
      return true;
    default:
      return false;
  }
}

// ── Human-readable diff rows ─────────────────────────────────────────────────

/** Which part of the graph an op touches — used to group a diff. */
export type OpCategory = "term" | "voice" | "relation" | "concept";

/** Visual weight for a diff row: a removal/ban reads destructive, a promotion success. */
export type OpTone = "default" | "destructive" | "success";

/** A change-set op decomposed for legible rendering. */
export interface OpDiffRow {
  category: OpCategory;
  /** A short imperative verb, e.g. "Ban", "Prefer", "Add relation". */
  verb: string;
  /** A full one-line sentence, e.g. "Ban “utilize” (en-US) → prefer “use”". */
  summary: string;
  /** Whether the op is governed (requires a second approval to merge). */
  governed: boolean;
  tone: OpTone;
}

const RELATION_PHRASE: Record<string, string> = {
  BROADER: "broader than",
  NARROWER: "narrower than",
  PART_OF: "part of",
  HAS_PART: "has part",
  RELATED: "related to",
  REPLACED_BY: "replaced by",
  USE_INSTEAD: "use instead",
  EXACT_MATCH: "exact match",
  CLOSE_MATCH: "close match",
  COMPETITOR: "competitor",
};

const q = (s: string): string => `“${s}”`;

/** Category an op belongs to, for grouping. */
export function opCategory(op: OpType): OpCategory {
  switch (op) {
    case "term.add":
    case "term.update":
    case "term.remove":
    case "term.status":
      return "term";
    case "relation.add":
    case "relation.remove":
      return "relation";
    case "voice.rule.add":
    case "voice.rule.remove":
      return "voice";
    default:
      return "concept";
  }
}

/** Decompose an op into a legible diff row. */
export function opDiffRow(op: ChangeSetOp): OpDiffRow {
  const governed = isGovernedOp(op);
  const base = { category: opCategory(op.op), governed } as const;

  switch (op.op) {
    case "concept.create": {
      const p = op.payload as ConceptCreatePayload;
      const term = p.concept.terms?.[0]?.text;
      return {
        ...base,
        verb: "Create concept",
        summary: term ? `Create concept ${q(term)}` : "Create a concept",
        tone: "default",
      };
    }
    case "concept.update": {
      const p = op.payload as ConceptUpdatePayload;
      const what = p.definition != null ? "definition" : p.domain != null ? "domain" : "metadata";
      return {
        ...base,
        verb: "Edit concept",
        summary: `Edit concept ${p.concept_id} (${what})`,
        tone: "default",
      };
    }
    case "concept.delete": {
      const p = op.payload as ConceptDeletePayload;
      return {
        ...base,
        verb: "Delete concept",
        summary: `Delete concept ${p.concept_id}`,
        tone: "destructive",
      };
    }
    case "term.add": {
      const p = op.payload as TermAddPayload;
      return {
        ...base,
        verb: "Add term",
        summary: `Add term ${q(p.term.text)} (${p.term.locale})`,
        tone: "default",
      };
    }
    case "term.update": {
      const p = op.payload as TermUpdatePayload;
      return {
        ...base,
        verb: "Edit term",
        summary: `Edit term ${q(p.text)} (${p.locale})`,
        tone: "default",
      };
    }
    case "term.remove": {
      const p = op.payload as TermRemovePayload;
      return {
        ...base,
        verb: "Remove term",
        summary: `Remove term ${q(p.text)} (${p.locale})`,
        tone: "destructive",
      };
    }
    case "term.status": {
      const p = op.payload as TermStatusPayload;
      if (p.to === "forbidden") {
        return {
          ...base,
          verb: "Ban",
          summary: `Ban ${q(p.text)} (${p.locale})`,
          tone: "destructive",
        };
      }
      if (p.to === "preferred") {
        return {
          ...base,
          verb: "Prefer",
          summary: `Prefer ${q(p.text)} (${p.locale})`,
          tone: "success",
        };
      }
      return {
        ...base,
        verb: "Set status",
        summary: `${q(p.text)} (${p.locale}): ${p.from} → ${p.to}`,
        tone: "default",
      };
    }
    case "relation.add": {
      const p = op.payload as RelationAddPayload;
      const phrase = RELATION_PHRASE[p.relation.relation_type] ?? p.relation.relation_type;
      return {
        ...base,
        verb: "Add relation",
        summary: `${p.relation.source_id} ${phrase} ${p.relation.target_id}`,
        tone: "default",
      };
    }
    case "relation.remove": {
      const p = op.payload as RelationRemovePayload;
      return {
        ...base,
        verb: "Remove relation",
        summary: `Remove relation ${p.relation_id}`,
        tone: "destructive",
      };
    }
    case "voice.rule.add": {
      const p = op.payload as VoiceRuleAddPayload;
      const tone: OpTone = p.list === "preferred" ? "success" : "destructive";
      const arrow = p.rule.replacement ? ` → prefer ${q(p.rule.replacement)}` : "";
      return {
        ...base,
        verb: `Add ${p.list} rule`,
        summary: `Add ${p.list} rule ${q(p.rule.term)}${arrow}`,
        tone,
      };
    }
    case "voice.rule.remove": {
      const p = op.payload as VoiceRuleRemovePayload;
      return {
        ...base,
        verb: `Remove ${p.list} rule`,
        summary: `Remove ${p.list} rule ${q(p.term)}`,
        tone: "destructive",
      };
    }
    default:
      return { ...base, verb: op.op, summary: op.op, tone: "default" };
  }
}

/** A one-line summary of an op (the row sentence). */
export function opSummary(op: ChangeSetOp): string {
  return opDiffRow(op).summary;
}

// ── Grouping ─────────────────────────────────────────────────────────────────

const CATEGORY_ORDER: OpCategory[] = ["term", "voice", "relation", "concept"];

export const CATEGORY_LABEL: Record<OpCategory, string> = {
  term: "Terms",
  voice: "Voice rules",
  relation: "Relations",
  concept: "Concepts",
};

/** A group of diff rows that share a category, preserving op order within. */
export interface OpGroup {
  category: OpCategory;
  rows: { op: ChangeSetOp; row: OpDiffRow }[];
}

/** Group a change-set's ops by category, in a stable display order. */
export function groupOps(ops: ChangeSetOp[]): OpGroup[] {
  const byCat = new Map<OpCategory, { op: ChangeSetOp; row: OpDiffRow }[]>();
  for (const op of ops) {
    const row = opDiffRow(op);
    const arr = byCat.get(row.category) ?? [];
    arr.push({ op, row });
    byCat.set(row.category, arr);
  }
  return CATEGORY_ORDER.filter((c) => byCat.has(c)).map((category) => ({
    category,
    rows: byCat.get(category)!,
  }));
}

/** How many of a change-set's ops are governed. */
export function governedOpCount(ops: ChangeSetOp[]): number {
  return ops.reduce((n, op) => n + (isGovernedOp(op) ? 1 : 0), 0);
}
