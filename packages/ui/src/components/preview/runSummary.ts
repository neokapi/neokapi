import {
  CODE_KIND_WORDS,
  codeKind,
  humanCodeName,
  projectRunsText,
  type RunSpec,
} from "@neokapi/kapi-format";
import type { Run } from "./types";

/**
 * The word for a code in a summary: its human name, else its kind. Never its
 * native data or a synthesized identifier.
 */
function word(
  code:
    | { type?: string; subType?: string; equiv?: string; disp?: string; data?: string }
    | undefined,
): string {
  const name = humanCodeName(code?.disp, code?.equiv, code?.data);
  if (name) return name;
  // A placeholder with no type is a value.
  return code?.type ? CODE_KIND_WORDS[codeKind(code.type, code.subType)] : CODE_KIND_WORDS.variable;
}

// A collapsed inspector label carries code tokens as well as literal text.
// Annotation-offset projections deliberately give codes zero width and cannot
// serve as a display summary.
const SUMMARY: RunSpec<Run, string> = {
  text: (run) => run.text ?? "",
  ph: (run) => `⟨${word(run.ph)}⟩`,
  // A paired code's content reads as it is; the code itself adds no word.
  pcOpen: () => "",
  pcClose: () => "",
  sub: (run) => `⟨${humanCodeName(run.sub?.equiv, run.sub?.ref) ?? CODE_KIND_WORDS.content}⟩`,
  plural: (run) =>
    `⟨${humanCodeName(run.plural?.pivot) ?? "value"}: ${branches(run.plural?.forms ?? {})}⟩`,
  select: (run) =>
    `⟨${humanCodeName(run.select?.pivot) ?? "value"}: ${branches(run.select?.cases ?? {})}⟩`,
  fallback: () => `⟨${CODE_KIND_WORDS.unknown}⟩`,
};

function branches(values: Record<string, Run[]>): string {
  return Object.entries(values)
    .map(([key, runs]) => `${key}: ${runSummary(runs)}`)
    .join("; ");
}

export function runSummary(runs: Run[] | undefined): string {
  return projectRunsText(runs, SUMMARY);
}
