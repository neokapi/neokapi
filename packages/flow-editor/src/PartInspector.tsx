import type { PartSnapshotSet } from "./traceTypes";
import { theme } from "./theme";

interface PartInspectorProps {
  nodeId: string;
  nodeName: string;
  /** All part snapshot sets from the trace. */
  parts: Record<string, PartSnapshotSet>;
}

/**
 * Part inspector sidebar — shows blocks that passed through a node,
 * with before/after source and target text.
 */
export function PartInspector({ nodeId, nodeName, parts }: PartInspectorProps) {
  // Filter to Block-type parts that have snapshots for this node.
  const relevantParts = Object.entries(parts).filter(
    ([, ss]) => ss.initial.type === "Block" && ss.afterNode?.[nodeId],
  );

  return (
    <div
      style={{
        width: 300,
        display: "flex",
        flexDirection: "column",
        borderLeft: `1px solid ${theme.border}`,
        background: theme.bg,
        overflow: "hidden",
      }}
    >
      <div
        style={{
          padding: "10px 12px",
          borderBottom: `1px solid ${theme.border}`,
        }}
      >
        <div style={{ fontSize: 11, fontWeight: 600, color: theme.fg }}>
          Part Inspector
        </div>
        <div style={{ fontSize: 10, color: theme.fgMuted, marginTop: 2 }}>
          {nodeName} &mdash; {relevantParts.length} block{relevantParts.length !== 1 ? "s" : ""}
        </div>
      </div>

      <div style={{ flex: 1, overflow: "auto", padding: "8px 12px" }}>
        {relevantParts.length === 0 && (
          <div style={{ fontSize: 11, color: theme.fgMuted, textAlign: "center", padding: "20px 0", fontStyle: "italic" }}>
            No block data for this node.
          </div>
        )}

        {relevantParts.map(([partId, ss]) => {
          const before = ss.initial;
          const after = ss.afterNode![nodeId];
          const sourceChanged = before.sourceText !== after.sourceText;
          const targetChanged = before.targetText !== after.targetText;

          return (
            <div
              key={partId}
              style={{
                marginBottom: 10,
                padding: 8,
                borderRadius: 6,
                border: `1px solid ${theme.border}`,
              }}
            >
              <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 6 }}>
                {before.summary}
              </div>

              {/* Source text */}
              {before.sourceText && (
                <div style={{ marginBottom: 4 }}>
                  <div style={{ fontSize: 9, color: theme.fgMuted, fontWeight: 600, marginBottom: 2 }}>
                    SOURCE
                  </div>
                  <div style={{ fontSize: 11, color: theme.fg, lineHeight: 1.4 }}>
                    {after.sourceText || before.sourceText}
                    {sourceChanged && (
                      <span style={{ fontSize: 9, color: theme.accent, marginLeft: 4 }}>changed</span>
                    )}
                  </div>
                </div>
              )}

              {/* Target text */}
              <div>
                <div style={{ fontSize: 9, color: theme.fgMuted, fontWeight: 600, marginBottom: 2 }}>
                  TARGET
                </div>
                {before.targetText || after.targetText ? (
                  <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                    {before.targetText && before.targetText !== after.targetText && (
                      <div
                        style={{
                          fontSize: 11,
                          color: theme.fgMuted,
                          textDecoration: "line-through",
                          opacity: 0.6,
                        }}
                      >
                        {before.targetText}
                      </div>
                    )}
                    <div style={{ fontSize: 11, color: targetChanged ? theme.accent : theme.fg, lineHeight: 1.4 }}>
                      {after.targetText || "(empty)"}
                    </div>
                  </div>
                ) : (
                  <div style={{ fontSize: 11, color: theme.fgMuted, fontStyle: "italic" }}>
                    (no target)
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
