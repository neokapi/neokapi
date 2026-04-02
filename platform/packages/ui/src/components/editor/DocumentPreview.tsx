import React, { useState, useEffect, useRef, useCallback } from "react";
import { useEditorApi } from "../../hooks/useEditorApi";
import type { BlockInfo } from "../../types/api";
import type { PreviewContentMode } from "./visual-editor-types";
import { pseudoTranslate, pseudoTranslateCoded } from "./pseudoTranslate";
import { cn } from "@neokapi/ui-primitives";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Expand a block's source coded text + spans into pseudo-translated display HTML. */
function pseudoBlockToHTML(block: BlockInfo): string {
  const spans = block.source_spans ?? [];
  if (!block.has_spans || !block.source_coded || spans.length === 0) {
    const pseudo = pseudoTranslate(block.source);
    return pseudo.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }
  const pseudoCoded = pseudoTranslateCoded(block.source_coded);
  let result = "";
  let spanIdx = 0;
  for (const ch of pseudoCoded) {
    const code = ch.codePointAt(0) ?? 0;
    if (code === 0xe001 || code === 0xe002 || code === 0xe003) {
      const span = spans[spanIdx++];
      if (span) result += span.data;
    } else if (ch === "&") {
      result += "&amp;";
    } else if (ch === "<") {
      result += "&lt;";
    } else if (ch === ">") {
      result += "&gt;";
    } else {
      result += ch;
    }
  }
  return result;
}

/** Expand a block's source coded text + spans into display HTML. */
function sourceBlockToHTML(block: BlockInfo): string {
  const spans = block.source_spans ?? [];
  if (!block.has_spans || !block.source_coded || spans.length === 0) {
    return block.source.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }
  let result = "";
  let spanIdx = 0;
  for (const ch of block.source_coded) {
    const code = ch.codePointAt(0) ?? 0;
    if (code === 0xe001 || code === 0xe002 || code === 0xe003) {
      const span = spans[spanIdx++];
      if (span) result += span.data;
    } else if (ch === "&") {
      result += "&amp;";
    } else if (ch === "<") {
      result += "&lt;";
    } else if (ch === ">") {
      result += "&gt;";
    } else {
      result += ch;
    }
  }
  return result;
}

interface DocumentPreviewProps {
  projectId: string;
  itemName: string;
  targetLocale: string;
  selectedBlockId?: string;
  onBlockSelect: (blockId: string) => void;
  blocks?: BlockInfo[];
  previewContentMode?: PreviewContentMode;
  // Inline mode props
  spacerHeight?: number;
  onContentHeight?: (h: number) => void;
  onSpacerPosition?: (y: number) => void;
}

export function DocumentPreview({
  projectId,
  itemName,
  targetLocale,
  selectedBlockId,
  onBlockSelect,
  blocks = [],
  previewContentMode,
  spacerHeight,
  onContentHeight,
  onSpacerPosition,
}: DocumentPreviewProps) {
  const [previewHTML, setPreviewHTML] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [iframeReady, setIframeReady] = useState(false);
  const [internalMode, setInternalMode] = useState<"source" | "target">("source");
  const [hovered, setHovered] = useState(false);
  const [iframeContentHeight, setIframeContentHeight] = useState<number>(0);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const { renderDocumentPreview, renderBlockHTML } = useEditorApi();

  // Inline mode: spacerHeight prop is provided
  const inlineMode = spacerHeight !== undefined;

  // Determine effective mode: controlled via prop, or internal toggle
  const isControlled = previewContentMode !== undefined;
  const showTarget = isControlled ? previewContentMode === "target" : internalMode === "target";
  const showPseudo = isControlled && previewContentMode === "pseudo";

  // Use refs for callback props to avoid re-running effects when they change
  const onBlockSelectRef = useRef(onBlockSelect);
  onBlockSelectRef.current = onBlockSelect;
  const onContentHeightRef = useRef(onContentHeight);
  onContentHeightRef.current = onContentHeight;
  const onSpacerPositionRef = useRef(onSpacerPosition);
  onSpacerPositionRef.current = onSpacerPosition;

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

  // Listen for block clicks, iframe ready signal, and content height / spacer position
  useEffect(() => {
    const handleMessage = (e: MessageEvent) => {
      if (e.data?.type === "kat-block-click" && e.data.blockId) {
        onBlockSelectRef.current(e.data.blockId);
      }
      if (e.data?.type === "kat-iframe-ready") {
        setIframeReady(true);
      }
      if (e.data?.type === "kat-content-height" && typeof e.data.height === "number") {
        setIframeContentHeight(e.data.height);
        onContentHeightRef.current?.(e.data.height);
      }
      if (e.data?.type === "kat-spacer-position" && typeof e.data.y === "number") {
        onSpacerPositionRef.current?.(e.data.y);
        if (typeof e.data.contentHeight === "number") {
          setIframeContentHeight(e.data.contentHeight);
          onContentHeightRef.current?.(e.data.contentHeight);
        }
      }
    };
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, []);

  // Fallback: mark ready on iframe load (for previews without kat-iframe-ready)
  const handleIframeLoad = useCallback(() => {
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

  // Send spacer insert/remove messages in inline mode
  useEffect(() => {
    const cw = iframeRef.current?.contentWindow;
    if (!cw || !iframeReady) return;

    if (inlineMode && selectedBlockId && spacerHeight > 0) {
      cw.postMessage(
        { type: "kat-insert-spacer", blockId: selectedBlockId, height: spacerHeight },
        "*",
      );
    } else {
      cw.postMessage({ type: "kat-remove-spacer" }, "*");
    }
  }, [selectedBlockId, spacerHeight, iframeReady, inlineMode]);

  // Push target/source/pseudo content into the iframe when mode or blocks change.
  // Source content is sent as HTML (with spans expanded) so inline markup like
  // <code> is preserved. Pseudo mode renders source text through the accent map
  // client-side. Target content is sent as plain text.
  useEffect(() => {
    const cw = iframeRef.current?.contentWindow;
    if (!cw || !iframeReady) return;

    for (const block of blocks) {
      if (showPseudo) {
        // Pseudo view: accent-map the source text on the fly
        cw.postMessage(
          { type: "kat-update-block", blockId: block.id, html: pseudoBlockToHTML(block) },
          "*",
        );
      } else if (showTarget && block.targets[targetLocale]) {
        cw.postMessage(
          { type: "kat-update-block", blockId: block.id, text: block.targets[targetLocale] },
          "*",
        );
      } else {
        cw.postMessage(
          { type: "kat-update-block", blockId: block.id, html: sourceBlockToHTML(block) },
          "*",
        );
      }
    }
  }, [showTarget, showPseudo, blocks, targetLocale, iframeReady]);

  // Use renderBlockHTML for richer block content when available (target mode only)
  useEffect(() => {
    const cw = iframeRef.current?.contentWindow;
    if (!cw || !iframeReady || !showTarget || showPseudo || !renderBlockHTML) return;

    let cancelled = false;
    for (const block of blocks) {
      if (block.targets[targetLocale]) {
        renderBlockHTML(projectId, block.id, targetLocale)
          .then((html) => {
            if (!cancelled) {
              cw.postMessage({ type: "kat-update-block", blockId: block.id, html }, "*");
            }
          })
          .catch(() => {
            /* fall back to plain text already sent */
          });
      }
    }
    return () => {
      cancelled = true;
    };
  }, [showTarget, showPseudo, blocks, targetLocale, iframeReady, renderBlockHTML, projectId]);

  if (loading) {
    return (
      <div
        className="flex items-center justify-center h-full text-[var(--text-secondary)] text-sm"
        data-testid="preview-loading"
      >
        Loading preview...
      </div>
    );
  }

  if (!previewHTML) {
    return (
      <div
        className="flex items-center justify-center h-full text-[var(--text-secondary)] text-sm"
        data-testid="preview-empty"
      >
        No preview available
      </div>
    );
  }

  return (
    <div
      className="relative w-full h-full"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <iframe
        ref={iframeRef}
        srcDoc={previewHTML}
        className="w-full h-full border border-[var(--border)] rounded-lg bg-white"
        style={
          inlineMode && iframeContentHeight > 0
            ? { minHeight: iframeContentHeight }
            : undefined
        }
        sandbox="allow-scripts"
        title="Document Preview"
        data-testid="preview-iframe"
        onLoad={handleIframeLoad}
      />
      {!isControlled && (
        <div
          className={cn(
            "absolute top-2 right-2 flex gap-1 transition-opacity duration-200",
            hovered ? "opacity-100 pointer-events-auto" : "opacity-0 pointer-events-none",
          )}
          data-testid="preview-overlay"
        >
          <button
            onClick={() => setInternalMode(internalMode === "source" ? "target" : "source")}
            className="px-3 py-1 text-white border-none rounded text-xs font-semibold cursor-pointer shadow-[0_1px_4px_rgba(0,0,0,0.3)]"
            style={{
              backgroundColor: internalMode === "target" ? "var(--accent)" : "rgba(30,30,46,0.85)",
            }}
            data-testid="preview-target-toggle"
          >
            {internalMode === "target" ? "Target" : "Source"}
          </button>
        </div>
      )}
    </div>
  );
}
