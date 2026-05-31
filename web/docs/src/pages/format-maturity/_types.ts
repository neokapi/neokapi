// Shape of web/docs/static/data/format-maturity.json, produced by the
// format-triage workflow (.claude/workflows/format-triage.js) and seeded from
// the format maturity gap analysis. See docs/internals/format-maturity.md.

export type Level = "L0" | "L1" | "L2" | "L3" | "L4";
export type DimScore = "complete" | "partial" | "none" | "na";
export type FormatType = "parity" | "harvest" | "read-only" | "internal";

export interface FormatRow {
  id: string;
  type: FormatType;
  level: Level;
  next_level: string;
  okapi_counterpart: string;
  dimensions: Record<string, DimScore>;
  blocking_gaps: string[];
  top_risk: string;
  confidence: string;
}

export interface MaturityData {
  generated_at: string;
  target_level: Level;
  source: string;
  summary: { total: number; by_level: Record<Level, number> };
  dimensions: string[];
  dimension_labels: Record<string, string>;
  formats: FormatRow[];
}

export interface HistorySnapshot {
  date: string;
  total: number;
  by_level: Record<Level, number>;
}

export const LEVELS: Level[] = ["L0", "L1", "L2", "L3", "L4"];

export const LEVEL_NAME: Record<Level, string> = {
  L0: "Experimental",
  L1: "Readable + writable",
  L2: "Specified",
  L3: "Parity-verified",
  L4: "Rock-solid",
};
