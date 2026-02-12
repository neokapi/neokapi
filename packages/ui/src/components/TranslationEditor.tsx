import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type { ProjectInfo, BlockInfo, WordCountResult, SpanInfo, TMMatchInfo, BlockTermMatch } from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useProviderConfigs } from "../hooks/useProviderApi";
import { useLocales } from "../hooks/useLocales";
import { SourceCellDisplay } from "./editor/SourceCellDisplay";
import { TargetCellEditor } from "./editor/TargetCellEditor";
import { parseCodedSegments } from "./editor/codedText";
import { TagChipComponent } from "./editor/TagChipComponent";
import { buildPairs, validateTags } from "./editor/tagSemantics";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";
import { ArrowLeft, ArrowRight, ArrowUp, ArrowDown, AlertTriangle } from "./icons";

interface TranslationEditorProps {
  project: ProjectInfo;
  fileName: string;
  onBack: () => void;
  /** Optional export handler override. If not provided, browser file download is used. */
  onExport?: (blob: Blob, fileName: string) => void;
  /** Optional preview component for split layouts. */
  renderPreview?: (props: {
    projectId: string;
    itemName: string;
    targetLocale: string;
    selectedBlockId?: string;
    onBlockSelect: (blockId: string) => void;
    blocks: BlockInfo[];
  }) => React.ReactNode;
}

type LayoutMode = "grid" | "focus" | "split-h" | "split-v";
type BlockStatus = "not-started" | "draft" | "translated" | "reviewed";

function getBlockStatus(block: BlockInfo, locale: string): BlockStatus {
  if (block.properties["translation-status"] === "reviewed") return "reviewed";
  if (block.properties["translation-status"] === "draft") return "draft";
  if (!block.targets[locale]) return "not-started";
  if (
    block.properties["translation-origin"] === "machine" ||
    block.properties["translation-origin"] === "pseudo"
  ) {
    return "draft";
  }
  return "translated";
}

const statusDotClass: Record<BlockStatus, string> = {
  "not-started": "bg-transparent",
  draft: "bg-amber-500",
  translated: "bg-blue-500",
  reviewed: "bg-green-500",
};

const statusBorderClass: Record<BlockStatus, string> = {
  "not-started": "border-l-transparent",
  draft: "border-l-amber-500",
  translated: "border-l-blue-500",
  reviewed: "border-l-green-500",
};

const statusBadgeClass: Record<BlockStatus, string> = {
  "not-started": "bg-muted-foreground text-white",
  draft: "bg-amber-500 text-white",
  translated: "bg-blue-500 text-white",
  reviewed: "bg-green-500 text-white",
};

function tmScoreClass(score: number): string {
  if (score >= 1.0) return "text-green-500 bg-green-500/[0.12]";
  if (score >= 0.9) return "text-blue-500 bg-blue-500/[0.12]";
  return "text-amber-500 bg-amber-500/[0.12]";
}

function termStatusClass(status: string): string {
  const colors: Record<string, string> = {
    preferred: "text-green-500 bg-green-500/[0.08]",
    approved: "text-blue-500 bg-blue-500/[0.08]",
    admitted: "text-violet-500 bg-violet-500/[0.08]",
    deprecated: "text-red-500 bg-red-500/[0.08]",
  };
  return colors[status] || "text-muted-foreground bg-muted";
}

export function TranslationEditor({ project, fileName, onBack, onExport, renderPreview }: TranslationEditorProps) {
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editValue, setEditValue] = useState("");
  const [targetLocale, setTargetLocale] = useState(project.target_locales[0] || "");
  const [wordCount, setWordCount] = useState<WordCountResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [layoutMode, setLayoutMode] = useState<LayoutMode>("grid");
  const [focusEditValue, setFocusEditValue] = useState("");

  // Context panel state
  const [showContextPanel, setShowContextPanel] = useState(false);
  const [tmMatches, setTmMatches] = useState<TMMatchInfo[]>([]);
  const [termMatches, setTermMatches] = useState<BlockTermMatch[]>([]);
  const [contextLoading, setContextLoading] = useState(false);
  const [appliedTMIndex, setAppliedTMIndex] = useState<number | null>(null);

  const { getDisplayName } = useLocales();
  const api = useEditorApi();
  const { getFileBlocks, getWordCount: getWordCountApi } = api;
  const { configs: providerConfigs } = useProviderConfigs();
  const [selectedProvider, setSelectedProvider] = useState("");
  const editInputRef = useRef<HTMLTextAreaElement>(null);
  const blockListRef = useRef<HTMLDivElement>(null);

  // Load blocks
  const loadBlocks = useCallback(async () => {
    try {
      const b = await getFileBlocks(project.id, fileName);
      setBlocks(b || []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load blocks");
    }
  }, [getFileBlocks, project.id, fileName]);

  const loadWordCount = useCallback(async () => {
    try {
      const wc = await getWordCountApi(project.id, fileName);
      setWordCount(wc);
    } catch {
      // ignore word count errors
    }
  }, [getWordCountApi, project.id, fileName]);

  useEffect(() => {
    loadBlocks();
    loadWordCount();
  }, [loadBlocks, loadWordCount]);

  // Filter blocks by search
  const filteredBlocks = searchQuery
    ? blocks.filter(
        (b) =>
          b.source.toLowerCase().includes(searchQuery.toLowerCase()) ||
          (b.targets[targetLocale] || "").toLowerCase().includes(searchQuery.toLowerCase()),
      )
    : blocks;

  const translatableBlocks = filteredBlocks.filter((b) => b.translatable);
  const translatedCount = translatableBlocks.filter((b) => b.targets[targetLocale]).length;
  const progress = translatableBlocks.length > 0
    ? Math.round((translatedCount / translatableBlocks.length) * 100)
    : 0;

  // Status counts for progress bar
  const statusCounts = useMemo(() => {
    const counts = { "not-started": 0, draft: 0, translated: 0, reviewed: 0 };
    for (const b of translatableBlocks) {
      counts[getBlockStatus(b, targetLocale)]++;
    }
    return counts;
  }, [translatableBlocks, targetLocale]);

  // Selected block ID for preview synchronization
  const selectedBlockId = filteredBlocks[selectedIndex]?.id;

  // Handle block selection from preview iframe -- use ref to avoid re-renders
  const filteredBlocksRef = useRef(filteredBlocks);
  filteredBlocksRef.current = filteredBlocks;
  const startEditingRef = useRef<(index: number) => void>(() => {});
  const handlePreviewBlockSelect = useCallback(
    (blockId: string) => {
      const index = filteredBlocksRef.current.findIndex((b) => b.id === blockId);
      if (index >= 0) {
        setSelectedIndex(index);
        startEditingRef.current(index);
      }
    },
    [],
  );

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (editingIndex !== null) {
        if (e.key === "Escape") {
          setEditingIndex(null);
        } else if (e.key === "Enter" && !e.shiftKey) {
          e.preventDefault();
          handleSaveEdit();
        }
        return;
      }

      if (e.key === "ArrowDown" || e.key === "j") {
        e.preventDefault();
        setSelectedIndex((i) => Math.min(i + 1, filteredBlocks.length - 1));
      } else if (e.key === "ArrowUp" || e.key === "k") {
        e.preventDefault();
        setSelectedIndex((i) => Math.max(i - 1, 0));
      } else if (e.key === "Enter") {
        e.preventDefault();
        startEditing(selectedIndex);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editingIndex, selectedIndex, filteredBlocks.length]);

  // Scroll selected block into view
  useEffect(() => {
    const container = blockListRef.current;
    if (!container) return;
    const row = container.querySelector(`[data-row-index="${selectedIndex}"]`);
    if (row) {
      row.scrollIntoView({ block: "nearest", behavior: "smooth" });
    }
  }, [selectedIndex]);

  // Focus textarea on edit
  useEffect(() => {
    if (editingIndex !== null && editInputRef.current) {
      editInputRef.current.focus();
    }
  }, [editingIndex]);

  // Load TM and term matches when selected block changes (only if panel open)
  useEffect(() => {
    if (!showContextPanel) return;
    const block = filteredBlocks[selectedIndex];
    if (!block || !block.translatable) {
      setTmMatches([]);
      setTermMatches([]);
      return;
    }
    setContextLoading(true);
    setAppliedTMIndex(null);
    // TM lookup
    const tmPromise = api.lookupTMForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTmMatches(m || []))
      .catch(() => setTmMatches([]));
    // Term lookup
    const termPromise = api.lookupTermsForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTermMatches(m || []))
      .catch(() => setTermMatches([]));
    Promise.all([tmPromise, termPromise]).finally(() => setContextLoading(false));
  }, [showContextPanel, selectedIndex, filteredBlocks, targetLocale, project.id, fileName, api]);

  // Update focusEditValue when selectedIndex changes and we're in focus mode
  useEffect(() => {
    if (layoutMode === "focus") {
      const block = filteredBlocks[selectedIndex];
      if (block) {
        setFocusEditValue(block.targets[targetLocale] || "");
      }
    }
  }, [layoutMode, selectedIndex, filteredBlocks, targetLocale]);

  const startEditing = (index: number) => {
    const block = filteredBlocks[index];
    if (!block || !block.translatable) return;
    setEditingIndex(index);
    if (block.has_spans) {
      // For coded text editing, the TargetCellEditor handles its own state
      setEditValue("");
    } else {
      setEditValue(block.targets[targetLocale] || "");
    }
  };
  startEditingRef.current = startEditing;

  const handleSaveEdit = async () => {
    if (editingIndex === null) return;
    const block = filteredBlocks[editingIndex];
    if (!block) return;

    try {
      await api.updateBlockTarget({
        project_id: project.id,
        item_name: fileName,
        block_id: block.id,
        target_locale: targetLocale,
        text: editValue,
      });

      // Update local state
      setBlocks((prev) =>
        prev.map((b) =>
          b.id === block.id
            ? { ...b, targets: { ...b.targets, [targetLocale]: editValue } }
            : b,
        ),
      );

      const nextIndex = editingIndex + 1;
      setEditingIndex(null);
      if (nextIndex < filteredBlocks.length) {
        setSelectedIndex(nextIndex);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save");
    }
  };

  const handleSaveCodedEdit = async (codedText: string, spans: SpanInfo[]) => {
    if (editingIndex === null) return;
    const block = filteredBlocks[editingIndex];
    if (!block) return;

    try {
      await api.updateBlockTargetCoded({
        project_id: project.id,
        item_name: fileName,
        block_id: block.id,
        target_locale: targetLocale,
        coded_text: codedText,
        spans,
      });

      // Strip markers to get plain text for the targets display
      const plainText = codedText.replace(/[\uE001-\uE003]/g, "");

      // Update local state
      setBlocks((prev) =>
        prev.map((b) =>
          b.id === block.id
            ? {
                ...b,
                targets: { ...b.targets, [targetLocale]: plainText },
                targets_coded: { ...(b.targets_coded || {}), [targetLocale]: codedText },
              }
            : b,
        ),
      );

      const nextIndex = editingIndex + 1;
      setEditingIndex(null);
      if (nextIndex < filteredBlocks.length) {
        setSelectedIndex(nextIndex);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save");
    }
  };

  const handlePseudoTranslate = async () => {
    setLoading(true);
    setError(null);
    try {
      const stats = await api.pseudoTranslateFile(project.id, fileName, targetLocale);
      setMessage(`Pseudo-translated ${stats.translated_blocks} of ${stats.total_blocks} blocks`);
      await loadBlocks();
      await loadWordCount();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Pseudo-translate failed");
    } finally {
      setLoading(false);
    }
  };

  const handleAITranslate = async () => {
    setLoading(true);
    setError(null);
    try {
      const stats = await api.aiTranslateFile({
        project_id: project.id,
        item_name: fileName,
        target_locale: targetLocale,
        provider: "",
        api_key: "",
        model: "",
        provider_config_id: selectedProvider || undefined,
      });
      setMessage(`AI-translated ${stats.translated_blocks} of ${stats.total_blocks} blocks`);
      await loadBlocks();
      await loadWordCount();
    } catch (e) {
      setError(e instanceof Error ? e.message : "AI translate failed");
    } finally {
      setLoading(false);
    }
  };

  const handleTMTranslate = async () => {
    setLoading(true);
    setError(null);
    try {
      const stats = await api.tmTranslateFile(project.id, fileName, targetLocale);
      setMessage(`TM matched ${stats.translated_blocks} of ${stats.total_blocks} blocks`);
      await loadBlocks();
      await loadWordCount();
    } catch (e) {
      setError(e instanceof Error ? e.message : "TM lookup failed");
    } finally {
      setLoading(false);
    }
  };

  const handleExport = async () => {
    setLoading(true);
    setError(null);
    try {
      const blob = await api.exportTranslatedFile(project.id, fileName, targetLocale);
      if (onExport) {
        onExport(blob, fileName);
      } else {
        // Default: browser file download
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = fileName;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
      }
      setMessage(`Exported to ${fileName}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Export failed");
    } finally {
      setLoading(false);
    }
  };

  // --- Action handlers ---

  const handleCopySource = async () => {
    const block = filteredBlocks[selectedIndex];
    if (!block || !block.translatable) return;
    try {
      await api.updateBlockTarget({
        project_id: project.id,
        item_name: fileName,
        block_id: block.id,
        target_locale: targetLocale,
        text: block.source,
      });
      setBlocks((prev) =>
        prev.map((b) =>
          b.id === block.id
            ? { ...b, targets: { ...b.targets, [targetLocale]: block.source } }
            : b,
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to copy source");
    }
  };

  const handleMarkReviewed = () => {
    const block = filteredBlocks[selectedIndex];
    if (!block) return;
    setBlocks((prev) =>
      prev.map((b) =>
        b.id === block.id
          ? { ...b, properties: { ...b.properties, "translation-status": "reviewed" } }
          : b,
      ),
    );
  };

  const handleNextUntranslated = () => {
    for (let i = selectedIndex + 1; i < filteredBlocks.length; i++) {
      if (filteredBlocks[i].translatable && !filteredBlocks[i].targets[targetLocale]) {
        setSelectedIndex(i);
        return;
      }
    }
  };

  const handlePrevUntranslated = () => {
    for (let i = selectedIndex - 1; i >= 0; i--) {
      if (filteredBlocks[i].translatable && !filteredBlocks[i].targets[targetLocale]) {
        setSelectedIndex(i);
        return;
      }
    }
  };

  const handleFocusSave = async () => {
    const block = filteredBlocks[selectedIndex];
    if (!block || !block.translatable) return;
    if (focusEditValue === (block.targets[targetLocale] || "")) return;
    try {
      await api.updateBlockTarget({
        project_id: project.id,
        item_name: fileName,
        block_id: block.id,
        target_locale: targetLocale,
        text: focusEditValue,
      });
      setBlocks((prev) =>
        prev.map((b) =>
          b.id === block.id
            ? { ...b, targets: { ...b.targets, [targetLocale]: focusEditValue } }
            : b,
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save");
    }
  };

  // Current block for focus view
  const currentBlock = filteredBlocks[selectedIndex] || null;

  // Available layout modes depend on whether preview is available
  const availableLayouts: LayoutMode[] = renderPreview
    ? ["grid", "focus", "split-h", "split-v"]
    : ["grid", "focus"];

  const blockGrid = (
    <div ref={blockListRef} className="flex-1 overflow-auto border border-border rounded-lg bg-card" data-testid="block-grid">
      {/* Header row */}
      <div className="flex px-3 py-2 text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider sticky top-0 bg-card/80 backdrop-blur-sm z-[1]">
        <span className="w-10 text-center">#</span>
        <span className="w-4" />
        <span className="flex-1">Source</span>
        <span className="flex-1">Target ({getDisplayName(targetLocale)})</span>
      </div>

      {/* Block rows */}
      {filteredBlocks.map((block, index) => {
        const status = getBlockStatus(block, targetLocale);
        return (
          <div
            key={block.id}
            data-row-index={index}
            data-testid={`block-row-${index}`}
            onClick={() => {
              setSelectedIndex(index);
              if (editingIndex !== index) setEditingIndex(null);
            }}
            onDoubleClick={() => startEditing(index)}
            className={cn(
              "flex px-3 py-2 border-b border-border cursor-pointer items-stretch min-h-[44px] transition-colors border-l-[3px]",
              selectedIndex === index
                ? "bg-muted/50 border-l-primary"
                : statusBorderClass[status],
            )}
          >
            <span className="w-10 text-center text-xs text-muted-foreground pt-0.5 shrink-0">{index + 1}</span>
            <span
              className={cn("w-2 h-2 rounded-full shrink-0 mt-1.5 mr-2", statusDotClass[status])}
              data-testid={`status-dot-${index}`}
              title={status}
            />
            <div className="flex-1 text-sm leading-relaxed pr-4 break-words">
              {block.has_spans && block.source_coded && block.source_spans ? (
                <SourceCellDisplay
                  codedText={block.source_coded}
                  spans={block.source_spans}
                />
              ) : showContextPanel && selectedIndex === index && termMatches.length > 0 ? (
                <HighlightedSource text={block.source} termMatches={termMatches} />
              ) : (
                block.source
              )}
              {!block.translatable && (
                <span className="ml-2 px-1.5 py-px bg-muted rounded text-[10px] text-muted-foreground align-middle">non-translatable</span>
              )}
            </div>
            <div
              className="flex-1 text-sm leading-relaxed break-words flex flex-col"
              data-testid={`target-cell-${index}`}
              onClick={(e) => {
                if (editingIndex !== index) {
                  e.stopPropagation();
                  setSelectedIndex(index);
                  startEditing(index);
                }
              }}
            >
              {editingIndex === index ? (
                block.has_spans && block.source_spans ? (
                  <TargetCellEditor
                    initialCodedText={block.targets_coded?.[targetLocale] || ""}
                    initialSpans={block.source_spans}
                    sourceSpans={block.source_spans}
                    onSave={handleSaveCodedEdit}
                    onCancel={() => setEditingIndex(null)}
                  />
                ) : (
                  <textarea
                    ref={editInputRef}
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    onBlur={handleSaveEdit}
                    className="w-full flex-1 min-h-[44px] p-1.5 bg-muted border border-primary rounded text-foreground text-sm leading-relaxed resize-y outline-none font-[inherit]"
                    data-testid={`edit-target-${index}`}
                  />
                )
              ) : (
                <span
                  className={cn(
                    block.targets[targetLocale] ? "text-foreground" : "text-muted-foreground italic",
                  )}
                  data-testid={`target-text-${index}`}
                >
                  {block.has_spans && block.targets_coded?.[targetLocale] ? (
                    <>
                      <CodedTextDisplay
                        codedText={block.targets_coded[targetLocale]}
                        spans={block.source_spans || []}
                      />
                      {block.source_spans && (
                        <RowTagWarning
                          sourceSpans={block.source_spans}
                          targetCodedText={block.targets_coded[targetLocale]}
                        />
                      )}
                    </>
                  ) : (
                    block.targets[targetLocale] || (block.translatable ? "Click to translate..." : "")
                  )}
                </span>
              )}
            </div>
          </div>
        );
      })}

      {filteredBlocks.length === 0 && (
        <div className="p-6 text-center text-muted-foreground">
          {searchQuery ? "No blocks match the search query" : "No blocks found"}
        </div>
      )}
    </div>
  );

  const previewComponent = renderPreview ? renderPreview({
    projectId: project.id,
    itemName: fileName,
    targetLocale,
    selectedBlockId,
    onBlockSelect: handlePreviewBlockSelect,
    blocks,
  }) : null;

  const focusView = currentBlock ? (
    <div className="flex-1 flex flex-col overflow-auto gap-4 p-4" data-testid="focus-view">
      {/* Navigation header */}
      <div className="flex items-center gap-3">
        <Button
          variant="outline"
          size="sm"
          onClick={() => setSelectedIndex(Math.max(0, selectedIndex - 1))}
          data-testid="focus-prev"
          disabled={selectedIndex <= 0}
        >
          <ArrowLeft className="w-3.5 h-3.5" />
        </Button>
        <span className="flex-1 text-center font-semibold text-sm">
          Block {selectedIndex + 1} of {filteredBlocks.length}
          <span
            className={cn("ml-2 px-2 py-0.5 rounded text-[11px] font-semibold", statusBadgeClass[getBlockStatus(currentBlock, targetLocale)])}
            data-testid="focus-status-badge"
          >
            {getBlockStatus(currentBlock, targetLocale)}
          </span>
        </span>
        <Button
          variant="outline"
          size="sm"
          onClick={() => setSelectedIndex(Math.min(filteredBlocks.length - 1, selectedIndex + 1))}
          data-testid="focus-next"
          disabled={selectedIndex >= filteredBlocks.length - 1}
        >
          <ArrowRight className="w-3.5 h-3.5" />
        </Button>
      </div>

      {/* Context: previous block */}
      {selectedIndex > 0 && (
        <div className="p-3 bg-muted rounded-md opacity-70" data-testid="focus-context-prev">
          <span className="text-[11px] text-muted-foreground font-semibold">Previous</span>
          <div className="opacity-50 text-[13px]">{filteredBlocks[selectedIndex - 1].source}</div>
        </div>
      )}

      {/* Current block - Source */}
      <div className="p-4 bg-card border border-border rounded-lg">
        <div className="mb-2 text-xs font-semibold text-muted-foreground uppercase">
          Source
        </div>
        <div className="text-base leading-relaxed" data-testid="focus-source">
          {currentBlock.has_spans && currentBlock.source_coded && currentBlock.source_spans ? (
            <SourceCellDisplay codedText={currentBlock.source_coded} spans={currentBlock.source_spans} />
          ) : showContextPanel && termMatches.length > 0 ? (
            <HighlightedSource text={currentBlock.source} termMatches={termMatches} />
          ) : (
            currentBlock.source
          )}
        </div>
      </div>

      {/* Current block - Target */}
      <div className="p-4 bg-card border border-border rounded-lg">
        <div className="flex justify-between mb-2">
          <span className="text-xs font-semibold text-muted-foreground uppercase">
            Target ({getDisplayName(targetLocale)})
          </span>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" className="text-[11px] h-6 px-2" onClick={handleCopySource} data-testid="focus-copy-source">
              Copy Source
            </Button>
            <Button variant="outline" size="sm" className="text-[11px] h-6 px-2" onClick={handleMarkReviewed} data-testid="focus-mark-reviewed">
              Reviewed
            </Button>
          </div>
        </div>
        <div data-testid="focus-target">
          {currentBlock.has_spans && currentBlock.source_spans ? (
            <TargetCellEditor
              key={`focus-${currentBlock.id}-${targetLocale}`}
              initialCodedText={currentBlock.targets_coded?.[targetLocale] || ""}
              initialSpans={currentBlock.source_spans}
              sourceSpans={currentBlock.source_spans}
              onSave={handleSaveCodedEdit}
              onCancel={() => {}}
            />
          ) : (
            <textarea
              value={focusEditValue}
              onChange={(e) => setFocusEditValue(e.target.value)}
              onBlur={handleFocusSave}
              className="w-full flex-1 min-h-[120px] p-1.5 bg-muted border border-primary rounded text-foreground text-sm leading-relaxed resize-y outline-none font-[inherit]"
              data-testid="focus-edit-target"
            />
          )}
        </div>
      </div>

      {/* Context: next block */}
      {selectedIndex < filteredBlocks.length - 1 && (
        <div className="p-3 bg-muted rounded-md opacity-70" data-testid="focus-context-next">
          <span className="text-[11px] text-muted-foreground font-semibold">Next</span>
          <div className="opacity-50 text-[13px]">{filteredBlocks[selectedIndex + 1].source}</div>
        </div>
      )}
    </div>
  ) : null;

  // Build progress bar segments
  const progressSegments = (
    <div className="flex h-full w-full absolute top-0 left-0">
      {statusCounts.reviewed > 0 && (
        <div
          data-testid="progress-reviewed"
          className="bg-green-500 opacity-40"
          style={{ width: `${(statusCounts.reviewed / Math.max(translatableBlocks.length, 1)) * 100}%` }}
        />
      )}
      {statusCounts.translated > 0 && (
        <div
          data-testid="progress-translated"
          className="bg-blue-500 opacity-40"
          style={{ width: `${(statusCounts.translated / Math.max(translatableBlocks.length, 1)) * 100}%` }}
        />
      )}
      {statusCounts.draft > 0 && (
        <div
          data-testid="progress-draft"
          className="bg-amber-500 opacity-40"
          style={{ width: `${(statusCounts.draft / Math.max(translatableBlocks.length, 1)) * 100}%` }}
        />
      )}
    </div>
  );

  // Build progress text with status breakdown
  const progressBreakdown: string[] = [];
  if (statusCounts.reviewed > 0) progressBreakdown.push(`${statusCounts.reviewed} reviewed`);
  if (statusCounts.translated > 0) progressBreakdown.push(`${statusCounts.translated} translated`);
  if (statusCounts.draft > 0) progressBreakdown.push(`${statusCounts.draft} draft`);
  if (statusCounts["not-started"] > 0) progressBreakdown.push(`${statusCounts["not-started"]} pending`);

  return (
    <div className="flex flex-col flex-1 min-h-0">
      {/* Header */}
      <div className="flex items-center gap-3 mb-3">
        <Button variant="outline" size="sm" onClick={onBack} data-testid="back-to-project">
          <ArrowLeft className="w-3.5 h-3.5 mr-1" /> {project.name}
        </Button>
        <span className="text-base font-semibold flex-1">{fileName}</span>
        {/* Layout mode switcher */}
        <div className="flex gap-0.5 bg-muted rounded-md p-0.5" data-testid="layout-switcher">
          {availableLayouts.map((mode) => (
            <button
              key={mode}
              onClick={() => setLayoutMode(mode)}
              data-testid={`layout-${mode}`}
              className={cn(
                "px-2 py-1 border-none rounded text-[11px] cursor-pointer",
                layoutMode === mode
                  ? "bg-primary text-primary-foreground font-semibold"
                  : "bg-transparent text-muted-foreground font-normal",
              )}
              title={mode === "grid" ? "Grid View" : mode === "focus" ? "Focus View" : mode === "split-h" ? "Horizontal Split" : "Vertical Split"}
            >
              {mode === "grid" ? "Grid" : mode === "focus" ? "Focus" : mode === "split-h" ? "H-Split" : "V-Split"}
            </button>
          ))}
        </div>
        <select
          value={targetLocale}
          onChange={(e) => setTargetLocale(e.target.value)}
          className="px-3 py-1.5 bg-muted border border-border rounded-md text-foreground text-sm"
          data-testid="locale-selector"
        >
          {project.target_locales.map((l) => (
            <option key={l} value={l}>{getDisplayName(l)} ({l})</option>
          ))}
        </select>
        <Button size="sm" onClick={handleExport} disabled={loading} data-testid="export-btn">
          Export
        </Button>
      </div>

      {/* Toolbar */}
      <div className="flex gap-2 py-2 items-center flex-wrap backdrop-blur-sm">
        <Button variant="outline" size="sm" onClick={handlePseudoTranslate} disabled={loading} data-testid="pseudo-btn">
          Pseudo
        </Button>
        <Button variant="outline" size="sm" onClick={handleAITranslate} disabled={loading} data-testid="ai-translate-btn">
          AI Translate
        </Button>
        {providerConfigs.length > 0 && (
          <select
            value={selectedProvider}
            onChange={(e) => setSelectedProvider(e.target.value)}
            className="px-3 py-1.5 bg-muted border border-border rounded-md text-foreground text-sm"
            data-testid="provider-select"
          >
            <option value="">Default provider</option>
            {providerConfigs.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        )}
        {providerConfigs.length === 0 && (
          <span className="text-[11px] text-muted-foreground" data-testid="provider-hint">
            Configure providers in Settings
          </span>
        )}
        <Button variant="outline" size="sm" onClick={handleTMTranslate} disabled={loading} data-testid="tm-btn">
          TM Lookup
        </Button>
        <div className="w-px h-5 bg-border" />
        <Button variant="outline" size="sm" onClick={handleCopySource} disabled={loading || selectedIndex < 0} data-testid="copy-source-btn">
          Copy Source
        </Button>
        <Button variant="outline" size="sm" onClick={handleMarkReviewed} disabled={loading || selectedIndex < 0} data-testid="mark-reviewed-btn">
          Reviewed
        </Button>
        <div className="w-px h-5 bg-border" />
        <Button variant="outline" size="sm" onClick={handlePrevUntranslated} data-testid="prev-untranslated-btn">
          <ArrowLeft className="w-3 h-3 mr-1" /> Untranslated
        </Button>
        <Button variant="outline" size="sm" onClick={handleNextUntranslated} data-testid="next-untranslated-btn">
          Untranslated <ArrowRight className="w-3 h-3 ml-1" />
        </Button>
        <div className="flex-1" />
        <Button
          variant={showContextPanel ? "default" : "outline"}
          size="sm"
          onClick={() => setShowContextPanel(!showContextPanel)}
          data-testid="context-panel-toggle"
          title="Toggle TM & Terminology panel"
        >
          Context
        </Button>
        <input
          type="text"
          placeholder="Search blocks..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="px-3 py-1.5 bg-muted border border-border rounded-md text-foreground text-sm outline-none w-[200px]"
          data-testid="search-input"
        />
      </div>

      {/* Progress bar */}
      <div className="relative h-6 bg-muted rounded overflow-hidden mb-2" data-testid="progress-bar">
        {progressSegments}
        <span className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-xs font-semibold text-foreground whitespace-nowrap" data-testid="progress-text">
          {progress}% ({translatedCount}/{translatableBlocks.length} translated)
          {progressBreakdown.length > 0 && ` \u2014 ${progressBreakdown.join(", ")}`}
        </span>
      </div>

      {/* Messages */}
      {error && <div className="px-3 py-2 bg-destructive/10 border border-destructive rounded-md text-destructive text-sm mb-2">{error}</div>}
      {message && <div className="px-3 py-2 bg-green-500/10 border border-green-500 rounded-md text-green-600 dark:text-green-400 text-sm mb-2">{message}</div>}

      {/* Main content area with optional context panel */}
      <div className="flex flex-1 overflow-hidden min-h-0">
        <div className="flex-1 flex flex-col overflow-hidden min-h-0">
          {layoutMode === "grid" && blockGrid}
          {layoutMode === "focus" && focusView}
          {layoutMode === "split-h" && previewComponent && (
            <div className="flex flex-col flex-1 gap-3 overflow-hidden">
              <div className="flex-1 min-h-0 flex flex-col overflow-hidden">
                {blockGrid}
              </div>
              <div className="h-[40%] min-h-[200px] overflow-hidden" data-testid="split-h-preview">
                {previewComponent}
              </div>
            </div>
          )}
          {layoutMode === "split-h" && !previewComponent && blockGrid}
          {layoutMode === "split-v" && previewComponent && (
            <div className="flex flex-1 gap-3 overflow-hidden" data-testid="split-layout">
              <div className="flex-1 min-w-0 overflow-hidden">
                {previewComponent}
              </div>
              <div className="flex-1 min-w-0 flex flex-col overflow-hidden">
                {blockGrid}
              </div>
            </div>
          )}
          {layoutMode === "split-v" && !previewComponent && blockGrid}
        </div>

        {/* Context Panel - TM & Terminology */}
        {showContextPanel && (
          <div className="w-[280px] min-w-[280px] border-l border-border bg-card overflow-auto p-3 shrink-0" data-testid="context-panel">
            {contextLoading && (
              <div className="text-center py-3 text-xs text-muted-foreground">
                Loading...
              </div>
            )}
            {/* TM Matches */}
            <div className="mb-4">
              <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 pb-1 border-b border-border">
                TM Matches
                {tmMatches.length > 0 && (
                  <span className="ml-1.5 font-normal text-[10px]">({tmMatches.length})</span>
                )}
              </div>
              {!contextLoading && tmMatches.length === 0 ? (
                <div className="text-xs text-muted-foreground italic py-2">No TM matches for this block</div>
              ) : (
                tmMatches.map((m, i) => (
                  <div key={i} className={cn(
                    "p-2 bg-muted rounded-md mb-1.5 border border-border",
                    appliedTMIndex === i && "border-green-500 bg-green-500/5",
                  )} data-testid={`tm-match-${i}`}>
                    <div className="flex justify-between mb-1">
                      <span className={cn("text-[11px] font-bold px-1.5 py-px rounded", tmScoreClass(m.score))}>
                        {Math.round(m.score * 100)}%
                      </span>
                      <span className="text-[10px] text-muted-foreground">
                        {m.match_type.replace(/-/g, " ")}
                      </span>
                    </div>
                    <div className="text-xs mb-1 text-muted-foreground">
                      {m.source}
                    </div>
                    <div className="text-xs font-medium">
                      {m.target}
                    </div>
                    <Button
                      size="sm"
                      className={cn(
                        "mt-1.5 text-[11px] h-6 px-2",
                        appliedTMIndex === i && "bg-green-500 hover:bg-green-500 opacity-80 cursor-default",
                      )}
                      onClick={() => {
                        const block = filteredBlocks[selectedIndex];
                        if (!block || !block.translatable) return;
                        api.updateBlockTarget({
                          project_id: project.id,
                          item_name: fileName,
                          block_id: block.id,
                          target_locale: targetLocale,
                          text: m.target,
                        }).then(() => {
                          setBlocks((prev) =>
                            prev.map((b) =>
                              b.id === block.id
                                ? {
                                    ...b,
                                    targets: { ...b.targets, [targetLocale]: m.target },
                                    properties: { ...b.properties, "translation-origin": "tm" },
                                  }
                                : b,
                            ),
                          );
                          setAppliedTMIndex(i);
                        });
                      }}
                      data-testid={`tm-apply-${i}`}
                      disabled={appliedTMIndex === i}
                    >
                      {appliedTMIndex === i ? "Applied" : "Apply"}
                    </Button>
                  </div>
                ))
              )}
            </div>

            {/* Term Matches */}
            <div>
              <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 pb-1 border-b border-border">
                Terminology
                {termMatches.length > 0 && (
                  <span className="ml-1.5 font-normal text-[10px]">({termMatches.length})</span>
                )}
              </div>
              {!contextLoading && termMatches.length === 0 ? (
                <div className="text-xs text-muted-foreground italic py-2">No terms found in this block</div>
              ) : (
                termMatches.map((m, i) => (
                  <div key={i} className="p-2 bg-muted rounded-md mb-1.5 border border-border" data-testid={`term-match-${i}`}>
                    <div className="flex items-center gap-1.5 mb-1">
                      <span className="text-[13px] font-semibold">{m.source_term}</span>
                      <span className={cn("text-[10px] font-semibold px-1.5 py-px rounded", termStatusClass(m.status))}>{m.status}</span>
                    </div>
                    {m.target_terms && m.target_terms.length > 0 ? (
                      <div className="text-xs inline-flex items-center gap-1">
                        <ArrowRight className="w-3 h-3 text-muted-foreground shrink-0" />
                        <span className="font-medium">{m.target_terms.join(", ")}</span>
                      </div>
                    ) : (
                      <div className="text-xs italic text-muted-foreground">
                        No target term defined
                      </div>
                    )}
                    {m.domain && (
                      <span className="inline-block mt-1 text-[10px] text-muted-foreground px-1.5 py-px rounded bg-card border border-border">{m.domain}</span>
                    )}
                  </div>
                ))
              )}
            </div>
          </div>
        )}
      </div>

      {/* Status bar */}
      <div className="flex justify-between py-2 text-xs text-muted-foreground" data-testid="status-bar">
        <span>
          Block {selectedIndex + 1} of {filteredBlocks.length}
        </span>
        {wordCount && (
          <span>
            Source: {wordCount.source_words} words, {wordCount.source_chars} chars
            {wordCount.target_words[targetLocale] !== undefined && (
              <> | Target: {wordCount.target_words[targetLocale]} words</>
            )}
          </span>
        )}
        <span className="text-muted-foreground inline-flex items-center gap-0.5">
          Enter: edit | Esc: cancel | <ArrowUp className="w-3 h-3 inline-block" /><ArrowDown className="w-3 h-3 inline-block" />: navigate
          {editingIndex !== null && filteredBlocks[editingIndex]?.has_spans && (
            <> | Ctrl+1..9: insert tag</>
          )}
        </span>
      </div>
    </div>
  );
}

/** Read-only display of coded text with pair-aware tag chips (for target cell, not editing). */
function CodedTextDisplay({ codedText, spans }: { codedText: string; spans: SpanInfo[] }) {
  const segments = parseCodedSegments(codedText, spans);
  const pairs = useMemo(() => buildPairs(spans), [spans]);
  const [hoveredPairIndex, setHoveredPairIndex] = useState<number | null>(null);
  let tagIndex = 0;

  return (
    <span>
      {segments.map((seg, i) => {
        if (seg.type === "text") {
          return <span key={i}>{seg.value}</span>;
        }
        const currentTagIndex = tagIndex;
        tagIndex++;
        const pairInfo = pairs.get(currentTagIndex);
        const pairIdx = pairInfo?.pairIndex;

        return (
          <span
            key={i}
            onMouseEnter={() => pairIdx != null && setHoveredPairIndex(pairIdx)}
            onMouseLeave={() => setHoveredPairIndex(null)}
          >
            <TagChipComponent
              spanInfo={seg.spanInfo}
              index={currentTagIndex + 1}
              pairIndex={pairIdx}
              highlighted={hoveredPairIndex != null && pairIdx === hoveredPairIndex}
            />
          </span>
        );
      })}
    </span>
  );
}

/** Row-level validation indicator for tag mismatches. */
function RowTagWarning({ sourceSpans, targetCodedText }: { sourceSpans: SpanInfo[]; targetCodedText: string }) {
  const targetSpans = useMemo(() => {
    const spans: SpanInfo[] = [];
    for (const ch of targetCodedText) {
      const code = ch.charCodeAt(0);
      if (code >= 0xe001 && code <= 0xe003) {
        if (spans.length < sourceSpans.length) {
          spans.push(sourceSpans[spans.length]);
        }
      }
    }
    return spans;
  }, [targetCodedText, sourceSpans]);

  const validation = useMemo(
    () => validateTags(sourceSpans, targetSpans),
    [sourceSpans, targetSpans],
  );

  if (validation.valid && validation.warnings.length === 0) return null;

  const issues = [...validation.errors, ...validation.warnings];
  const tooltip = issues.map((i) => i.message).join("\n");

  return (
    <span
      title={tooltip}
      className={cn(
        "ml-1 cursor-help inline-flex",
        validation.errors.length > 0 ? "text-red-600" : "text-amber-700",
      )}
    >
      <AlertTriangle className="w-3.5 h-3.5" />
    </span>
  );
}

/** Highlights matched terminology in source text with underline styling. */
function HighlightedSource({ text, termMatches }: { text: string; termMatches: BlockTermMatch[] }) {
  if (termMatches.length === 0) return <>{text}</>;

  const sorted = [...termMatches]
    .filter(m => m.start >= 0 && m.end > m.start && m.end <= text.length)
    .sort((a, b) => a.start - b.start);

  const parts: React.ReactNode[] = [];
  let lastEnd = 0;

  for (const m of sorted) {
    if (m.start < lastEnd) continue;
    if (m.start > lastEnd) {
      parts.push(<span key={`t-${lastEnd}`}>{text.slice(lastEnd, m.start)}</span>);
    }
    parts.push(
      <span
        key={`h-${m.start}`}
        className="underline decoration-dotted decoration-violet-500 underline-offset-2 cursor-help"
        title={`${m.source_term} \u2192 ${m.target_terms?.join(", ") || "?"} (${m.status})`}
      >
        {text.slice(m.start, m.end)}
      </span>,
    );
    lastEnd = m.end;
  }
  if (lastEnd < text.length) {
    parts.push(<span key={`t-${lastEnd}`}>{text.slice(lastEnd)}</span>);
  }
  return <>{parts}</>;
}
