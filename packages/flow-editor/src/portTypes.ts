// Port-type visual vocabulary — the semantic "data types" that flow between
// tools (overlays, block annotations, and the source/target pseudo-ports).
//
// Keyed off the canonical `PortType` union generated from Go
// (@neokapi/contract-types), so a new overlay/annotation added in the backend
// is a compile error here until it is given a presentation. This replaces the
// old block/data/media/layer color map that rendered every real port gray.
//
// Colors are grouped by *family* (one hue band each) so the canvas reads as
// "content vs. annotations vs. findings" at a glance; per-type icons + labels
// disambiguate within a family. Hues are chosen to sit clear of the category
// rail hues in ./category.ts.

import type { PortType } from "./types";
import {
  AlignLeft,
  Languages,
  Scissors,
  BookMarked,
  AtSign,
  GitCompare,
  Tag,
  ShieldAlert,
  Database,
  Sparkles,
  Megaphone,
  Diff,
  ShieldCheck,
  Hash,
  ListOrdered,
  Repeat,
  ClipboardList,
  StickyNote,
  Lock,
  Circle,
  type LucideIcon,
} from "lucide-react";

export type PortFamily =
  | "content" // the block's source text / committed target
  | "linguistic" // run-anchored interpretations (segmentation, terms, entities…)
  | "quality" // findings to act on
  | "suggestion" // candidate content / matches
  | "metric" // read-only counts & reports
  | "note" // human/AI notes
  | "security"; // redaction secrets

export interface FamilyStyle {
  /** OKLCH accent color shared by every port type in the family. */
  color: string;
  /** Translucent background variant. */
  bg: string;
  /** Family label for the legend. */
  label: string;
  /** One-line family description for the legend. */
  description: string;
}

export const PORT_FAMILIES: Record<PortFamily, FamilyStyle> = {
  content: {
    color: "oklch(0.62 0.13 250)",
    bg: "oklch(0.62 0.13 250 / 0.12)",
    label: "Content",
    description: "The block's source text and committed target translation.",
  },
  linguistic: {
    color: "oklch(0.7 0.11 195)",
    bg: "oklch(0.7 0.11 195 / 0.12)",
    label: "Linguistic",
    description: "Run-anchored interpretations: segments, terms, entities, alignment.",
  },
  quality: {
    color: "oklch(0.68 0.18 30)",
    bg: "oklch(0.68 0.18 30 / 0.12)",
    label: "Quality findings",
    description: "Issues a downstream step or reviewer should act on.",
  },
  suggestion: {
    color: "oklch(0.66 0.16 320)",
    bg: "oklch(0.66 0.16 320 / 0.12)",
    label: "Matches & suggestions",
    description: "Candidate translations, TM matches, brand/term proposals.",
  },
  metric: {
    color: "oklch(0.6 0.04 265)",
    bg: "oklch(0.6 0.04 265 / 0.12)",
    label: "Metrics & reports",
    description: "Read-only counts and analysis reports.",
  },
  note: {
    color: "oklch(0.72 0.12 95)",
    bg: "oklch(0.72 0.12 95 / 0.12)",
    label: "Notes",
    description: "Free-text notes attached to a block.",
  },
  security: {
    color: "oklch(0.6 0.2 15)",
    bg: "oklch(0.6 0.2 15 / 0.12)",
    label: "Security",
    description: "Redaction secrets restored after processing.",
  },
};

export interface PortTypeDef {
  label: string;
  family: PortFamily;
  icon: LucideIcon;
  description: string;
}

const PORT_TYPES: Record<PortType, PortTypeDef> = {
  // Content (pseudo-ports)
  source: {
    label: "Source",
    family: "content",
    icon: AlignLeft,
    description: "The source text (rewritten by source-transform tools).",
  },
  target: {
    label: "Target",
    family: "content",
    icon: Languages,
    description: "The committed target translation.",
  },
  // Linguistic overlays
  segmentation: {
    label: "Segments",
    family: "linguistic",
    icon: Scissors,
    description: "Sentence/chunk boundaries as a stand-off overlay.",
  },
  term: {
    label: "Terms",
    family: "linguistic",
    icon: BookMarked,
    description: "Matched terminology spans.",
  },
  entity: {
    label: "Entities",
    family: "linguistic",
    icon: AtSign,
    description: "Recognized named-entity spans.",
  },
  alignment: {
    label: "Alignment",
    family: "linguistic",
    icon: GitCompare,
    description: "Source↔target span alignment.",
  },
  "term-candidate": {
    label: "Term candidates",
    family: "linguistic",
    icon: Tag,
    description: "Proposed terminology not yet in the termbase.",
  },
  // Quality
  qa: {
    label: "QA findings",
    family: "quality",
    icon: ShieldAlert,
    description: "Quality-check findings anchored to spans.",
  },
  // Matches & suggestions
  "tm-match": {
    label: "TM matches",
    family: "suggestion",
    icon: Database,
    description: "Translation-memory matches.",
  },
  "alt-translation": {
    label: "Alt translations",
    family: "suggestion",
    icon: Sparkles,
    description: "Candidate translations from TM/MT/AI.",
  },
  "brand-voice": {
    label: "Brand voice",
    family: "suggestion",
    icon: Megaphone,
    description: "Brand-voice findings and rewrites.",
  },
  comparison: {
    label: "Comparison",
    family: "suggestion",
    icon: Diff,
    description: "Comparison of two candidate translations.",
  },
  "term-enforcement": {
    label: "Term enforcement",
    family: "suggestion",
    icon: ShieldCheck,
    description: "Enforced terminology corrections.",
  },
  // Metrics & reports
  "word-count": {
    label: "Word count",
    family: "metric",
    icon: Hash,
    description: "Per-block word counts.",
  },
  "char-count": {
    label: "Char count",
    family: "metric",
    icon: Hash,
    description: "Per-block character counts.",
  },
  "seg-count": {
    label: "Segment count",
    family: "metric",
    icon: ListOrdered,
    description: "Per-block segment counts.",
  },
  repetition: {
    label: "Repetition",
    family: "metric",
    icon: Repeat,
    description: "Repetition analysis across blocks.",
  },
  "scoping-report": {
    label: "Scoping",
    family: "metric",
    icon: ClipboardList,
    description: "Project scoping report.",
  },
  "entity-mapping": {
    label: "Entity mapping",
    family: "metric",
    icon: AtSign,
    description: "Entity-to-translation mapping.",
  },
  // Notes
  note: {
    label: "Notes",
    family: "note",
    icon: StickyNote,
    description: "Free-text notes on a block.",
  },
  // Security
  "redaction.secret": {
    label: "Redaction secret",
    family: "security",
    icon: Lock,
    description: "In-process map restoring redacted originals.",
  },
};

export interface PortTypeStyle extends PortTypeDef {
  /** Family accent color. */
  color: string;
  /** Family translucent background. */
  bg: string;
  /** Family display label. */
  familyLabel: string;
}

/** Heuristic family inference for unknown (plugin) port-type strings. */
function inferFamily(type: string): PortFamily {
  if (/count|report|stat|metric/.test(type)) return "metric";
  if (/qa|check|issue|error|warn/.test(type)) return "quality";
  if (/match|alt|suggest|candidate/.test(type)) return "suggestion";
  if (/term|segment|entity|align/.test(type)) return "linguistic";
  return "metric";
}

/**
 * getPortType resolves a port-type string (from a tool's consumes/produces) to
 * its full presentation. Unknown plugin types get a family-inferred fallback
 * (with the raw type as the label) so they still render meaningfully, never
 * gray-and-anonymous.
 */
export function getPortType(type: string): PortTypeStyle {
  const def =
    (PORT_TYPES as Record<string, PortTypeDef | undefined>)[type] ??
    ({
      label: type,
      family: inferFamily(type),
      icon: Circle,
      description: type,
    } satisfies PortTypeDef);
  const fam = PORT_FAMILIES[def.family];
  return { ...def, color: fam.color, bg: fam.bg, familyLabel: fam.label };
}
