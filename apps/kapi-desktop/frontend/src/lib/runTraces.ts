/**
 * Helpers over the last run's retained traces (ListRunTraces / GetLastTrace):
 * which run belongs on the flow shown, and how a retained file is named.
 */

import type { FlowStep, RunTraceFile } from "../types/api";

/** A stable key for a retained file: its input path and locale pass. */
export function traceFileKey(file: RunTraceFile): string {
  return `${file.file_path}\n${file.locale ?? ""}`;
}

function baseName(path: string): string {
  const cut = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return cut >= 0 ? path.slice(cut + 1) : path;
}

/** The picker's label for a retained file: its name, and the locale pass when there was one. */
export function traceFileLabel(file: RunTraceFile): string {
  const name = baseName(file.file_path);
  return file.locale ? `${name} · ${file.locale}` : name;
}

/**
 * Whether the steps a run executed are the steps shown: the same tools in
 * the same order with the same options, parallel groups included. A trace of
 * other steps would replay on nodes it never ran through, so the Run view
 * withholds it until the flow is changed back. Labels are display only and
 * do not count.
 */
export function sameSteps(ran: FlowStep[] | null | undefined, shown: FlowStep[]): boolean {
  if (!ran || ran.length !== shown.length) return false;
  return ran.every((step, i) => sameStep(step, shown[i]));
}

function sameStep(a: FlowStep, b: FlowStep): boolean {
  if ((a.tool ?? "") !== (b.tool ?? "")) return false;
  if (!sameValue(a.config ?? {}, b.config ?? {})) return false;
  const ap = a.parallel ?? [];
  const bp = b.parallel ?? [];
  return ap.length === bp.length && ap.every((step, i) => sameStep(step, bp[i]));
}

/** Structural equality over JSON values; object key order does not count. */
function sameValue(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
    return a.every((v, i) => sameValue(v, b[i]));
  }
  if (a !== null && b !== null && typeof a === "object" && typeof b === "object") {
    const oa = a as Record<string, unknown>;
    const ob = b as Record<string, unknown>;
    const ka = Object.keys(oa).sort();
    const kb = Object.keys(ob).sort();
    if (ka.length !== kb.length) return false;
    return ka.every((k, i) => k === kb[i] && sameValue(oa[k], ob[k]));
  }
  return false;
}
