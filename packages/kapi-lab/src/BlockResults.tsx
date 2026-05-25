import React from "react";
import type { FlowTrace } from "./types";
import styles from "./BlockResults.module.css";

export interface BlockResultsProps {
  trace: FlowTrace;
  /** Locale label for the target column header (e.g. "fr"). */
  targetLocale?: string;
}

interface Row {
  id: string;
  srcBefore: string;
  srcAfter: string | null; // null ⇒ the part was dropped (skip)
  tgtBefore: string;
  tgtAfter: string;
  dropped: boolean;
}

// The node whose output we compare against the initial state — the last tool in
// the pipeline (reader/writer aside), so this works for a single tool and for a
// multi-step flow alike.
function lastToolNodeId(trace: FlowTrace): string {
  const tools = trace.nodes.filter((n) => n.type === "tool");
  return tools.length > 0 ? tools[tools.length - 1].id : "tool-0";
}

// BlockResults renders what a tool/flow did to each Block: source and target
// before → after, highlighting what changed and marking dropped parts. Unlike a
// source-only diff it surfaces target edits (e.g. a translation), and the per-row
// layout leaves room for annotations once snapshots carry them.
export default function BlockResults({
  trace,
  targetLocale,
}: BlockResultsProps): React.ReactElement {
  const nodeId = lastToolNodeId(trace);

  const rows: Row[] = Object.values(trace.parts)
    .filter((ss) => ss.initial.type === "Block")
    .map((ss) => {
      const after = ss.afterNode?.[nodeId];
      return {
        id: ss.initial.id,
        srcBefore: ss.initial.sourceText ?? "",
        srcAfter: after ? (after.sourceText ?? "") : null,
        tgtBefore: ss.initial.targetText ?? "",
        tgtAfter: after?.targetText ?? "",
        dropped: !after,
      };
    });

  // Only show the target column when something actually has a target — keeps
  // source-only transforms (uppercase, redact, …) uncluttered.
  const showTarget = rows.some((r) => r.tgtAfter || r.tgtBefore);

  if (rows.length === 0) {
    return <div className={styles.empty}>No blocks were produced.</div>;
  }

  return (
    <table className={styles.table}>
      <thead>
        <tr>
          <th className={styles.idCol}>Block</th>
          <th>Source</th>
          {showTarget && <th>Target{targetLocale ? ` · ${targetLocale}` : ""}</th>}
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.id} className={r.dropped ? styles.droppedRow : ""}>
            <td className={styles.id}>{r.id}</td>
            <td>
              <Cell before={r.srcBefore} after={r.srcAfter} dropped={r.dropped} />
            </td>
            {showTarget && (
              <td>
                <Cell
                  before={r.tgtBefore}
                  after={r.dropped ? null : r.tgtAfter}
                  dropped={r.dropped}
                />
              </td>
            )}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// Cell shows a value, or a before → after pair when it changed.
function Cell({
  before,
  after,
  dropped,
}: {
  before: string;
  after: string | null;
  dropped: boolean;
}): React.ReactElement {
  if (dropped) return <em className={styles.muted}>dropped (skip)</em>;
  if (after === null || (after === "" && before === ""))
    return <span className={styles.muted}>—</span>;
  if (after === before) return <span className={styles.unchanged}>{before}</span>;
  return (
    <span className={styles.changed}>
      {before !== "" && (
        <>
          <span className={styles.before}>{before}</span>
          <span className={styles.arrow}> → </span>
        </>
      )}
      <span className={styles.after}>
        {after === "" ? <span className={styles.muted}>(empty)</span> : after}
      </span>
    </span>
  );
}
