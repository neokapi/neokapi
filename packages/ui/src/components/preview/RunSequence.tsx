import React from "react";
import { cn } from "../../lib/utils";
import type { Run, SegmentSpan } from "./types";

interface RunSequenceProps {
  runs: Run[];
  /** Optional segment boundary overlay; dotted markers are drawn at boundaries. */
  segments?: SegmentSpan[];
}

// Per-kind chip styling, as theme-aware Tailwind utilities (no custom CSS):
// placeholders = amber, paired codes = blue, plural/select = violet, sub = teal.
const CHIP = "inline-flex items-center gap-1 rounded border px-1 text-[0.75rem] leading-normal";
const CHIP_PH = "border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-400";
const CHIP_PC = "border-blue-500/30 bg-blue-500/15 text-blue-700 dark:text-blue-300";
const CHIP_PLURAL = "border-violet-500/30 bg-violet-500/15 text-violet-700 dark:text-violet-300";
const CHIP_SUB = "border-teal-500/30 bg-teal-500/15 text-teal-700 dark:text-teal-300";

// RunSequence renders a Block's run sequence inline, making the RFC 0001
// discriminators visible: plain text reads normally, while inline codes
// (placeholders, paired tags), sub-references and plurals render as labelled
// chips — so a learner can see exactly where structure lives inside text.
export default function RunSequence({ runs, segments }: RunSequenceProps): React.ReactElement {
  const base = "inline-flex flex-wrap items-center gap-1 font-mono text-[0.82rem]";
  if (!runs || runs.length === 0) {
    return <span className={cn(base, "text-muted-foreground italic")}>(empty)</span>;
  }
  const boundaries = new Set((segments ?? []).map((s) => s.start).filter((i) => i > 0));

  return (
    <span className={base}>
      {runs.map((run, i) => (
        <React.Fragment key={i}>
          {boundaries.has(i) && (
            <span
              className="mx-0.5 self-stretch border-l-2 border-dotted border-muted-foreground/50"
              title="segment boundary"
            />
          )}
          <RunChip run={run} />
        </React.Fragment>
      ))}
    </span>
  );
}

function RunChip({ run }: { run: Run }): React.ReactElement {
  if (run.text !== undefined) {
    return <span className="whitespace-pre-wrap">{run.text}</span>;
  }
  if (run.ph) {
    const label = run.ph.equiv || run.ph.data || `#${run.ph.id}`;
    return (
      <span className={cn(CHIP, CHIP_PH)} title={`placeholder ${run.ph.type ?? ""}`}>
        {label}
      </span>
    );
  }
  if (run.pcOpen) {
    const label = run.pcOpen.equiv || run.pcOpen.data || `<${run.pcOpen.id}>`;
    return (
      <span className={cn(CHIP, CHIP_PC)} title={`open code ${run.pcOpen.type ?? ""}`}>
        {label}
      </span>
    );
  }
  if (run.pcClose) {
    const label = run.pcClose.equiv || run.pcClose.data || `</${run.pcClose.id}>`;
    return (
      <span className={cn(CHIP, CHIP_PC)} title="close code">
        {label}
      </span>
    );
  }
  if (run.sub) {
    return (
      <span className={cn(CHIP, CHIP_SUB)} title="sub-block reference">
        → {run.sub.ref}
      </span>
    );
  }
  if (run.plural) {
    const forms = Object.keys(run.plural.forms).join(", ");
    return (
      <span className={cn(CHIP, CHIP_PLURAL)} title={`plural on ${run.plural.pivot}: ${forms}`}>
        plural({run.plural.pivot})
      </span>
    );
  }
  if (run.select) {
    const cases = Object.keys(run.select.cases).join(", ");
    return (
      <span className={cn(CHIP, CHIP_PLURAL)} title={`select on ${run.select.pivot}: ${cases}`}>
        select({run.select.pivot})
      </span>
    );
  }
  return <span className="text-muted-foreground italic">?</span>;
}
