// Shape of web/static/data/format-maturity.json, produced by the
// format-triage workflow (.claude/workflows/format-triage.js) and seeded from
// the format maturity gap analysis. See docs/internals/format-maturity.md,
// especially §3 "Dataset & history contract (v3)".
//
// Version compatibility: the page must render a v1 dataset (no
// `scorer_version`), a v2 dataset (`scorer_version: 2`, `run_integrity`,
// evidence/floor/ceiling/delta on rows) and a v3 dataset (per-axis `levels`,
// `dims`, `tier`, `summary.by_axis`) unchanged — every post-v1 field below is
// therefore optional/additive.

export type Level = "L0" | "L1" | "L2" | "L3" | "L4"; // engine axis
export type VocabGrade = "V0" | "V1" | "V2" | "V3";
export type EditorGrade = "E0" | "E1" | "E2" | "E3" | "E4";
export type KnowledgeGrade = "K0" | "K1" | "K2" | "K3";
export type CorpusGrade = "C0" | "C1" | "C2" | "C3";
export type Grade = Level | VocabGrade | EditorGrade | KnowledgeGrade | CorpusGrade;

export type AxisId = "engine" | "vocabulary" | "editor" | "knowledge" | "corpus";
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
  L0: "Experimental",
  L1: "Readable + writable",
  L2: "Specified",
  L3: "Parity-verified",
  L4: "Rock-solid",
};

export const AXIS_IDS: AxisId[] = ["engine", "vocabulary", "editor", "knowledge", "corpus"];

export const AXIS_LABEL: Record<AxisId, string> = {
  engine: "Engine",
  vocabulary: "Vocabulary",
  editor: "Editor",
  knowledge: "Knowledge",
  corpus: "Corpus",
};

export const AXIS_GRADES: Record<AxisId, Grade[]> = {
  engine: LEVELS,
  vocabulary: ["V0", "V1", "V2", "V3"],
  editor: ["E0", "E1", "E2", "E3", "E4"],
  knowledge: ["K0", "K1", "K2", "K3"],
  corpus: ["C0", "C1", "C2", "C3"],
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
};

export const GRADE_NAME: Record<Grade, string> = {
  ...LEVEL_NAME,
  V0: "Opaque",
  V1: "Typed reading",
  V2: "Bidirectional",
  V3: "Fidelity-proven",
  E0: "None",
  E1: "Faithful preview",
  E2: "Round-trip workflow",
  E3: "Embedded",
  E4: "Continuous",
  K0: "Undocumented",
  K1: "Grounded",
  K2: "Executable",
  K3: "Living",
  C0: "Unprovenanced",
  C1: "Exemplars",
  C2: "Manifested + fetched",
  C3: "Broad",
};

/** Demotion ladder order, highest promise first (rubric §1). */
export const TIER_ORDER: SupportTier[] = ["supported", "maintained", "available"];

export const TIER_LABEL: Record<SupportTier, string> = {
  supported: "Supported",
  maintained: "Maintained",
  available: "Available",
};

/** Certification decay thresholds in days (rubric §1): older than 45 days the
 * tier is flagged stale; older than 120 days the dashboard displays the
 * decayed tier (one level down) alongside the declared one. */
export const TIER_STALE_DAYS = 45;
export const TIER_DECAY_DAYS = 120;
