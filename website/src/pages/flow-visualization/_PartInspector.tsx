import React from 'react';
import type { PartSnapshotSet, FlowNode } from './_types';
import styles from './_index.module.css';

interface PartInspectorProps {
  partId: string | null;
  parts: Record<string, PartSnapshotSet>;
  nodes: FlowNode[];
}

export default function PartInspector({
  partId,
  parts,
  nodes,
}: PartInspectorProps): React.ReactElement {
  if (!partId || !parts[partId]) {
    return (
      <div className={styles.inspector}>
        <div className={styles.inspectorHint}>Click a particle to inspect</div>
      </div>
    );
  }

  const snapshots = parts[partId];
  const initial = snapshots.initial;

  return (
    <div className={styles.inspector}>
      <div className={styles.inspectorTitle}>
        {initial.type} &mdash; {initial.id}
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
          {nodes.map(node => {
            const after = snapshots.afterNode?.[node.id];
            if (!after) return null;
            return (
              <div key={node.id} className={styles.nodeHistoryItem}>
                <span className={styles.nodeHistoryLabel}>{node.label}</span>
                <div>
                  <div className={styles.nodeHistoryValue}>{after.summary}</div>
                  {after.targetText && (
                    <div className={styles.nodeHistoryValue}>
                      Target: {after.targetText}
                    </div>
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
