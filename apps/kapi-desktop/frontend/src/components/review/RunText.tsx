import { Fragment, type ReactNode } from "react";
import { otherBranch, projectRuns, type RunSpec } from "@neokapi/kapi-format";
import type { Run } from "@neokapi/contract-types";

/**
 * The Review surface's reading of a run sequence: prose reads as prose, and
 * everything that is not prose reads as a labelled chip.
 *
 * The neighbourhood, the prior version and the memory match all travel as run
 * sequences, and a reviewer judging a translation has to see the placeholders
 * and the plurals inside it. The projection is declared rather than looped, so
 * a kind added to the model is a compile error here instead of a variable that
 * quietly stops being drawn (packages/kapi-format/src/run-projection.ts).
 */

const CHIP =
  "mx-0.5 inline-flex items-center rounded border px-1 align-baseline font-mono text-[10px] leading-tight";
const CHIP_PH = "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400";
const CHIP_PC = "border-blue-500/40 bg-blue-500/10 text-blue-700 dark:text-blue-300";
const CHIP_GROUP = "border-violet-500/40 bg-violet-500/10 text-violet-700 dark:text-violet-300";
const CHIP_SUB = "border-teal-500/40 bg-teal-500/10 text-teal-700 dark:text-teal-300";

function chip(cls: string, label: string, title: string): ReactNode {
  return (
    <span className={`${CHIP} ${cls}`} title={title} translate="no">
      {label}
    </span>
  );
}

const REVIEW_RUNS: RunSpec<Run, ReactNode> = {
  text: (r) => <span className="whitespace-pre-wrap">{r.text}</span>,
  ph: (r) => chip(CHIP_PH, r.ph.equiv || r.ph.data || r.ph.id, `placeholder ${r.ph.type}`),
  pcOpen: (r) => chip(CHIP_PC, r.pcOpen.equiv || `<${r.pcOpen.id}>`, `open code ${r.pcOpen.type}`),
  pcClose: (r) =>
    chip(CHIP_PC, r.pcClose.equiv || `</${r.pcClose.id}>`, `close code ${r.pcClose.type}`),
  sub: (r) => chip(CHIP_SUB, r.sub.equiv || r.sub.ref, "sub-block reference"),
  // A plural keeps its marker and its wording: the chip names the pivot that
  // chooses the form, and one form is read after it so the reviewer sees words.
  plural: {
    expand: (r) => [
      chip(CHIP_GROUP, `plural(${r.plural.pivot})`, Object.keys(r.plural.forms).join(", ")),
      ...projectRuns(otherBranch(r.plural.forms), REVIEW_RUNS),
    ],
  },
  select: {
    expand: (r) => [
      chip(CHIP_GROUP, `select(${r.select.pivot})`, Object.keys(r.select.cases).join(", ")),
      ...projectRuns(otherBranch(r.select.cases), REVIEW_RUNS),
    ],
  },
  fallback: (kind) => chip(CHIP_PH, kind, `a run this build cannot draw: ${kind}`),
};

export interface RunTextProps {
  runs: Run[] | undefined;
  className?: string;
  /** Direction and language attributes for the locale the runs are written in. */
  dirAttrs?: { dir?: string; lang?: string };
}

/** One run sequence, read inline. */
export function RunText({ runs, className, dirAttrs }: RunTextProps) {
  const parts = projectRuns(runs, REVIEW_RUNS);
  return (
    <span className={className} translate="no" {...dirAttrs}>
      {parts.map((part, i) => (
        <Fragment key={i}>{part}</Fragment>
      ))}
    </span>
  );
}
