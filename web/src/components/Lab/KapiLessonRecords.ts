export interface ContextAnswer {
  provenance?: { revision?: number; stale?: boolean; stale_reason?: string };
  notes?: string[];
  suggestions?: Array<{ operation: string; term: string; replacement: string; status: string }>;
  terms?: Array<{
    term: string;
    status: string;
    replacement?: string;
    uses?: number;
    top_uses?: Array<{ document: string; block_id: string; snippet: string }>;
  }>;
}
export interface CheckAnswer {
  pass: boolean;
  verdict: string;
  summary: { findings: number; failing: number; reporting: number };
  findings: Array<{ message: string; fails: boolean; suggestion?: string }>;
}
function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function json(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}
export function parseContextAnswer(text: string): ContextAnswer | null {
  const value = json(text);
  if (!object(value)) return null;
  if (
    value.provenance !== undefined &&
    (!object(value.provenance) ||
      (value.provenance.revision !== undefined && typeof value.provenance.revision !== "number") ||
      (value.provenance.stale_reason !== undefined &&
        typeof value.provenance.stale_reason !== "string"))
  )
    return null;
  if (
    value.terms !== undefined &&
    (!Array.isArray(value.terms) ||
      !value.terms.every(
        (term) =>
          object(term) &&
          typeof term.term === "string" &&
          typeof term.status === "string" &&
          (term.replacement === undefined || typeof term.replacement === "string"),
      ))
  )
    return null;
  if (
    value.suggestions !== undefined &&
    (!Array.isArray(value.suggestions) ||
      !value.suggestions.every(
        (term) =>
          object(term) &&
          [term.operation, term.term, term.replacement, term.status].every(
            (field) => typeof field === "string",
          ),
      ))
  )
    return null;
  if (
    value.notes !== undefined &&
    (!Array.isArray(value.notes) || !value.notes.every((note) => typeof note === "string"))
  )
    return null;
  if (
    Array.isArray(value.terms) &&
    !value.terms.every(
      (term) =>
        object(term) &&
        (term.uses === undefined || typeof term.uses === "number") &&
        (term.top_uses === undefined ||
          (Array.isArray(term.top_uses) &&
            term.top_uses.every(
              (use) =>
                object(use) &&
                [use.document, use.block_id, use.snippet].every(
                  (field) => typeof field === "string",
                ),
            ))),
    )
  )
    return null;
  return value as ContextAnswer;
}
export function parseCheckAnswer(text: string): CheckAnswer | null {
  const value = json(text);
  if (
    !object(value) ||
    typeof value.pass !== "boolean" ||
    typeof value.verdict !== "string" ||
    !object(value.summary) ||
    ![value.summary.findings, value.summary.failing, value.summary.reporting].every(
      (field) => typeof field === "number",
    ) ||
    !Array.isArray(value.findings) ||
    !value.findings.every(
      (finding) =>
        object(finding) &&
        typeof finding.message === "string" &&
        typeof finding.fails === "boolean" &&
        (finding.suggestion === undefined || typeof finding.suggestion === "string"),
    )
  )
    return null;
  return value as unknown as CheckAnswer;
}
