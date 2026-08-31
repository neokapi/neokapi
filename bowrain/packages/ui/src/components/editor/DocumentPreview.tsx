import { useState, useEffect, useLayoutEffect, useMemo, useRef, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { FormatPreview, extOf } from "@neokapi/ui-primitives/preview";
import type { BlockAttrs } from "@neokapi/ui-primitives/preview";
import { useEditorApi } from "../../hooks/useEditorApi";
import { useLocales } from "../../hooks/useLocales";
import { useStream } from "../../context/StreamContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import type { BlockInfo, SpanInfo } from "../../types/api";
import type { PreviewContentMode } from "./visual-editor-types";
import { getTargetText } from "./blockStatus";
import { pseudoTranslate, pseudoTranslateCoded } from "./pseudoTranslate";
import { blocksToContentTree } from "../../preview/toContentTree";
import { cn } from "@neokapi/ui-primitives";
import { Languages } from "../icons";

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
 * The id this block answers to *inside the rendered document*.
 *
 * Two id spaces meet at the iframe boundary. The store mints its own id when a
 * block is ingested and keeps the reader's as `source_id`; the document is
 * rendered by the format's own preview builder, which marks its blocks with the
 * reader's id. So a `<kat-block>` in the document and the block object beside it
 * are the same content under two different names.
 *
 * Every message crossing that boundary has to be spoken in the document's
 * dialect. Nothing enforces it — `postMessage` has no delivery receipt and a
 * `querySelector` that finds nothing is not an error — so a mismatch is
 * completely silent: the document keeps rendering its source while the surface
 * believes it has been updated, which reads as the translation having gone
 * missing rather than as a message that landed nowhere.
 *
 * The fallback is not defensive padding. A document the server built from the
 * stored blocks themselves (`core/editor`'s generic preview, used when no reader
 * supplied one) marks them with the store's ids and carries no `source_id`, and
 * there the two spaces are already one.
 */
export function documentIdOf(block: BlockInfo): string {
  return block.source_id || block.id;
}

/** Both directions of the block ↔ document id translation, built once. */
export function idTranslation(blocks: BlockInfo[]) {
  const toDocument = new Map<string, string>();
  const toBlock = new Map<string, string>();
  for (const b of blocks) {
    const docId = documentIdOf(b);
    toDocument.set(b.id, docId);
    toBlock.set(docId, b.id);
  }
  return { toDocument, toBlock };
}

/**
 * The item's blocks for the preview, in one paged fetch per (item, locale) and
 * cached for as long as the surface keeps asking. The iframe covers the whole
 * document, so this is the document — never a request per block.
 */
export function useDocumentBlocks(projectId: string, itemName: string, targetLocale: string) {
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

/**
 * The marker `core/editor`'s fallback preview builders put on their container:
 * the response is a plain listing of the item's blocks, not the format's own
 * rendering, because no reader supplied PreviewHTML for it. See
 * `genericPreviewOpen` in core/editor/preview_generic.go.
 */
const GENERIC_PREVIEW_MARKER = 'data-kat-preview="generic"';

/** The synthetic locale the pseudo-translated reading is projected under. */
const PSEUDO_LOCALE = "__pseudo";

/**
 * ContentPreview renders the item from the content model rather than from
 * server HTML — the reading used when the server has no format-aware preview to
 * give. The shared preview kit lays the blocks out by their structure and marks
 * their entities and findings inline, which a monospace listing of expanded
 * coded text cannot do.
 *
 * It keeps the iframe path's contract with the visual editor: clicking a block
 * selects it, and in inline mode the selected block opens a gap the editor card
 * drops into, with the gap's position and the document's height reported back.
 */
function ContentPreview({
  blocks,
  itemName,
  sourceLocale,
  targetLocale,
  side,
  selectedBlockId,
  onBlockSelect,
  spacerHeight,
  onContentHeight,
  onSpacerPosition,
}: {
  blocks: BlockInfo[];
  itemName: string;
  sourceLocale?: string;
  targetLocale: string;
  side: "source" | "target" | "pseudo";
  selectedBlockId?: string;
  onBlockSelect: (blockId: string) => void;
  spacerHeight?: number;
  onContentHeight?: (h: number) => void;
  onSpacerPosition?: (y: number) => void;
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  // The element currently holding the editor gap, so it can be released when
  // the selection moves.
  const spacedRef = useRef<HTMLElement | null>(null);

  // Pseudo-translation is generated here, so it travels as one more target
  // locale the kit can render — the projection stays the only place a block
  // becomes a content node.
  const projected = useMemo(() => {
    if (side !== "pseudo") return blocks;
    return blocks.map((block) => ({
      ...block,
      targets: { ...block.targets, [PSEUDO_LOCALE]: pseudoTranslate(block.source) },
    }));
  }, [blocks, side]);

  const tree = useMemo(
    () => blocksToContentTree(projected, { format: extOf(itemName), name: itemName, sourceLocale }),
    [projected, itemName, sourceLocale],
  );

  const blockAttrs = useCallback(
    (id: string): BlockAttrs => ({ "data-testid": `preview-block-${id}` }),
    [],
  );

  const report = useCallback(() => {
    const root = rootRef.current;
    if (!root) return;
    onContentHeight?.(root.scrollHeight);
  }, [onContentHeight]);

  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    if (spacedRef.current) {
      spacedRef.current.style.marginBottom = "";
      spacedRef.current = null;
    }
    const el =
      selectedBlockId && spacerHeight
        ? root.querySelector<HTMLElement>(`[data-block-id="${CSS.escape(selectedBlockId)}"]`)
        : null;
    if (el && spacerHeight) {
      el.style.marginBottom = `${spacerHeight}px`;
      spacedRef.current = el;
      const top = el.getBoundingClientRect().bottom - root.getBoundingClientRect().top;
      onSpacerPosition?.(top + root.scrollTop);
    }
    report();
  }, [selectedBlockId, spacerHeight, onSpacerPosition, report, tree]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(report);
    observer.observe(root);
    return () => observer.disconnect();
  }, [report]);

  return (
    <div
      ref={rootRef}
      // The container is the document's own surface, so it takes the colour the
      // preview kit paints its page with. Left as the app's card, a document
      // shorter than the pane showed the card below the page — invisible in a
      // light theme, a hard edge in a dark one.
      className="relative h-full w-full overflow-auto rounded-lg border border-border bg-[var(--kapi-preview-bg,#fff)] px-6 py-5"
      data-testid="preview-content"
    >
      <FormatPreview
        tree={tree}
        side={side === "source" ? "source" : side === "pseudo" ? PSEUDO_LOCALE : targetLocale}
        onSelectBlock={onBlockSelect}
        selectedBlockId={selectedBlockId}
        blockAttrs={blockAttrs}
        reducedMotion
      />
    </div>
  );
}

interface DocumentPreviewProps {
  projectId: string;
  itemName: string;
  /** The project's source language, for the content-model fallback preview's direction. */
  sourceLocale?: string;
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
  sourceLocale,
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

  const { getDisplayName } = useLocales();

  // The document's blocks, plus the surface's edited copies on top.
  const { data: documentBlocks } = useDocumentBlocks(projectId, itemName, targetLocale);
  const contentBlocks = useMemo(() => {
    const byId = new Map((documentBlocks ?? []).map((b) => [b.id, b]));
    for (const b of blocks) byId.set(b.id, b);
    return [...byId.values()];
  }, [documentBlocks, blocks]);

  // The block ↔ document id translation for the blocks now on screen. Rebuilt
  // with them, because a document that changed is a document whose markers did.
  const blockIds = useMemo(() => idTranslation(contentBlocks), [contentBlocks]);
  const blockIdsRef = useRef(blockIds);
  blockIdsRef.current = blockIds;

  // Inline mode: spacerHeight prop is provided
  const inlineMode = spacerHeight !== undefined;

  // Determine effective mode: controlled via prop, or internal toggle
  const isControlled = previewContentMode !== undefined;
  const showTarget = isControlled ? previewContentMode === "target" : internalMode === "target";
  const showPseudo = isControlled && previewContentMode === "pseudo";

  // How much of the document the chosen locale actually covers. Counted over
  // the translatable blocks only: a block with nothing to translate is not
  // outstanding work, and counting it would report a document that can never
  // reach the whole.
  const coverage = useMemo(() => {
    let total = 0;
    let translated = 0;
    for (const block of contentBlocks) {
      if (block.translatable === false) continue;
      total++;
      if (getTargetText(block, targetLocale).trim()) translated++;
    }
    return { total, translated };
  }, [contentBlocks, targetLocale]);
  const localeName = getDisplayName(targetLocale);

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
        // The document names the block it was clicked on; the surface knows it
        // by the store's id. Read through a ref rather than a dependency: this
        // listener is mounted once on purpose, and re-registering it on every
        // block change would drop clicks in the gap.
        const clicked = e.data.blockId as string;
        onBlockSelectRef.current(blockIdsRef.current.toBlock.get(clicked) ?? clicked);
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
  const readyTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const handleIframeLoad = useCallback(() => {
    clearTimeout(readyTimerRef.current);
    readyTimerRef.current = setTimeout(() => setIframeReady(true), 50);
  }, []);
  // Without this the timer outlives an unmount and sets state on a dead tree.
  useEffect(() => () => clearTimeout(readyTimerRef.current), []);

  // Send selection to iframe when selectedBlockId changes
  useEffect(() => {
    if (!iframeRef.current?.contentWindow || !selectedBlockId || !iframeReady) return;
    iframeRef.current.contentWindow.postMessage(
      {
        type: "kat-select-block",
        blockId: blockIds.toDocument.get(selectedBlockId) ?? selectedBlockId,
      },
      "*",
    );
  }, [selectedBlockId, iframeReady, blockIds]);

  // Tell the document who scrolls. Inline mode grows the frame to the content
  // and scrolls the surrounding page, so the document must not scroll too;
  // every other host gives the frame a fixed height, and there the document is
  // the only thing that can scroll.
  useEffect(() => {
    const cw = iframeRef.current?.contentWindow;
    if (!cw || !iframeReady) return;
    cw.postMessage({ type: "kat-fit-height", fit: inlineMode }, "*");
  }, [iframeReady, inlineMode]);

  // Send spacer insert/remove messages in inline mode
  useEffect(() => {
    const cw = iframeRef.current?.contentWindow;
    if (!cw || !iframeReady) return;

    if (inlineMode && selectedBlockId && spacerHeight > 0) {
      cw.postMessage(
        {
          type: "kat-insert-spacer",
          blockId: blockIds.toDocument.get(selectedBlockId) ?? selectedBlockId,
          height: spacerHeight,
        },
        "*",
      );
    } else {
      cw.postMessage({ type: "kat-remove-spacer" }, "*");
    }
  }, [selectedBlockId, spacerHeight, iframeReady, inlineMode, blockIds]);

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
      cw.postMessage({ type: "kat-update-block", blockId: documentIdOf(block), html }, "*");
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
      className="relative flex h-full w-full flex-col gap-2"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      {/* Reading a locale the document has not been translated into shows the
          source — a document is still a document with its translation
          outstanding. What it must not do is show the source *silently*, under
          the locale's own name: that reads as a translation that happens to
          match, or as a switch that did nothing. */}
      {showTarget && coverage.total > 0 && coverage.translated < coverage.total && (
        <div
          className="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground"
          data-testid="preview-coverage"
        >
          <Languages className="h-3.5 w-3.5 shrink-0" />
          {coverage.translated === 0
            ? `Nothing is translated into ${localeName} yet — reading the source.`
            : `${coverage.translated} of ${coverage.total} blocks are translated into ${localeName}; the rest read as the source.`}
        </div>
      )}

      {/* The split: a reader-generated preview is the document as its own format
          renders it, and only an iframe can show that faithfully. The server's
          fallback is a listing of the blocks — the content model already says
          everything that listing does, so the kit renders it instead, in the
          app's own type, structure-aware and with the block annotations marked
          where they occur. */}
      {previewHTML.includes(GENERIC_PREVIEW_MARKER) ? (
        <ContentPreview
          blocks={contentBlocks}
          itemName={itemName}
          sourceLocale={sourceLocale}
          targetLocale={targetLocale}
          side={showPseudo ? "pseudo" : showTarget ? "target" : "source"}
          selectedBlockId={selectedBlockId}
          onBlockSelect={onBlockSelect}
          spacerHeight={spacerHeight}
          onContentHeight={onContentHeight}
          onSpacerPosition={onSpacerPosition}
        />
      ) : (
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
      )}
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
