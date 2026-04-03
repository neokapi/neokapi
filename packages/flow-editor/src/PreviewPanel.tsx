import { useState, useCallback, useRef } from "react";
import { Eye, Loader2 } from "lucide-react";
import { cn, Button, Textarea, PanelHeader } from "@neokapi/ui-primitives";
import type { PartSnapshotSet } from "./traceTypes";

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

/** Single preview card showing source/target text after a node. */
function PreviewSegment({
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
      className={cn(
        "min-w-[140px] max-w-[200px] shrink-0 rounded-md border bg-card p-2 opacity-0",
        changed ? "border-accent" : "border-border",
        changed || isFirst ? "border-l-[3px] border-l-accent" : "",
      )}
      style={{
        animation: "cardReveal 0.25s ease-out forwards",
        animationDelay: `${index * 60}ms`,
      }}
    >
      <div className="mb-1 text-[9px] font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      {sourceText && (
        <div className="mb-0.5 text-[10px] leading-tight text-foreground">
          {sourceText.length > 60 ? sourceText.slice(0, 60) + "..." : sourceText}
        </div>
      )}
      {!isFirst && (
        <div
          className={cn(
            "text-[10px] leading-tight",
            changed ? "font-medium text-accent-foreground" : "text-muted-foreground",
          )}
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
  const _blockId = blockPart?.[0];
  const snapshots = blockPart?.[1];

  return (
    <div className="border-t border-border bg-background">
      <PanelHeader className="border-b-0">
        <Eye size={12} className="text-accent-foreground" />
        <span className="text-[11px] font-semibold text-foreground">Preview</span>
      </PanelHeader>

      {/* Input area */}
      <div className="px-3 pb-2 flex gap-1.5">
        <Textarea
          value={sampleText}
          onChange={handleTextChange}
          placeholder="Enter sample text to preview..."
          rows={2}
          className="min-h-0 flex-1 resize-none rounded border-border bg-card py-1.5 px-2 text-[11px]"
        />
        <Button
          onClick={handlePreview}
          disabled={loading || !sampleText.trim()}
          size="sm"
          className="self-start"
        >
          {loading ? <Loader2 size={12} style={{ animation: "spin 1s linear infinite" }} /> : "Run"}
        </Button>
      </div>

      {/* Per-node results */}
      {snapshots && (
        <div className="flex items-center gap-0 overflow-x-auto pb-1">
          {/* Initial state */}
          <PreviewSegment
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
              <div key={nodeId} className="flex items-center gap-0">
                <span className="shrink-0 px-1 text-sm text-muted-foreground">&rarr;</span>
                <PreviewSegment
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
