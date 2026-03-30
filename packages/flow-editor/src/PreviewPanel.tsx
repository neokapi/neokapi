import { useState, useCallback, useRef } from "react";
import { Eye, Loader2 } from "lucide-react";
import type { PartSnapshotSet } from "./traceTypes";
import { theme } from "./theme";

interface PreviewPanelProps {
  /** Called to run a preview with sample text. Returns per-node snapshots. */
  onPreview: (sampleText: string, sourceLang: string, targetLang: string) => Promise<PreviewResult>;
  sourceLang?: string;
  targetLang?: string;
  /** Node names keyed by ID. */
  nodeNames: Map<string, string>;
}

export interface PreviewResult {
  parts: Record<string, PartSnapshotSet>;
  nodeOrder: string[]; // ordered node IDs
}

/**
 * Live preview panel — type sample text and see how each tool transforms it.
 */
export function PreviewPanel({
  onPreview,
  sourceLang = "en-US",
  targetLang = "fr-FR",
  nodeNames,
}: PreviewPanelProps) {
  const [sampleText, setSampleText] = useState("");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<PreviewResult | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();

  const handlePreview = useCallback(async () => {
    if (!sampleText.trim()) return;
    setLoading(true);
    try {
      const res = await onPreview(sampleText, sourceLang, targetLang);
      setResult(res);
    } finally {
      setLoading(false);
    }
  }, [sampleText, sourceLang, targetLang, onPreview]);

  const handleTextChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      setSampleText(e.target.value);
      clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => {
        // Auto-preview on debounce if text is non-empty
        if (e.target.value.trim()) {
          handlePreview();
        }
      }, 500);
    },
    [handlePreview],
  );

  // Find the first block part for display.
  const blockPart = result
    ? Object.entries(result.parts).find(([, ss]) => ss.initial.type === "Block")
    : null;
  const blockId = blockPart?.[0];
  const snapshots = blockPart?.[1];

  return (
    <div
      style={{
        borderTop: `1px solid ${theme.border}`,
        background: theme.bg,
        padding: "8px 12px",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 8 }}>
        <Eye size={12} style={{ color: theme.accent }} />
        <span style={{ fontSize: 11, fontWeight: 600, color: theme.fg }}>Preview</span>
      </div>

      {/* Input area */}
      <div style={{ display: "flex", gap: 6, marginBottom: 8 }}>
        <textarea
          value={sampleText}
          onChange={handleTextChange}
          placeholder="Enter sample text to preview..."
          rows={2}
          style={{
            flex: 1,
            padding: "6px 8px",
            borderRadius: 4,
            border: `1px solid ${theme.border}`,
            background: theme.bgCard,
            color: theme.fg,
            fontSize: 11,
            fontFamily: "inherit",
            resize: "none",
            outline: "none",
          }}
        />
        <button
          onClick={handlePreview}
          disabled={loading || !sampleText.trim()}
          style={{
            padding: "4px 10px",
            borderRadius: 4,
            border: "none",
            background: theme.accent,
            color: theme.accentFg,
            fontSize: 11,
            fontWeight: 600,
            cursor: "pointer",
            opacity: loading || !sampleText.trim() ? 0.5 : 1,
            alignSelf: "flex-start",
          }}
        >
          {loading ? <Loader2 size={12} style={{ animation: "spin 1s linear infinite" }} /> : "Run"}
        </button>
      </div>

      {/* Per-node results */}
      {snapshots && (
        <div
          style={{
            display: "flex",
            gap: 0,
            overflowX: "auto",
            paddingBottom: 4,
            alignItems: "center",
          }}
        >
          {/* Initial state */}
          <PreviewCard
            label="Input"
            sourceText={snapshots.initial.sourceText ?? ""}
            targetText=""
            isFirst
            index={0}
          />

          {/* After each node */}
          {result!.nodeOrder.map((nodeId, i) => {
            const after = snapshots.afterNode?.[nodeId];
            if (!after) return null;
            const prevNodeId = result!.nodeOrder[result!.nodeOrder.indexOf(nodeId) - 1];
            const prevTarget = prevNodeId
              ? (snapshots.afterNode?.[prevNodeId]?.targetText ?? "")
              : "";
            const changed = after.targetText !== prevTarget;

            return (
              <div key={nodeId} style={{ display: "flex", alignItems: "center", gap: 0 }}>
                <span
                  style={{ fontSize: 14, color: theme.fgMuted, padding: "0 4px", flexShrink: 0 }}
                >
                  &rarr;
                </span>
                <PreviewCard
                  label={nodeNames.get(nodeId) ?? nodeId}
                  sourceText={after.sourceText ?? ""}
                  targetText={after.targetText ?? ""}
                  changed={changed}
                  index={i + 1}
                />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function PreviewCard({
  label,
  sourceText,
  targetText,
  changed,
  isFirst,
  index = 0,
}: {
  label: string;
  sourceText: string;
  targetText: string;
  changed?: boolean;
  isFirst?: boolean;
  index?: number;
}) {
  return (
    <div
      style={{
        minWidth: 140,
        maxWidth: 200,
        padding: 8,
        borderRadius: 6,
        border: `1px solid ${changed ? theme.accent : theme.border}`,
        borderLeft: changed || isFirst ? `3px solid ${theme.accent}` : `1px solid ${theme.border}`,
        background: theme.bgCard,
        flexShrink: 0,
        animation: "cardReveal 0.25s ease-out forwards",
        animationDelay: `${index * 60}ms`,
        opacity: 0,
      }}
    >
      <div
        style={{
          fontSize: 9,
          fontWeight: 600,
          color: theme.fgMuted,
          marginBottom: 4,
          textTransform: "uppercase",
          letterSpacing: "0.04em",
        }}
      >
        {label}
      </div>
      {sourceText && (
        <div style={{ fontSize: 10, color: theme.fg, lineHeight: 1.3, marginBottom: 2 }}>
          {sourceText.length > 60 ? sourceText.slice(0, 60) + "..." : sourceText}
        </div>
      )}
      {!isFirst && (
        <div
          style={{
            fontSize: 10,
            color: changed ? theme.accent : theme.fgMuted,
            lineHeight: 1.3,
            fontWeight: changed ? 500 : 400,
          }}
        >
          {targetText
            ? targetText.length > 60
              ? targetText.slice(0, 60) + "..."
              : targetText
            : "(no target)"}
        </div>
      )}
    </div>
  );
}
