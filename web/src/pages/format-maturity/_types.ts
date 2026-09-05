import { t } from "@neokapi/i18n-react/runtime";

// Shape of web/static/data/format-maturity.json, produced by the
// format-triage workflow (.claude/workflows/format-triage.js) and seeded from
// the format maturity gap analysis. See docs/internals/format-maturity.md,
// especially §3 "Dataset & history contract (v3)".
//
// Version compatibility: the page must render a v1 dataset (no
// `scorer_version`), a v2 dataset (`scorer_version: 2`, `run_integrity`,
// evidence/floor/ceiling/delta on rows), a v3 dataset (per-axis `levels`,
// `dims`, `tier`, `summary.by_axis`) and a v4 dataset (the additive
// Structure & Geometry axis `structure`) unchanged — every post-v1 field below
// is therefore optional/additive. A v3 (6-axis) dataset simply carries no
// `structure` data, and the page guards every structure-bearing field.

export type Level = "L0" | "L1" | "L2" | "L3" | "L4"; // engine axis
export type VocabGrade = "V0" | "V1" | "V2" | "V3";
export type EditorGrade = "E0" | "E1" | "E2" | "E3" | "E4";
export type KnowledgeGrade = "K0" | "K1" | "K2" | "K3";
export type CorpusGrade = "C0" | "C1" | "C2" | "C3";
export type SecurityGrade = "S0" | "S1" | "S2" | "S3" | "S4";
/** Structure & Geometry axis (scorer v4) — a cumulative comprehension-depth
 * ladder: G0 opaque → G1 metadata → G2 linear body text → G3 logical structure
 * → G4 + spatial geometry. */
export type StructureGrade = "G0" | "G1" | "G2" | "G3" | "G4";
export type Grade =
  | Level
  | VocabGrade
  | EditorGrade
  | KnowledgeGrade
  | CorpusGrade
  | SecurityGrade
  | StructureGrade;

export type AxisId =
  | "engine"
  | "vocabulary"
  | "editor"
  | "knowledge"
  | "corpus"
  | "security"
  | "structure";
export type DimScore = "complete" | "partial" | "none" | "na";
export type FormatType = "parity" | "harvest" | "read-only" | "internal";

/** Support tier ladder (docs/internals/format-maturity.md §1). */
export type SupportTier = "supported" | "maintained" | "available";

/**
 * Per-row tier block, read from core/formats/support.yaml at publish time.
 * Staleness is computed client-side from `generated_at − last_certified`
 * (>45d stale, >120d decayed). `last_certified: null` means never certified
 * (bootstrap / grandfathered) — there is no baseline, so no decay is shown.
 */
export interface TierInfo {
  declared: SupportTier;
  since?: string;
  last_certified?: string | null;
  gates?: string[];
}

export interface MoveCounts {
  published: number;
  suppressed: number;
}

/** Emitted since scorer v2; a v3 run may key moves/low_agreement per axis. */
export interface RunIntegrity {
  samples?: number;
  anchored?: boolean;
  moves?: MoveCounts | Partial<Record<AxisId, MoveCounts>>;
  low_agreement?: string[] | Partial<Record<AxisId, string[]>>;
  golden_passed?: boolean;
}

export interface FormatRow {
  id: string;
  type: FormatType;
  /** Mirrors the engine axis in every scorer version (v3 contract). */
  level: Level;
  next_level: string;
  okapi_counterpart: string;
  /** Legacy flat grid — the engine dimensions in every scorer version. */
  dimensions: Record<string, DimScore>;
  blocking_gaps: string[];
  top_risk: string;
  confidence: string;
  // ── scorer v2 additive fields (emitted since v2, previously untyped) ──
  evidence?: Record<string, string>;
  floor?: string;
  ceiling?: string;
  derived_from?: string;
  delta?: { from?: string; to?: string; why?: string } | null;
  agreement?: number;
  samples?: number;
  // ── scorer v3 additive fields (multi-axis) ──
  levels?: Partial<Record<AxisId, Grade>>;
  dims?: Partial<Record<AxisId, Record<string, DimScore>>>;
  next?: Partial<Record<AxisId, string>>;
  tier?: TierInfo;
}

export interface MaturityData {
  generated_at: string;
  target_level: Level;
  source: string;
  /** Absent ⇒ scorer v1 (the prior parser never gates on version). */
  scorer_version?: number;
  run_integrity?: RunIntegrity;
  summary: {
    total: number;
    /** Remains the engine distribution in every scorer version. */
    by_level: Record<Level, number>;
    /** Additive in v3. */
    by_axis?: Partial<Record<AxisId, Partial<Record<Grade, number>>>>;
  };
  dimensions: string[];
  dimension_labels: Record<string, string>;
  /** Additive in v3: axis id → ordered grade ladder. */
  axes?: Partial<Record<AxisId, string[]>>;
  axis_labels?: Partial<Record<AxisId, string>>;
  /** Additive in v3: dimension id → owning axis. */
  dimension_axes?: Record<string, AxisId>;
  formats: FormatRow[];
}

export interface HistorySnapshot {
  date: string;
  total: number;
  by_level: Record<Level, number>;
  // Appended by v2+ publishes; the oldest snapshots never carry them.
  golden_passed?: boolean;
  moves?: MoveCounts | Partial<Record<AxisId, MoveCounts>>;
  // v3 snapshots only — old entries are never rewritten, so the page must
  // guard every access (h.by_axis?.… ?? 0).
  by_axis?: Partial<Record<AxisId, Partial<Record<Grade, number>>>>;
}

export const LEVELS: Level[] = ["L0", "L1", "L2", "L3", "L4"];

export const LEVEL_NAME: Record<Level, string> = {
  L0: t("Experimental", "format maturity level"),
  L1: t("Readable + writable", "format maturity level"),
  L2: t("Specified", "format maturity level"),
  L3: t("Parity-verified", "format maturity level"),
  L4: t("Rock-solid", "format maturity level"),
};

export const AXIS_IDS: AxisId[] = [
  "engine",
  "vocabulary",
  "editor",
  "knowledge",
  "corpus",
  "security",
  "structure",
];

export const AXIS_LABEL: Record<AxisId, string> = {
  engine: t("Engine", "format maturity axis"),
  vocabulary: t("Vocabulary", "format maturity axis"),
  editor: t("Editor", "format maturity axis"),
  knowledge: t("Knowledge", "format maturity axis"),
  corpus: t("Corpus", "format maturity axis"),
  security: t("Security", "format maturity axis"),
  structure: t("Structure & Geometry", "format maturity axis"),
};

export const AXIS_GRADES: Record<AxisId, Grade[]> = {
  engine: LEVELS,
  vocabulary: ["V0", "V1", "V2", "V3"],
  editor: ["E0", "E1", "E2", "E3", "E4"],
  knowledge: ["K0", "K1", "K2", "K3"],
  corpus: ["C0", "C1", "C2", "C3"],
  security: ["S0", "S1", "S2", "S3", "S4"],
  structure: ["G0", "G1", "G2", "G3", "G4"],
};

/**
 * Canonical per-axis dimension ids (rubric §2/§3 floor signals). The corpus
 * quality dimension (`corpus`) is shared between the Engine and Corpus axes.
 * Editor is floor-only — probed via integrations.yaml — and has no canonical
 * dimensions; any probe dims a dataset carries are rendered as-is.
 */
export const AXIS_DIMS: Record<AxisId, string[]> = {
  engine: [
    "reader",
    "writer",
    "config",
    "spec",
    "parity",
    "malformed",
    "corpus",
    "detection",
    "docs",
  ],
  vocabulary: ["vocabmap", "vocabtypes", "writecells", "equivalence"],
  editor: [],
  knowledge: ["dossier", "sidecar", "refs", "citations", "contextpack"],
  corpus: ["corpusmanifest", "corpus", "fetchwiring", "acceptance", "sweep"],
  // Security is floor-only (rubric §2.6); these signal ids map S1–S4.
  security: ["safeio", "fuzz", "sweepclean", "sustained"],
  // Structure & Geometry is floor-only (rubric §2.7); these signal ids map the
  // cumulative G1–G4 rungs (metadata plane / reading order / roles / geometry).
  structure: ["metaplane", "readingorder", "roles", "geometry"],
};

/**
 * One-line "measures" text per axis — verbatim from the rubric §2 axes table
 * (docs/internals/format-maturity.md) and the public axes page
 * (web/docs/framework/format-maturity/axes.mdx), so the dashboard and the docs
 * never drift. Surfaced as `title=` tooltips and in the visible axis key.
 */
export const AXIS_DESC: Record<AxisId, string> = {
  engine: t(
    "Parse/round-trip/parity fidelity and robustness",
    "what a format maturity axis measures",
  ),
  vocabulary: t(
    "How richly format semantics map into the canonical content-model vocabulary (and back)",
    "what a format maturity axis measures",
  ),
  editor: t(
    "How close kapi gets to the format's native editing surface",
    "what a format maturity axis measures",
  ),
  knowledge: t(
    "The spec/learning assets that let a person or model work on the format",
    "what a format maturity axis measures",
  ),
  corpus: t(
    "Reference files that validate support, with provenance",
    "what a format maturity axis measures",
  ),
  security: t(
    "Resource-boundedness, fuzzing, and hostile-corpus hardening of the parser (non-gating display axis)",
    "what a format maturity axis measures",
  ),
  structure: t(
    "How much of the document's logical and spatial structure the reader recovers — roles, reading order, tables, relations, geometry (non-gating display axis)",
    "what a format maturity axis measures",
  ),
};

/** The two non-gating display axes (rubric §2): they score and rank work but
 * do not enter the tier minimum (for now). */
export const NON_GATING_AXES: AxisId[] = ["security", "structure"];

export const GRADE_NAME: Record<Grade, string> = {
  ...LEVEL_NAME,
  V0: t("Opaque", "format maturity grade"),
  V1: t("Typed reading", "format maturity grade"),
  V2: t("Bidirectional", "format maturity grade"),
  V3: t("Fidelity-proven", "format maturity grade"),
  E0: t("None", "format maturity grade"),
  E1: t("Faithful preview", "format maturity grade"),
  E2: t("Round-trip workflow", "format maturity grade"),
  E3: t("Embedded", "format maturity grade"),
  E4: t("Continuous", "format maturity grade"),
  K0: t("Undocumented", "format maturity grade"),
  K1: t("Grounded", "format maturity grade"),
  K2: t("Executable", "format maturity grade"),
  K3: t("Living", "format maturity grade"),
  C0: t("Unprovenanced", "format maturity grade"),
  C1: t("Exemplars", "format maturity grade"),
  C2: t("Manifested + fetched", "format maturity grade"),
  C3: t("Broad", "format maturity grade"),
  S0: t("Unbounded", "format maturity grade"),
  S1: t("Bounded", "format maturity grade"),
  S2: t("Fuzzed", "format maturity grade"),
  S3: t("Hostile-hardened", "format maturity grade"),
  S4: t("Continuously-assured", "format maturity grade"),
  G0: t("Opaque", "format maturity grade"),
  G1: t("Metadata", "format maturity grade"),
  G2: t("Linear body text", "format maturity grade"),
  G3: t("Logical structure", "format maturity grade"),
  G4: t("Spatial geometry", "format maturity grade"),
};

// ── Axis families (rubric §1 — a dashboard reading aid, NOT a gating unit) ──
// The seven axes group into three families by the question each answers
// ("how deeply we read it / how we prove it / how we work with it"). The
// support-tier gate is unchanged: it is still `min` over the gating axis set
// (engine ∧ corpus ∧ knowledge), which deliberately straddles all three
// families. Families only organize the dashboard; they never gate.
export type FamilyId = "comprehension" | "assurance" | "enablement";

export const FAMILY_ORDER: FamilyId[] = ["comprehension", "assurance", "enablement"];

export const FAMILY_LABEL: Record<FamilyId, string> = {
  comprehension: t("Comprehension", "format maturity axis family"),
  assurance: t("Assurance", "format maturity axis family"),
  enablement: t("Enablement", "format maturity axis family"),
};

/** One-line mental model for each family (rubric §1), used as a tooltip. */
export const FAMILY_TAGLINE: Record<FamilyId, string> = {
  comprehension: t("How deeply we read it", "what an axis family is for"),
  assurance: t("How we prove it", "what an axis family is for"),
  enablement: t("How we work with it", "what an axis family is for"),
};

/** Axes per family, in display order. Comprehension is the three fidelity
 * resolutions (bytes → inline → structure); Structure & Geometry is the new
 * third member. Any axis absent from a dataset is filtered out at render time,
 * so a v3 (6-axis) dataset still groups cleanly. */
export const FAMILY_AXES: Record<FamilyId, AxisId[]> = {
  comprehension: ["engine", "vocabulary", "structure"],
  assurance: ["corpus", "security"],
  enablement: ["knowledge", "editor"],
};

/** Demotion ladder order, highest promise first (rubric §1). */
export const TIER_ORDER: SupportTier[] = ["supported", "maintained", "available"];

export const TIER_LABEL: Record<SupportTier, string> = {
  supported: t("Supported", "format support tier"),
  maintained: t("Maintained", "format support tier"),
  available: t("Available", "format support tier"),
};

/** What each support tier promises (rubric §1 "Meaning" column) — the contract
 * a user may rely on, changed only by an explicit human-approved event. */
export const TIER_MEANING: Record<SupportTier, string> = {
  supported: t(
    "Release-gating — a regression in this format blocks any release.",
    "what a format support tier promises",
  ),
  maintained: t(
    "Tested and kept green; regressions fixed on cadence, may not block a release.",
    "what a format support tier promises",
  ),
  available: t(
    "Registered and usable; explicitly experimental, no fidelity promise.",
    "what a format support tier promises",
  ),
};

/** Certification decay thresholds in days (rubric §1): older than 45 days the
 * tier is flagged stale; older than 120 days the dashboard displays the
 * decayed tier (one level down) alongside the declared one. */
export const TIER_STALE_DAYS = 45;
export const TIER_DECAY_DAYS = 120;
