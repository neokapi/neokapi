import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type { ProjectInfo, BlockInfo, WordCountResult, SpanInfo } from "../types/api";
import { useEditorApi, useProviderConfigs } from "../hooks/useApi";
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
  const [showPreview, setShowPreview] = useState(false);

  const api = useEditorApi();
  const { configs: providerConfigs } = useProviderConfigs();
  const [selectedProvider, setSelectedProvider] = useState("");
  const editInputRef = useRef<HTMLTextAreaElement>(null);
  const blockListRef = useRef<HTMLDivElement>(null);

  // Load blocks
  const loadBlocks = useCallback(async () => {
    try {
      const b = await api.getFileBlocks(project.id, fileName);
      setBlocks(b || []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load blocks");
    }
  }, [api, project.id, fileName]);

  const loadWordCount = useCallback(async () => {
    try {
      const wc = await api.getWordCount(project.id, fileName);
      setWordCount(wc);
    } catch {
      // ignore word count errors
    }
  }, [api, project.id, fileName]);

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

  // Selected block ID for preview synchronization
  const selectedBlockId = filteredBlocks[selectedIndex]?.id;

  // Handle block selection from preview iframe — use ref to avoid re-renders
  const filteredBlocksRef = useRef(filteredBlocks);
  filteredBlocksRef.current = filteredBlocks;
  const handlePreviewBlockSelect = useCallback(
    (blockId: string) => {
      const index = filteredBlocksRef.current.findIndex((b) => b.id === blockId);
      if (index >= 0) {
        setSelectedIndex(index);
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

  const blockGrid = (
    <div ref={blockListRef} style={gridContainerStyle} data-testid="block-grid">
      {/* Header row */}
      <div style={gridHeaderStyle}>
        <span style={{ width: 50, textAlign: "center" }}>#</span>
        <span style={{ flex: 1 }}>Source</span>
        <span style={{ flex: 1 }}>Target ({targetLocale})</span>
      </div>

      {/* Block rows */}
      {filteredBlocks.map((block, index) => (
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
                : "3px solid transparent",
          }}
        >
          <span style={indexCellStyle}>{index + 1}</span>
          <div style={sourceCellStyle}>
            {block.has_spans && block.source_coded && block.source_spans ? (
              <SourceCellDisplay
                codedText={block.source_coded}
                spans={block.source_spans}
              />
            ) : (
              block.source
            )}
            {!block.translatable && (
              <span style={nonTransBadge}>non-translatable</span>
            )}
          </div>
          <div style={targetCellStyle}>
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
      ))}

      {filteredBlocks.length === 0 && (
        <div style={{ padding: 24, textAlign: "center", color: "var(--text-secondary)" }}>
          {searchQuery ? "No blocks match the search query" : "No blocks found"}
        </div>
      )}
    </div>
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 12 }}>
        <button onClick={onBack} style={backBtnStyle} data-testid="back-to-project">
          &#8592; {project.name}
        </button>
        <span style={{ fontSize: 16, fontWeight: 600, flex: 1 }}>{fileName}</span>
        <button
          onClick={() => setShowPreview(!showPreview)}
          style={{
            ...toolBtnStyle,
            backgroundColor: showPreview ? "var(--accent)" : "var(--bg-secondary)",
            color: showPreview ? "#fff" : "var(--text-primary)",
          }}
          data-testid="preview-toggle"
        >
          Preview
        </button>
        <select
          value={targetLocale}
          onChange={(e) => setTargetLocale(e.target.value)}
          style={selectStyle}
          data-testid="locale-selector"
        >
          {project.target_locales.map((l) => (
            <option key={l} value={l}>{l}</option>
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
        <div style={{ flex: 1 }} />
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
      <div style={progressContainerStyle}>
        <div style={{ ...progressBarStyle, width: `${progress}%` }} />
        <span style={progressTextStyle} data-testid="progress-text">
          {progress}% ({translatedCount}/{translatableBlocks.length} translated)
        </span>
      </div>

      {/* Messages */}
      {error && <div style={errorStyle}>{error}</div>}
      {message && <div style={messageStyle}>{message}</div>}

      {/* Main content: preview + grid or just grid */}
      {showPreview ? (
        <div style={splitContainerStyle} data-testid="split-layout">
          <div style={previewPaneStyle}>
            <DocumentPreview
              projectId={project.id}
              itemName={fileName}
              targetLocale={targetLocale}
              selectedBlockId={selectedBlockId}
              onBlockSelect={handlePreviewBlockSelect}
              blocks={blocks}
            />
          </div>
          <div style={gridPaneStyle}>
            {blockGrid}
          </div>
        </div>
      ) : (
        blockGrid
      )}

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

const progressBarStyle: React.CSSProperties = {
  height: "100%",
  backgroundColor: "var(--accent)",
  transition: "width 0.3s ease",
  borderRadius: 4,
  opacity: 0.3,
};

const progressTextStyle: React.CSSProperties = {
  position: "absolute",
  top: "50%",
  left: "50%",
  transform: "translate(-50%, -50%)",
  fontSize: 12,
  fontWeight: 600,
  color: "var(--text-primary)",
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
  width: 50,
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
