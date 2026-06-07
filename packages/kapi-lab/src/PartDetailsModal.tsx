import React, { useEffect, useState } from "react";
import { X } from "lucide-react";
import type { FlowNode, PartSnapshot, PartSnapshotSet } from "@neokapi/ui-primitives/preview";
import { RunSequence } from "@neokapi/ui-primitives/preview";
import styles from "./PartDetailsModal.module.css";

export interface PartDetailsModalProps {
  /** The selected part's snapshot set, or null to render nothing. */
  set: PartSnapshotSet | null;
  /** Pipeline nodes in order, used to label and order the per-stage history. */
  nodes: FlowNode[];
  onClose: () => void;
}

// PartDetailsModal is the "drill into a part" view: the full content model of a
// Block — its source and every target locale as run sequences (inline codes
// preserved), its properties, how it changed at each pipeline stage, and the raw
// snapshot JSON. Opened from the inspector; the flow graph stays put underneath.
export default function PartDetailsModal({
  set,
  nodes,
  onClose,
}: PartDetailsModalProps): React.ReactElement | null {
  const [showRaw, setShowRaw] = useState(false);

  // Close on Escape; lock body scroll while open.
  useEffect(() => {
    if (!set) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener("keydown", onKey, true);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey, true);
      document.body.style.overflow = prevOverflow;
    };
  }, [set, onClose]);

  if (!set) return null;

  const { initial } = set;
  // The part's final state = the last node (in pipeline order) that snapshotted
  // it, falling back to the initial read.
  let finalSnap: PartSnapshot = initial;
  for (let i = nodes.length - 1; i >= 0; i--) {
    const s = set.afterNode?.[nodes[i].id];
    if (s) {
      finalSnap = s;
      break;
    }
  }
  const detail = finalSnap.detail ?? initial.detail;

  // Pipeline stages: the reader (initial) then each node that touched the part.
  const stages: { label: string; snap: PartSnapshot }[] = [{ label: "read", snap: initial }];
  for (const n of nodes) {
    const s = set.afterNode?.[n.id];
    if (s) stages.push({ label: n.label, snap: s });
  }

  const properties = detail?.properties ?? {};
  const propertyKeys = Object.keys(properties);

  return (
    <div className={styles.overlay} onClick={onClose} role="presentation">
      <div
        className={styles.modal}
        role="dialog"
        aria-modal="true"
        aria-label={`Part ${initial.id} details`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className={styles.header}>
          <div>
            <span className={styles.kind}>{initial.type}</span>
            <span className={styles.id}>{initial.id}</span>
            {detail?.translatable && <span className={styles.badge}>translatable</span>}
          </div>
          <button className={styles.close} onClick={onClose} aria-label="Close">
            <X size={16} />
          </button>
        </div>

        <div className={styles.body}>
          {detail ? (
            <>
              <Section title="Source">
                <RunSequence runs={detail.source ?? []} />
              </Section>

              {detail.targets && Object.keys(detail.targets).length > 0 && (
                <Section title="Targets">
                  {Object.entries(detail.targets).map(([loc, runs]) => (
                    <div key={loc} className={styles.localeRow}>
                      <span className={styles.locale}>{loc}</span>
                      <RunSequence runs={runs} />
                    </div>
                  ))}
                </Section>
              )}

              {propertyKeys.length > 0 && (
                <Section title="Properties">
                  <table className={styles.props}>
                    <tbody>
                      {propertyKeys.map((k) => (
                        <tr key={k}>
                          <td className={styles.propKey}>{k}</td>
                          <td className={styles.propVal}>{properties[k]}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </Section>
              )}
            </>
          ) : (
            <Section title="Summary">
              <div className={styles.mono}>{initial.summary}</div>
            </Section>
          )}

          <Section title="Through the pipeline">
            <table className={styles.stages}>
              <thead>
                <tr>
                  <th>Stage</th>
                  <th>Source</th>
                  <th>Target</th>
                </tr>
              </thead>
              <tbody>
                {stages.map((s, i) => (
                  <tr key={i}>
                    <td className={styles.stageLabel}>{s.label}</td>
                    <td className={styles.mono}>
                      {s.snap.sourceText || <span className={styles.muted}>—</span>}
                    </td>
                    <td className={styles.mono}>
                      {s.snap.targetText || <span className={styles.muted}>—</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Section>

          <Section title="Raw">
            <button className={styles.rawToggle} onClick={() => setShowRaw((v) => !v)}>
              {showRaw ? "Hide" : "Show"} snapshot JSON
            </button>
            {showRaw && <pre className={styles.raw}>{JSON.stringify(set, null, 2)}</pre>}
          </Section>
        </div>
      </div>
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <section className={styles.section}>
      <h4 className={styles.sectionTitle}>{title}</h4>
      {children}
    </section>
  );
}
