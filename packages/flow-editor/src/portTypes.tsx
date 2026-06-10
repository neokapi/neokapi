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

import { t } from "@neokapi/kapi-react/runtime";
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
    get label() {
      return t("Content", "port type");
    },
    get description() {
      return t("The block's source text and committed target translation.");
    },
  },
  linguistic: {
    color: "oklch(0.7 0.11 195)",
    bg: "oklch(0.7 0.11 195 / 0.12)",
    get label() {
      return t("Linguistic", "port type");
    },
    get description() {
      return t("Run-anchored interpretations: segments, terms, entities, alignment.");
    },
  },
  quality: {
    color: "oklch(0.68 0.18 30)",
    bg: "oklch(0.68 0.18 30 / 0.12)",
    get label() {
      return t("Quality findings", "port type");
    },
    get description() {
      return t("Issues a downstream step or reviewer should act on.");
    },
  },
  suggestion: {
    color: "oklch(0.66 0.16 320)",
    bg: "oklch(0.66 0.16 320 / 0.12)",
    get label() {
      return t("Matches & suggestions", "port type");
    },
    get description() {
      return t("Candidate translations, TM matches, brand/term proposals.");
    },
  },
  metric: {
    color: "oklch(0.6 0.04 265)",
    bg: "oklch(0.6 0.04 265 / 0.12)",
    get label() {
      return t("Metrics & reports", "port type");
    },
    get description() {
      return t("Read-only counts and analysis reports.");
    },
  },
  note: {
    color: "oklch(0.72 0.12 95)",
    bg: "oklch(0.72 0.12 95 / 0.12)",
    get label() {
      return t("Notes", "port type");
    },
    get description() {
      return t("Free-text notes attached to a block.");
    },
  },
  security: {
    color: "oklch(0.6 0.2 15)",
    bg: "oklch(0.6 0.2 15 / 0.12)",
    get label() {
      return t("Security", "port type");
    },
    get description() {
      return t("Redaction secrets restored after processing.");
    },
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
    get label() {
      return t("Source", "port type");
    },
    family: "content",
    icon: AlignLeft,
    get description() {
      return t("The source text (rewritten by transformer tools).");
    },
  },
  target: {
    get label() {
      return t("Target", "port type");
    },
    family: "content",
    icon: Languages,
    get description() {
      return t("The committed target translation.");
    },
  },
  // Linguistic overlays
  segmentation: {
    get label() {
      return t("Segments", "port type");
    },
    family: "linguistic",
    icon: Scissors,
    get description() {
      return t("Sentence/chunk boundaries as a stand-off overlay.");
    },
  },
  term: {
    get label() {
      return t("Terms", "port type");
    },
    family: "linguistic",
    icon: BookMarked,
    get description() {
      return t("Matched terminology spans.");
    },
  },
  entity: {
    get label() {
      return t("Entities", "port type");
    },
    family: "linguistic",
    icon: AtSign,
    get description() {
      return t("Recognized named-entity spans.");
    },
  },
  alignment: {
    get label() {
      return t("Alignment", "port type");
    },
    family: "linguistic",
    icon: GitCompare,
    get description() {
      return t("Source↔target span alignment.");
    },
  },
  "term-candidate": {
    get label() {
      return t("Term candidates", "port type");
    },
    family: "linguistic",
    icon: Tag,
    get description() {
      return t("Proposed terminology not yet in the termbase.");
    },
  },
  // Quality
  qa: {
    get label() {
      return t("QA findings", "port type");
    },
    family: "quality",
    icon: ShieldAlert,
    get description() {
      return t("Quality-check findings anchored to spans.");
    },
  },
  // Matches & suggestions
  "tm-match": {
    get label() {
      return t("TM matches", "port type");
    },
    family: "suggestion",
    icon: Database,
    get description() {
      return t("Translation-memory matches.");
    },
  },
  "alt-translation": {
    get label() {
      return t("Alt translations", "port type");
    },
    family: "suggestion",
    icon: Sparkles,
    get description() {
      return t("Candidate translations from TM/MT/AI.");
    },
  },
  "brand-voice": {
    get label() {
      return t("Brand voice", "port type");
    },
    family: "suggestion",
    icon: Megaphone,
    get description() {
      return t("Brand-voice findings and rewrites.");
    },
  },
  comparison: {
    get label() {
      return t("Comparison", "port type");
    },
    family: "suggestion",
    icon: Diff,
    get description() {
      return t("Comparison of two candidate translations.");
    },
  },
  "term-enforcement": {
    get label() {
      return t("Term enforcement", "port type");
    },
    family: "suggestion",
    icon: ShieldCheck,
    get description() {
      return t("Enforced terminology corrections.");
    },
  },
  // Metrics & reports
  "word-count": {
    get label() {
      return t("Word count", "port type");
    },
    family: "metric",
    icon: Hash,
    get description() {
      return t("Per-block word counts.");
    },
  },
  "char-count": {
    get label() {
      return t("Char count", "port type");
    },
    family: "metric",
    icon: Hash,
    get description() {
      return t("Per-block character counts.");
    },
  },
  "seg-count": {
    get label() {
      return t("Segment count", "port type");
    },
    family: "metric",
    icon: ListOrdered,
    get description() {
      return t("Per-block segment counts.");
    },
  },
  repetition: {
    get label() {
      return t("Repetition", "port type");
    },
    family: "metric",
    icon: Repeat,
    get description() {
      return t("Repetition analysis across blocks.");
    },
  },
  "scoping-report": {
    get label() {
      return t("Scoping", "port type");
    },
    family: "metric",
    icon: ClipboardList,
    get description() {
      return t("Project scoping report.");
    },
  },
  "entity-mapping": {
    get label() {
      return t("Entity mapping", "port type");
    },
    family: "metric",
    icon: AtSign,
    get description() {
      return t("Entity-to-translation mapping.");
    },
  },
  // Notes
  note: {
    get label() {
      return t("Notes", "port type");
    },
    family: "note",
    icon: StickyNote,
    get description() {
      return t("Free-text notes on a block.");
    },
  },
  // Security
  "redaction.secret": {
    get label() {
      return t("Redaction secret", "port type");
    },
    family: "security",
    icon: Lock,
    get description() {
      return t("In-process map restoring redacted originals.");
    },
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
