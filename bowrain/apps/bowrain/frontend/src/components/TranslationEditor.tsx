import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type { ProjectInfo, BlockInfo, WordCountResult, SpanInfo, TMMatchInfo, BlockTermMatch } from "../types/api";
import { useEditorApi, useProviderConfigs } from "../hooks/useApi";
import { useLocales } from "../hooks/useLocale";
import { DocumentPreview } from "./DocumentPreview";
import { SourceCellDisplay } from "./editor/SourceCellDisplay";
import { TargetCellEditor } from "./editor/TargetCellEditor";
import { parseCodedSegments } from "./editor/codedText";
import { TagChipComponent } from "./editor/TagChipComponent";
import { buildPairs, validateTags } from "./editor/tagSemantics";

interface TranslationEditorProps {
  project: ProjectInfo;
  fileName: string;
  onBack: () => void;
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

const statusColors: Record<BlockStatus, string> = {
  "not-started": "transparent",
  draft: "#f59e0b",
  translated: "#3b82f6",
  reviewed: "#22c55e",
};

export function TranslationEditor({ project, fileName, onBack }: TranslationEditorProps) {
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
      const path = await api.exportTranslatedFile(project.id, fileName, targetLocale);
      setMessage(`Exported to: ${path}`);
      // Open in OS
      await api.openFileInOS(path);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Export failed");
    } finally {
      setLoading(false);
    }
  };

  // --- New action handlers ---

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

  const blockGrid = (
    <div ref={blockListRef} style={gridContainerStyle} data-testid="block-grid">
      {/* Header row */}
      <div style={gridHeaderStyle}>
        <span style={{ width: 40, textAlign: "center" }}>#</span>
        <span style={{ width: 16 }} />
        <span style={{ flex: 1 }}>Source</span>
        <span style={{ flex: 1 }}>Target ({getDisplayName(targetLocale)})</span>
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
            style={{
              ...blockRowStyle,
              backgroundColor:
                selectedIndex === index ? "var(--bg-tertiary)" : "transparent",
              borderLeft:
                selectedIndex === index
                  ? "3px solid var(--accent)"
                  : status !== "not-started"
                    ? `3px solid ${statusColors[status]}`
                    : "3px solid transparent",
            }}
          >
            <span style={indexCellStyle}>{index + 1}</span>
            <span
              style={{
                width: 8,
                height: 8,
                borderRadius: "50%",
                backgroundColor: statusColors[status],
                flexShrink: 0,
                marginTop: 6,
                marginRight: 8,
              }}
              data-testid={`status-dot-${index}`}
              title={status}
            />
            <div style={sourceCellStyle}>
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
                <span style={nonTransBadge}>non-translatable</span>
              )}
            </div>
            <div
              style={targetCellStyle}
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
                    style={editTextareaStyle}
                    data-testid={`edit-target-${index}`}
                  />
                )
              ) : (
                <span
                  style={{
                    color: block.targets[targetLocale]
                      ? "var(--text-primary)"
                      : "var(--text-secondary)",
                    fontStyle: block.targets[targetLocale] ? "normal" : "italic",
                  }}
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
        <div style={{ padding: 24, textAlign: "center", color: "var(--text-secondary)" }}>
          {searchQuery ? "No blocks match the search query" : "No blocks found"}
        </div>
      )}
    </div>
  );

  const previewComponent = (
    <DocumentPreview
      projectId={project.id}
      itemName={fileName}
      targetLocale={targetLocale}
      selectedBlockId={selectedBlockId}
      onBlockSelect={handlePreviewBlockSelect}
      blocks={blocks}
    />
  );

  const focusView = currentBlock ? (
    <div style={{ flex: 1, display: "flex", flexDirection: "column", overflow: "auto", gap: 16, padding: 16 }} data-testid="focus-view">
      {/* Navigation header */}
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <button
          onClick={() => setSelectedIndex(Math.max(0, selectedIndex - 1))}
          style={toolBtnStyle}
          data-testid="focus-prev"
          disabled={selectedIndex <= 0}
        >
          &larr;
        </button>
        <span style={{ flex: 1, textAlign: "center", fontWeight: 600, fontSize: 14 }}>
          Block {selectedIndex + 1} of {filteredBlocks.length}
          <span
            style={{
              marginLeft: 8,
              padding: "2px 8px",
              borderRadius: 4,
              fontSize: 11,
              fontWeight: 600,
              color: "#fff",
              backgroundColor: statusColors[getBlockStatus(currentBlock, targetLocale)] === "transparent"
                ? "var(--text-secondary)"
                : statusColors[getBlockStatus(currentBlock, targetLocale)],
            }}
            data-testid="focus-status-badge"
          >
            {getBlockStatus(currentBlock, targetLocale)}
          </span>
        </span>
        <button
          onClick={() => setSelectedIndex(Math.min(filteredBlocks.length - 1, selectedIndex + 1))}
          style={toolBtnStyle}
          data-testid="focus-next"
          disabled={selectedIndex >= filteredBlocks.length - 1}
        >
          &rarr;
        </button>
      </div>

      {/* Context: previous block */}
      {selectedIndex > 0 && (
        <div style={contextBlockStyle} data-testid="focus-context-prev">
          <span style={{ fontSize: 11, color: "var(--text-secondary)", fontWeight: 600 }}>Previous</span>
          <div style={{ opacity: 0.5, fontSize: 13 }}>{filteredBlocks[selectedIndex - 1].source}</div>
        </div>
      )}

      {/* Current block - Source */}
      <div style={focusCardStyle}>
        <div style={{ marginBottom: 8, fontSize: 12, fontWeight: 600, color: "var(--text-secondary)", textTransform: "uppercase" as const }}>
          Source
        </div>
        <div style={{ fontSize: 16, lineHeight: 1.6 }} data-testid="focus-source">
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
      <div style={focusCardStyle}>
        <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 8 }}>
          <span style={{ fontSize: 12, fontWeight: 600, color: "var(--text-secondary)", textTransform: "uppercase" as const }}>
            Target ({getDisplayName(targetLocale)})
          </span>
          <div style={{ display: "flex", gap: 8 }}>
            <button onClick={handleCopySource} style={{ ...toolBtnStyle, fontSize: 11, padding: "2px 8px" }} data-testid="focus-copy-source">
              Copy Source
            </button>
            <button onClick={handleMarkReviewed} style={{ ...toolBtnStyle, fontSize: 11, padding: "2px 8px" }} data-testid="focus-mark-reviewed">
              Reviewed
            </button>
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
              style={{ ...editTextareaStyle, minHeight: 120 }}
              data-testid="focus-edit-target"
            />
          )}
        </div>
      </div>

      {/* Context: next block */}
      {selectedIndex < filteredBlocks.length - 1 && (
        <div style={contextBlockStyle} data-testid="focus-context-next">
          <span style={{ fontSize: 11, color: "var(--text-secondary)", fontWeight: 600 }}>Next</span>
          <div style={{ opacity: 0.5, fontSize: 13 }}>{filteredBlocks[selectedIndex + 1].source}</div>
        </div>
      )}
    </div>
  ) : null;

  // Build progress bar segments
  const progressSegments = (
    <div style={{ display: "flex", height: "100%", width: "100%", position: "absolute", top: 0, left: 0 }}>
      {statusCounts.reviewed > 0 && (
        <div
          data-testid="progress-reviewed"
          style={{ width: `${(statusCounts.reviewed / Math.max(translatableBlocks.length, 1)) * 100}%`, backgroundColor: "#22c55e", opacity: 0.4 }}
        />
      )}
      {statusCounts.translated > 0 && (
        <div
          data-testid="progress-translated"
          style={{ width: `${(statusCounts.translated / Math.max(translatableBlocks.length, 1)) * 100}%`, backgroundColor: "#3b82f6", opacity: 0.4 }}
        />
      )}
      {statusCounts.draft > 0 && (
        <div
          data-testid="progress-draft"
          style={{ width: `${(statusCounts.draft / Math.max(translatableBlocks.length, 1)) * 100}%`, backgroundColor: "#f59e0b", opacity: 0.4 }}
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
    <div style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 12 }}>
        <button onClick={onBack} style={backBtnStyle} data-testid="back-to-project">
          &#8592; {project.name}
        </button>
        <span style={{ fontSize: 16, fontWeight: 600, flex: 1 }}>{fileName}</span>
        {/* Layout mode switcher */}
        <div style={{ display: "flex", gap: 2, backgroundColor: "var(--bg-tertiary)", borderRadius: 6, padding: 2 }} data-testid="layout-switcher">
          {(["grid", "focus", "split-h", "split-v"] as const).map((mode) => (
            <button
              key={mode}
              onClick={() => setLayoutMode(mode)}
              data-testid={`layout-${mode}`}
              style={{
                padding: "4px 8px",
                backgroundColor: layoutMode === mode ? "var(--accent)" : "transparent",
                color: layoutMode === mode ? "#fff" : "var(--text-secondary)",
                border: "none",
                borderRadius: 4,
                fontSize: 11,
                cursor: "pointer",
                fontWeight: layoutMode === mode ? 600 : 400,
              }}
              title={mode === "grid" ? "Grid View" : mode === "focus" ? "Focus View" : mode === "split-h" ? "Horizontal Split" : "Vertical Split"}
            >
              {mode === "grid" ? "Grid" : mode === "focus" ? "Focus" : mode === "split-h" ? "H-Split" : "V-Split"}
            </button>
          ))}
        </div>
        <select
          value={targetLocale}
          onChange={(e) => setTargetLocale(e.target.value)}
          style={selectStyle}
          data-testid="locale-selector"
        >
          {project.target_locales.map((l) => (
            <option key={l} value={l}>{getDisplayName(l)} ({l})</option>
          ))}
        </select>
        <button onClick={handleExport} disabled={loading} style={exportBtnStyle} data-testid="export-btn">
          Export
        </button>
      </div>

      {/* Toolbar */}
      <div style={toolbarStyle}>
        <button onClick={handlePseudoTranslate} disabled={loading} style={toolBtnStyle} data-testid="pseudo-btn">
          Pseudo
        </button>
        <button onClick={handleAITranslate} disabled={loading} style={toolBtnStyle} data-testid="ai-translate-btn">
          AI Translate
        </button>
        {providerConfigs.length > 0 && (
          <select
            value={selectedProvider}
            onChange={(e) => setSelectedProvider(e.target.value)}
            style={selectStyle}
            data-testid="provider-select"
          >
            <option value="">Default provider</option>
            {providerConfigs.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        )}
        {providerConfigs.length === 0 && (
          <span style={{ fontSize: 11, color: "var(--text-secondary)" }} data-testid="provider-hint">
            Configure providers in Settings
          </span>
        )}
        <button onClick={handleTMTranslate} disabled={loading} style={toolBtnStyle} data-testid="tm-btn">
          TM Lookup
        </button>
        <div style={{ width: 1, height: 20, backgroundColor: "var(--border)" }} />
        <button onClick={handleCopySource} disabled={loading || selectedIndex < 0} style={toolBtnStyle} data-testid="copy-source-btn">
          Copy Source
        </button>
        <button onClick={handleMarkReviewed} disabled={loading || selectedIndex < 0} style={toolBtnStyle} data-testid="mark-reviewed-btn">
          Reviewed
        </button>
        <div style={{ width: 1, height: 20, backgroundColor: "var(--border)" }} />
        <button onClick={handlePrevUntranslated} style={toolBtnStyle} data-testid="prev-untranslated-btn">
          &larr; Untranslated
        </button>
        <button onClick={handleNextUntranslated} style={toolBtnStyle} data-testid="next-untranslated-btn">
          Untranslated &rarr;
        </button>
        <div style={{ flex: 1 }} />
        <button
          onClick={() => setShowContextPanel(!showContextPanel)}
          style={{
            ...toolBtnStyle,
            backgroundColor: showContextPanel ? "var(--accent)" : "var(--bg-secondary)",
            color: showContextPanel ? "#fff" : "var(--text-primary)",
          }}
          data-testid="context-panel-toggle"
          title="Toggle TM & Terminology panel"
        >
          Context
        </button>
        <input
          type="text"
          placeholder="Search blocks..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          style={searchStyle}
          data-testid="search-input"
        />
      </div>

      {/* Progress bar */}
      <div style={progressContainerStyle} data-testid="progress-bar">
        {progressSegments}
        <span style={progressTextStyle} data-testid="progress-text">
          {progress}% ({translatedCount}/{translatableBlocks.length} translated)
          {progressBreakdown.length > 0 && ` \u2014 ${progressBreakdown.join(", ")}`}
        </span>
      </div>

      {/* Messages */}
      {error && <div style={errorStyle}>{error}</div>}
      {message && <div style={messageStyle}>{message}</div>}

      {/* Main content area with optional context panel */}
      <div style={{ display: "flex", flex: 1, gap: 0, overflow: "hidden", minHeight: 0 }}>
        <div style={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden", minHeight: 0 }}>
          {layoutMode === "grid" && blockGrid}
          {layoutMode === "focus" && focusView}
          {layoutMode === "split-h" && (
            <div style={{ display: "flex", flexDirection: "column", flex: 1, gap: 12, overflow: "hidden" }}>
              <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
                {blockGrid}
              </div>
              <div style={{ height: "40%", minHeight: 200, overflow: "hidden" }} data-testid="split-h-preview">
                {previewComponent}
              </div>
            </div>
          )}
          {layoutMode === "split-v" && (
            <div style={splitContainerStyle} data-testid="split-layout">
              <div style={previewPaneStyle}>
                {previewComponent}
              </div>
              <div style={gridPaneStyle}>
                {blockGrid}
              </div>
            </div>
          )}
        </div>

        {/* Context Panel - TM & Terminology */}
        {showContextPanel && (
          <div style={contextPanelStyle} data-testid="context-panel">
            {contextLoading && (
              <div style={{ textAlign: "center", padding: "12px 0", fontSize: 12, color: "var(--text-secondary)" }}>
                Loading...
              </div>
            )}
            {/* TM Matches */}
            <div style={{ marginBottom: 16 }}>
              <div style={contextSectionHeader}>
                TM Matches
                {tmMatches.length > 0 && (
                  <span style={{ marginLeft: 6, fontWeight: 400, fontSize: 10 }}>({tmMatches.length})</span>
                )}
              </div>
              {!contextLoading && tmMatches.length === 0 ? (
                <div style={contextEmptyStyle}>No TM matches for this block</div>
              ) : (
                tmMatches.map((m, i) => (
                  <div key={i} style={{
                    ...tmMatchCardStyle,
                    ...(appliedTMIndex === i ? { borderColor: "#22c55e", backgroundColor: "rgba(34,197,94,0.05)" } : {}),
                  }} data-testid={`tm-match-${i}`}>
                    <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 4 }}>
                      <span style={tmScoreBadge(m.score)}>
                        {Math.round(m.score * 100)}%
                      </span>
                      <span style={{ fontSize: 10, color: "var(--text-secondary)" }}>
                        {m.match_type.replace(/-/g, " ")}
                      </span>
                    </div>
                    <div style={{ fontSize: 12, marginBottom: 4, color: "var(--text-secondary)" }}>
                      {m.source}
                    </div>
                    <div style={{ fontSize: 12, fontWeight: 500 }}>
                      {m.target}
                    </div>
                    <button
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
                      style={appliedTMIndex === i ? tmAppliedBtnStyle : tmApplyBtnStyle}
                      data-testid={`tm-apply-${i}`}
                      disabled={appliedTMIndex === i}
                    >
                      {appliedTMIndex === i ? "Applied" : "Apply"}
                    </button>
                  </div>
                ))
              )}
            </div>

            {/* Term Matches */}
            <div>
              <div style={contextSectionHeader}>
                Terminology
                {termMatches.length > 0 && (
                  <span style={{ marginLeft: 6, fontWeight: 400, fontSize: 10 }}>({termMatches.length})</span>
                )}
              </div>
              {!contextLoading && termMatches.length === 0 ? (
                <div style={contextEmptyStyle}>No terms found in this block</div>
              ) : (
                termMatches.map((m, i) => (
                  <div key={i} style={termMatchCardStyle} data-testid={`term-match-${i}`}>
                    <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 4 }}>
                      <span style={{ fontSize: 13, fontWeight: 600 }}>{m.source_term}</span>
                      <span style={termStatusBadge(m.status)}>{m.status}</span>
                    </div>
                    {m.target_terms && m.target_terms.length > 0 ? (
                      <div style={{ fontSize: 12 }}>
                        <span style={{ color: "var(--text-secondary)" }}>&#8594; </span>
                        <span style={{ fontWeight: 500 }}>{m.target_terms.join(", ")}</span>
                      </div>
                    ) : (
                      <div style={{ fontSize: 12, fontStyle: "italic", color: "var(--text-secondary)" }}>
                        No target term defined
                      </div>
                    )}
                    {m.domain && (
                      <span style={domainBadgeStyle}>{m.domain}</span>
                    )}
                  </div>
                ))
              )}
            </div>
          </div>
        )}
      </div>

      {/* Status bar */}
      <div style={statusBarStyle} data-testid="status-bar">
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
        <span style={{ color: "var(--text-secondary)" }}>
          Enter: edit | Esc: cancel | &#8593;&#8595;: navigate
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
    // Extract spans from coded text by counting markers and matching against source spans
    const spans: SpanInfo[] = [];
    for (const ch of targetCodedText) {
      const code = ch.charCodeAt(0);
      if (code >= 0xe001 && code <= 0xe003) {
        // Map marker to source span by index (same order as source)
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
      style={{
        marginLeft: 4,
        cursor: "help",
        fontSize: 14,
        color: validation.errors.length > 0 ? "rgb(220, 38, 38)" : "rgb(161, 98, 7)",
      }}
    >
      {"\u26A0"}
    </span>
  );
}

/** Highlights matched terminology in source text with underline styling. */
function HighlightedSource({ text, termMatches }: { text: string; termMatches: BlockTermMatch[] }) {
  if (termMatches.length === 0) return <>{text}</>;

  // Sort matches by start position, deduplicate overlapping
  const sorted = [...termMatches]
    .filter(m => m.start >= 0 && m.end > m.start && m.end <= text.length)
    .sort((a, b) => a.start - b.start);

  const parts: React.ReactNode[] = [];
  let lastEnd = 0;

  for (const m of sorted) {
    if (m.start < lastEnd) continue; // skip overlapping
    if (m.start > lastEnd) {
      parts.push(<span key={`t-${lastEnd}`}>{text.slice(lastEnd, m.start)}</span>);
    }
    parts.push(
      <span
        key={`h-${m.start}`}
        style={termHighlightStyle}
        title={`${m.source_term} → ${m.target_terms?.join(", ") || "?"} (${m.status})`}
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

const termHighlightStyle: React.CSSProperties = {
  textDecoration: "underline",
  textDecorationStyle: "dotted",
  textDecorationColor: "#8b5cf6",
  textUnderlineOffset: "2px",
  cursor: "help",
};

const backBtnStyle: React.CSSProperties = {
  padding: "6px 12px",
  backgroundColor: "var(--bg-tertiary)",
  color: "var(--text-primary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
};

const selectStyle: React.CSSProperties = {
  padding: "6px 12px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  color: "var(--text-primary)",
  fontSize: 13,
};

const exportBtnStyle: React.CSSProperties = {
  padding: "6px 16px",
  backgroundColor: "var(--accent)",
  color: "#fff",
  border: "none",
  borderRadius: 6,
  fontSize: 13,
  cursor: "pointer",
  fontWeight: 600,
};

const toolbarStyle: React.CSSProperties = {
  display: "flex",
  gap: 8,
  padding: "8px 0",
  alignItems: "center",
  flexWrap: "wrap",
};

const toolBtnStyle: React.CSSProperties = {
  padding: "6px 12px",
  backgroundColor: "var(--bg-secondary)",
  color: "var(--text-primary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  fontSize: 12,
  cursor: "pointer",
  fontWeight: 500,
};

const searchStyle: React.CSSProperties = {
  padding: "6px 12px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  color: "var(--text-primary)",
  fontSize: 13,
  outline: "none",
  width: 200,
};

const progressContainerStyle: React.CSSProperties = {
  position: "relative",
  height: 24,
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 4,
  overflow: "hidden",
  marginBottom: 8,
};

const progressTextStyle: React.CSSProperties = {
  position: "absolute",
  top: "50%",
  left: "50%",
  transform: "translate(-50%, -50%)",
  fontSize: 12,
  fontWeight: 600,
  color: "var(--text-primary)",
  whiteSpace: "nowrap",
};

const errorStyle: React.CSSProperties = {
  padding: "8px 12px",
  backgroundColor: "rgba(239,68,68,0.1)",
  border: "1px solid var(--error)",
  borderRadius: 6,
  color: "var(--error)",
  fontSize: 13,
  marginBottom: 8,
};

const messageStyle: React.CSSProperties = {
  padding: "8px 12px",
  backgroundColor: "rgba(34,197,94,0.1)",
  border: "1px solid var(--success)",
  borderRadius: 6,
  color: "var(--success)",
  fontSize: 13,
  marginBottom: 8,
};

const gridContainerStyle: React.CSSProperties = {
  flex: 1,
  overflow: "auto",
  border: "1px solid var(--border)",
  borderRadius: 8,
  backgroundColor: "var(--bg-secondary)",
};

const gridHeaderStyle: React.CSSProperties = {
  display: "flex",
  padding: "8px 12px",
  fontSize: 12,
  fontWeight: 600,
  color: "var(--text-secondary)",
  borderBottom: "1px solid var(--border)",
  textTransform: "uppercase",
  letterSpacing: 0.5,
  position: "sticky",
  top: 0,
  backgroundColor: "var(--bg-secondary)",
  zIndex: 1,
};

const blockRowStyle: React.CSSProperties = {
  display: "flex",
  padding: "8px 12px",
  borderBottom: "1px solid var(--border)",
  cursor: "pointer",
  transition: "background-color 0.1s ease",
  alignItems: "stretch",
  minHeight: 44,
};

const indexCellStyle: React.CSSProperties = {
  width: 40,
  textAlign: "center",
  fontSize: 12,
  color: "var(--text-secondary)",
  paddingTop: 2,
  flexShrink: 0,
};

const sourceCellStyle: React.CSSProperties = {
  flex: 1,
  fontSize: 14,
  lineHeight: 1.5,
  paddingRight: 16,
  wordBreak: "break-word",
};

const targetCellStyle: React.CSSProperties = {
  flex: 1,
  fontSize: 14,
  lineHeight: 1.5,
  wordBreak: "break-word",
  display: "flex",
  flexDirection: "column",
};

const editTextareaStyle: React.CSSProperties = {
  width: "100%",
  flex: 1,
  minHeight: 44,
  padding: "6px 8px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--accent)",
  borderRadius: 4,
  color: "var(--text-primary)",
  fontSize: 14,
  lineHeight: 1.5,
  resize: "vertical",
  outline: "none",
  fontFamily: "inherit",
  boxSizing: "border-box",
};

const nonTransBadge: React.CSSProperties = {
  marginLeft: 8,
  padding: "1px 6px",
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 3,
  fontSize: 10,
  color: "var(--text-secondary)",
  verticalAlign: "middle",
};

const statusBarStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  padding: "8px 0",
  fontSize: 12,
  color: "var(--text-secondary)",
};

const splitContainerStyle: React.CSSProperties = {
  display: "flex",
  flex: 1,
  gap: 12,
  overflow: "hidden",
};

const previewPaneStyle: React.CSSProperties = {
  flex: 1,
  minWidth: 0,
  overflow: "hidden",
};

const gridPaneStyle: React.CSSProperties = {
  flex: 1,
  minWidth: 0,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
};

const focusCardStyle: React.CSSProperties = {
  padding: 16,
  backgroundColor: "var(--bg-secondary)",
  border: "1px solid var(--border)",
  borderRadius: 8,
};

const contextBlockStyle: React.CSSProperties = {
  padding: 12,
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 6,
  opacity: 0.7,
};

const contextPanelStyle: React.CSSProperties = {
  width: 280,
  minWidth: 280,
  borderLeft: "1px solid var(--border)",
  backgroundColor: "var(--bg-secondary)",
  overflow: "auto",
  padding: 12,
  flexShrink: 0,
};

const contextSectionHeader: React.CSSProperties = {
  fontSize: 11,
  fontWeight: 700,
  color: "var(--text-secondary)",
  textTransform: "uppercase",
  letterSpacing: 0.5,
  marginBottom: 8,
  paddingBottom: 4,
  borderBottom: "1px solid var(--border)",
};

const contextEmptyStyle: React.CSSProperties = {
  fontSize: 12,
  color: "var(--text-secondary)",
  fontStyle: "italic",
  padding: "8px 0",
};

const tmMatchCardStyle: React.CSSProperties = {
  padding: 8,
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 6,
  marginBottom: 6,
  border: "1px solid var(--border)",
};

const tmApplyBtnStyle: React.CSSProperties = {
  marginTop: 6,
  padding: "2px 8px",
  fontSize: 11,
  backgroundColor: "var(--accent)",
  color: "#fff",
  border: "none",
  borderRadius: 4,
  cursor: "pointer",
  fontWeight: 500,
};

const tmAppliedBtnStyle: React.CSSProperties = {
  ...tmApplyBtnStyle,
  backgroundColor: "#22c55e",
  cursor: "default",
  opacity: 0.8,
};

const domainBadgeStyle: React.CSSProperties = {
  display: "inline-block",
  marginTop: 4,
  fontSize: 10,
  color: "var(--text-secondary)",
  padding: "1px 6px",
  borderRadius: 3,
  backgroundColor: "var(--bg-secondary)",
  border: "1px solid var(--border)",
};

function tmScoreBadge(score: number): React.CSSProperties {
  const color = score >= 1.0 ? "#22c55e" : score >= 0.9 ? "#3b82f6" : "#f59e0b";
  return {
    fontSize: 11,
    fontWeight: 700,
    color,
    padding: "1px 6px",
    borderRadius: 3,
    backgroundColor: `${color}20`,
  };
}

const termMatchCardStyle: React.CSSProperties = {
  padding: 8,
  backgroundColor: "var(--bg-tertiary)",
  borderRadius: 6,
  marginBottom: 6,
  border: "1px solid var(--border)",
};

function termStatusBadge(status: string): React.CSSProperties {
  const colors: Record<string, string> = {
    preferred: "#22c55e",
    approved: "#3b82f6",
    admitted: "#8b5cf6",
    deprecated: "#ef4444",
  };
  const color = colors[status] || "var(--text-secondary)";
  return {
    fontSize: 10,
    fontWeight: 600,
    color,
    padding: "1px 5px",
    borderRadius: 3,
    backgroundColor: `${color}15`,
  };
}
