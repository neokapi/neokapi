import React from "react";
import type { FlowNode, PartSnapshotSet } from "@neokapi/ui-primitives/preview";
import styles from "./styles.module.css";

interface PartInspectorProps {
  partId: string | null;
  parts: Record<string, PartSnapshotSet>;
  nodes: FlowNode[];
  /** Node ids touched at the current step — highlighted in the history. */
  activeNodeIds?: string[];
  /** Opens the full Part details modal for the selected part, if provided. */
  onOpenDetails?: () => void;
}

export default function PartInspector({
  partId,
  parts,
  nodes,
  activeNodeIds = [],
  onOpenDetails,
}: PartInspectorProps): React.ReactElement {
  if (!partId || !parts[partId]) {
    return (
      <div className={styles.inspector}>
        <div className={styles.inspectorHint}>
          Step through the flow, or click a part, to inspect it.
        </div>
      </div>
    );
  }

  const snapshots = parts[partId];
  const initial = snapshots.initial;
  const active = new Set(activeNodeIds);

  return (
    <div className={styles.inspector}>
      <div className={styles.inspectorTitleRow}>
        <div className={styles.inspectorTitle}>
          {initial.type} &mdash; {initial.id}
        </div>
        {onOpenDetails && (
          <button className={styles.fileMetaBtn} onClick={onOpenDetails}>
            Details…
          </button>
        )}
      </div>

      {initial.sourceText && (
        <div className={styles.inspectorField}>
          <div className={styles.inspectorLabel}>Source</div>
          <div className={styles.inspectorValue}>{initial.sourceText}</div>
        </div>
      )}

      {initial.targetText && (
        <div className={styles.inspectorField}>
          <div className={styles.inspectorLabel}>Target</div>
          <div className={styles.inspectorValue}>{initial.targetText}</div>
        </div>
      )}

      <div className={styles.inspectorField}>
        <div className={styles.inspectorLabel}>Summary</div>
        <div className={styles.inspectorValue}>{initial.summary}</div>
      </div>

      {snapshots.afterNode && (
        <div className={styles.nodeHistory}>
          <div className={styles.inspectorLabel}>Transformations</div>
          {nodes.map((node) => {
            const after = snapshots.afterNode?.[node.id];
            if (!after) return null;
            const isActive = active.has(node.id);
            return (
              <div
                key={node.id}
                className={`${styles.nodeHistoryItem} ${isActive ? styles.nodeHistoryItemActive : ""}`}
              >
                <span className={styles.nodeHistoryLabel}>{node.label}</span>
                <div>
                  <div className={styles.nodeHistoryValue}>{after.summary}</div>
                  {after.targetText && (
                    <div className={styles.nodeHistoryValue}>Target: {after.targetText}</div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
