import { useState, useEffect, useRef, useCallback } from "react";
import type { BlockInfo, Run } from "../types/api";
import { useEditorApi } from "../hooks/useApi";

/**
 * Flatten an RFC 0001 Run sequence to plain display text. Text runs
 * contribute their text; placeholders / paired codes / subs contribute
 * their equiv label so the preview shows a readable approximation;
 * plural / select fall back to the "other" form.
 */
function flattenRuns(runs: Run[] | undefined): string {
  if (!runs) return "";
  let out = "";
  for (const r of runs) {
    if (r.text) out += r.text.text;
    else if (r.ph) out += r.ph.equiv || r.ph.id;
    else if (r.sub) out += r.sub.equiv || r.sub.ref;
    else if (r.plural) out += flattenRuns(r.plural.forms.other);
    else if (r.select) out += flattenRuns(r.select.cases.other);
    // pcOpen / pcClose carry no visible text in a plain preview.
  }
  return out;
}

interface DocumentPreviewProps {
  projectId: string;
  itemName: string;
  targetLocale: string;
  selectedBlockId?: string;
  onBlockSelect: (blockId: string) => void;
  blocks?: BlockInfo[];
}

export function DocumentPreview({
  projectId,
  itemName,
  targetLocale,
  selectedBlockId,
  onBlockSelect,
  blocks = [],
}: DocumentPreviewProps) {
  const [previewHTML, setPreviewHTML] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [iframeReady, setIframeReady] = useState(false);
  const [showTarget, setShowTarget] = useState(false);
  const [hovered, setHovered] = useState(false);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const { renderDocumentPreview } = useEditorApi();

  // Use refs for callback props to avoid re-running effects when they change
  const onBlockSelectRef = useRef(onBlockSelect);
  onBlockSelectRef.current = onBlockSelect;

  // Load preview HTML
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setIframeReady(false);
    renderDocumentPreview(projectId, itemName, targetLocale)
      .then((html) => {
        if (!cancelled) {
          setPreviewHTML(html);
          setLoading(false);
        }
      })
      .catch(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [renderDocumentPreview, projectId, itemName, targetLocale]);

  // Listen for block clicks and iframe ready signal
  useEffect(() => {
    const handleMessage = (e: MessageEvent) => {
      if (e.data?.type === "kat-block-click" && e.data.blockId) {
        onBlockSelectRef.current(e.data.blockId);
      }
      if (e.data?.type === "kat-iframe-ready") {
        setIframeReady(true);
      }
    };
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, []);

  // Fallback: mark ready on iframe load (for previews without kat-iframe-ready)
  const handleIframeLoad = useCallback(() => {
    // Small delay to let scripts in the iframe execute
    setTimeout(() => setIframeReady(true), 50);
  }, []);

  // Send selection to iframe when selectedBlockId changes
  useEffect(() => {
    if (!iframeRef.current?.contentWindow || !selectedBlockId || !iframeReady) return;
    iframeRef.current.contentWindow.postMessage(
      { type: "kat-select-block", blockId: selectedBlockId },
      "*",
    );
  }, [selectedBlockId, iframeReady]);

  // Push target/source text into the iframe when showTarget or blocks change
  useEffect(() => {
    const cw = iframeRef.current?.contentWindow;
    if (!cw || !iframeReady) return;

    for (const block of blocks) {
      const targetRuns = block.targetRuns?.[targetLocale];
      const html =
        showTarget && targetRuns && targetRuns.length > 0
          ? flattenRuns(targetRuns)
          : flattenRuns(block.sourceRuns);
      cw.postMessage({ type: "kat-update-block", blockId: block.id, html }, "*");
    }
  }, [showTarget, blocks, targetLocale, iframeReady]);

  if (loading) {
    return (
      <div style={loadingStyle} data-testid="preview-loading">
        Loading preview...
      </div>
    );
  }

  if (!previewHTML) {
    return (
      <div style={emptyStyle} data-testid="preview-empty">
        No preview available
      </div>
    );
  }

  return (
    <div
      style={containerStyle}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <iframe
        ref={iframeRef}
        srcDoc={previewHTML}
        style={iframeStyle}
        sandbox="allow-scripts"
        title="Document Preview"
        data-testid="preview-iframe"
        onLoad={handleIframeLoad}
      />
      <div
        style={{
          ...overlayStyle,
          opacity: hovered ? 1 : 0,
          pointerEvents: hovered ? "auto" : "none",
        }}
        data-testid="preview-overlay"
      >
        <button
          onClick={() => setShowTarget(!showTarget)}
          style={{
            ...toggleBtnStyle,
            backgroundColor: showTarget ? "var(--accent)" : "rgba(30,30,46,0.85)",
          }}
          data-testid="preview-target-toggle"
        >
          {showTarget ? "Target" : "Source"}
        </button>
      </div>
    </div>
  );
}

const containerStyle: React.CSSProperties = {
  position: "relative",
  width: "100%",
  height: "100%",
};

const iframeStyle: React.CSSProperties = {
  width: "100%",
  height: "100%",
  border: "1px solid var(--border)",
  borderRadius: 8,
  backgroundColor: "#fff",
};

const overlayStyle: React.CSSProperties = {
  position: "absolute",
  top: 8,
  right: 8,
  transition: "opacity 0.2s ease",
  display: "flex",
  gap: 4,
};

const toggleBtnStyle: React.CSSProperties = {
  padding: "4px 12px",
  color: "#fff",
  border: "none",
  borderRadius: 4,
  fontSize: 12,
  fontWeight: 600,
  cursor: "pointer",
  boxShadow: "0 1px 4px rgba(0,0,0,0.3)",
};

const loadingStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: "var(--text-secondary)",
  fontSize: 14,
};

const emptyStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: "var(--text-secondary)",
  fontSize: 14,
};
