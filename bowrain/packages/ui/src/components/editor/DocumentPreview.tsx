import { useState, useEffect, useMemo, useRef, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useEditorApi } from "../../hooks/useEditorApi";
import { useStream } from "../../context/StreamContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import type { BlockInfo, SpanInfo } from "../../types/api";
import type { PreviewContentMode } from "./visual-editor-types";
import { getTargetText } from "./blockStatus";
import { pseudoTranslate, pseudoTranslateCoded } from "./pseudoTranslate";
import { cn } from "@neokapi/ui-primitives";

/** How many blocks one page of the preview's document fetch carries. */
const PREVIEW_PAGE_SIZE = 500;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Escape the characters that would otherwise be read as markup. */
function escapeHTML(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

/**
 * Expand coded text into display HTML: each inline-code placeholder takes the
 * data of the span at its position, and literal text is escaped. Targets carry
 * the source's spans — the codes are the same ones, reordered.
 */
function codedToHTML(coded: string, spans: SpanInfo[]): string {
  let result = "";
  let spanIdx = 0;
  for (const ch of coded) {
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

/** Expand a block's source coded text + spans into pseudo-translated display HTML. */
function pseudoBlockToHTML(block: BlockInfo): string {
  const spans = block.source_spans ?? [];
  if (!block.has_spans || !block.source_coded || spans.length === 0) {
    return escapeHTML(pseudoTranslate(block.source));
  }
  return codedToHTML(pseudoTranslateCoded(block.source_coded), spans);
}

/** Expand a block's source coded text + spans into display HTML. */
function sourceBlockToHTML(block: BlockInfo): string {
  const spans = block.source_spans ?? [];
  if (!block.has_spans || !block.source_coded || spans.length === 0) {
    return escapeHTML(block.source);
  }
  return codedToHTML(block.source_coded, spans);
}

/**
 * A locale's target as display HTML, with inline codes expanded through the
 * source's spans. A target without codes is escaped text.
 */
function targetBlockToHTML(block: BlockInfo, locale: string): string {
  const spans = block.source_spans ?? [];
  const coded = block.targets_coded?.[locale] ?? "";
  if (!block.has_spans || !coded || spans.length === 0) {
    return escapeHTML(getTargetText(block, locale));
  }
  return codedToHTML(coded, spans);
}

/**
 * The item's blocks for the preview, in one paged fetch per (item, locale) and
 * cached for as long as the surface keeps asking. The iframe covers the whole
 * document, so this is the document — never a request per block.
 */
function useDocumentBlocks(projectId: string, itemName: string, targetLocale: string) {
  const { getFileBlocks } = useEditorApi();
  const { activeStream } = useStream();
  const { activeWorkspace } = useWorkspace();
  return useQuery({
    queryKey: [
      "preview-blocks",
      activeWorkspace?.slug ?? "",
      projectId,
      activeStream,
      itemName,
      targetLocale,
    ],
    queryFn: async (): Promise<BlockInfo[]> => {
      const all: BlockInfo[] = [];
      for (let offset = 0; ; offset += PREVIEW_PAGE_SIZE) {
        const page = await getFileBlocks(projectId, itemName, {
          locale: targetLocale,
          limit: PREVIEW_PAGE_SIZE,
          offset,
        });
        all.push(...(page ?? []));
        if (!page || page.length < PREVIEW_PAGE_SIZE) break;
      }
      return all;
    },
    staleTime: 30_000,
  });
}

interface DocumentPreviewProps {
  projectId: string;
  itemName: string;
  targetLocale: string;
  selectedBlockId?: string;
  onBlockSelect: (blockId: string) => void;
  /**
   * Blocks the surface already holds — its in-progress edits. They overlay the
   * document the preview fetches for itself, so a save shows immediately
   * without narrowing the preview to whatever the surface has loaded.
   */
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
  const { renderDocumentPreview } = useEditorApi();

  // The document's blocks, plus the surface's edited copies on top.
  const { data: documentBlocks } = useDocumentBlocks(projectId, itemName, targetLocale);
  const contentBlocks = useMemo(() => {
    const byId = new Map((documentBlocks ?? []).map((b) => [b.id, b]));
    for (const b of blocks) byId.set(b.id, b);
    return [...byId.values()];
  }, [documentBlocks, blocks]);

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

  // Push target/source/pseudo content into the iframe when the mode or the
  // blocks change. Every mode sends HTML with the block's inline codes expanded
  // through its spans, so markup like <code> renders in source, pseudo and
  // target alike; a locale with no target falls back to the source.
  useEffect(() => {
    const cw = iframeRef.current?.contentWindow;
    if (!cw || !iframeReady) return;

    for (const block of contentBlocks) {
      const html = showPseudo
        ? pseudoBlockToHTML(block)
        : showTarget && getTargetText(block, targetLocale)
          ? targetBlockToHTML(block, targetLocale)
          : sourceBlockToHTML(block);
      cw.postMessage({ type: "kat-update-block", blockId: block.id, html }, "*");
    }
  }, [showTarget, showPseudo, contentBlocks, targetLocale, iframeReady]);

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
          inlineMode && iframeContentHeight > 0 ? { minHeight: iframeContentHeight } : undefined
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
