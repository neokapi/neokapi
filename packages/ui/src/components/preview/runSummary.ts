import { projectRunsText, type RunSpec } from "@neokapi/kapi-format";
import type { Run } from "./types";

// A collapsed inspector label carries code tokens as well as literal text.
// Annotation-offset projections deliberately give codes zero width and cannot
// serve as a display summary.
const SUMMARY: RunSpec<Run, string> = {
  text: (run) => run.text ?? "",
  ph: (run) => {
    const label = run.ph?.disp || run.ph?.equiv || run.ph?.data || run.ph?.id || "placeholder";
    return label === "#" ? "#" : `⟨${label}⟩`;
  },
  pcOpen: (run) =>
    run.pcOpen?.disp || run.pcOpen?.equiv || run.pcOpen?.data || `<${run.pcOpen?.id}>`,
  pcClose: (run) =>
    run.pcClose?.disp || run.pcClose?.equiv || run.pcClose?.data || `</${run.pcClose?.id}>`,
  sub: (run) => `[${run.sub?.equiv || run.sub?.ref || run.sub?.id}]`,
  plural: (run) => `{${run.plural?.pivot}, plural, ${branches(run.plural?.forms ?? {})}}`,
  select: (run) => `{${run.select?.pivot}, select, ${branches(run.select?.cases ?? {})}}`,
  fallback: (kind) => `⟨${kind}⟩`,
};

function branches(values: Record<string, Run[]>): string {
  return Object.entries(values)
    .map(([key, runs]) => `${key} {${runSummary(runs)}}`)
    .join(" ");
}

export function runSummary(runs: Run[] | undefined): string {
  return projectRunsText(runs, SUMMARY);
}
