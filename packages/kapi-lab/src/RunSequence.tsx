import React from "react";
import type { Run, SegmentSpan } from "./types";
import styles from "./styles.module.css";

interface RunSequenceProps {
  runs: Run[];
  /** Optional segment boundary overlay; dotted markers are drawn at boundaries. */
  segments?: SegmentSpan[];
}

// RunSequence renders a Block's run sequence inline, making the RFC 0001
// discriminators visible: plain text reads normally, while inline codes
// (placeholders, paired tags), sub-references and plurals render as labelled
// chips — so a learner can see exactly where structure lives inside text.
export default function RunSequence({ runs, segments }: RunSequenceProps): React.ReactElement {
  if (!runs || runs.length === 0) {
    return <span className={`${styles.runs} ${styles.runEmpty}`}>(empty)</span>;
  }
  const boundaries = new Set((segments ?? []).map((s) => s.start).filter((i) => i > 0));

  return (
    <span className={styles.runs}>
      {runs.map((run, i) => (
        <React.Fragment key={i}>
          {boundaries.has(i) && <span className={styles.segMarker} title="segment boundary" />}
          <RunChip run={run} />
        </React.Fragment>
      ))}
    </span>
  );
}

function RunChip({ run }: { run: Run }): React.ReactElement {
  if (run.text) {
    return <span className={styles.runText}>{run.text.text}</span>;
  }
  if (run.ph) {
    const label = run.ph.equiv || run.ph.data || `#${run.ph.id}`;
    return (
      <span
        className={`${styles.runCode} ${styles.runPh}`}
        title={`placeholder ${run.ph.type ?? ""}`}
      >
        {label}
      </span>
    );
  }
  if (run.pcOpen) {
    const label = run.pcOpen.equiv || run.pcOpen.data || `<${run.pcOpen.id}>`;
    return (
      <span
        className={`${styles.runCode} ${styles.runPc}`}
        title={`open code ${run.pcOpen.type ?? ""}`}
      >
        {label}
      </span>
    );
  }
  if (run.pcClose) {
    const label = run.pcClose.equiv || run.pcClose.data || `</${run.pcClose.id}>`;
    return (
      <span className={`${styles.runCode} ${styles.runPc}`} title="close code">
        {label}
      </span>
    );
  }
  if (run.sub) {
    return (
      <span className={`${styles.runCode} ${styles.runSub}`} title="sub-block reference">
        → {run.sub.ref}
      </span>
    );
  }
  if (run.plural) {
    const forms = Object.keys(run.plural.forms).join(", ");
    return (
      <span
        className={`${styles.runCode} ${styles.runPlural}`}
        title={`plural on ${run.plural.pivot}: ${forms}`}
      >
        plural({run.plural.pivot})
      </span>
    );
  }
  if (run.select) {
    const cases = Object.keys(run.select.cases).join(", ");
    return (
      <span
        className={`${styles.runCode} ${styles.runPlural}`}
        title={`select on ${run.select.pivot}: ${cases}`}
      >
        select({run.select.pivot})
      </span>
    );
  }
  return <span className={styles.runEmpty}>?</span>;
}
