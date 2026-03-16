import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import type {
  ProjectInfo,
  BlockInfo,
  WordCountResult,
  SpanInfo,
  TMMatchInfo,
  BlockTermMatch,
  BlockNote,
  QAIssue,
  BlockHistoryEntry,
  FileQAResult,
  AddConceptRequest,
  EntityInfo,
} from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { useLocales } from "../hooks/useLocales";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";
import { FormattedSourceDisplay } from "./editor/FormattedSourceDisplay";
import { TargetCellEditor } from "./editor/TargetCellEditor";
import { validateTags } from "./editor/tagSemantics";
import { HighlightedSource, entityLabel } from "./editor/HighlightedSource";
import { EntityMarkPopover } from "./editor/EntityMarkPopover";
import { VisualEditorLayout } from "./editor/VisualEditorLayout";
import { DocumentPreview } from "./editor/DocumentPreview";
import type { VisualEditorMode, PreviewContentMode } from "./editor/visual-editor-types";
import { Button } from "./ui/button";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "./ui/select";
import { Alert, AlertDescription } from "./ui/alert";
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
  /** Optional presence slot rendered in the editor toolbar. */
  presenceSlot?: React.ReactNode;
}

type LayoutMode = "grid" | "focus" | "split-h" | "split-v" | "visual";
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
    admitted: "text-orange-600 bg-orange-600/[0.08]",
    deprecated: "text-red-500 bg-red-500/[0.08]",
  };
  return colors[status] || "text-muted-foreground bg-muted";
}

export function TranslationEditor({
  project,
  fileName,
  onBack,
  onExport,
  renderPreview,
  presenceSlot,
}: TranslationEditorProps) {
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editValue, setEditValue] = useState("");
  const [targetLocale, setTargetLocale] = useState(project.target_languages[0] || "");
  const [wordCount, setWordCount] = useState<WordCountResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [layoutMode, setLayoutMode] = useState<LayoutMode>("visual");
  const [editorMode, setEditorMode] = useState<VisualEditorMode>("translate");
  const [previewContentMode, setPreviewContentMode] = useState<PreviewContentMode>("source");
  const [focusEditValue, setFocusEditValue] = useState("");

  // Context panel state
  const [showContextPanel, setShowContextPanel] = useState(false);
  const [tmMatches, setTmMatches] = useState<TMMatchInfo[]>([]);
  const [termMatches, setTermMatches] = useState<BlockTermMatch[]>([]);
  const [contextLoading, setContextLoading] = useState(false);
  const [appliedTMIndex, setAppliedTMIndex] = useState<number | null>(null);

  // Visual editor extended state (QA, history, notes)
  const [blockQAIssues, setBlockQAIssues] = useState<QAIssue[]>([]);
  const [fileQAResults, setFileQAResults] = useState<FileQAResult[] | undefined>(undefined);
  const [qaLoading, setQaLoading] = useState(false);
  const [blockHistory, setBlockHistory] = useState<BlockHistoryEntry[]>([]);
  const [blockNotes, setBlockNotes] = useState<BlockNote[]>([]);

  // Entity marking state
  const [entityMarkState, setEntityMarkState] = useState<{
    text: string;
    start: number;
    end: number;
    position: { x: number; y: number };
  } | null>(null);

  const { getDisplayName } = useLocales();
  const fullApi = useApi();
  const { activeWorkspace } = useWorkspace();
  const wsSlug = activeWorkspace?.slug ?? "";

  // Register breadcrumb in the top bar area
  const breadcrumbNode = useMemo(
    () => (
      <button
        onClick={onBack}
        data-testid="back-to-project"
        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer bg-transparent border-none p-0"
      >
        <ArrowLeft className="w-3.5 h-3.5" /> {project.name}
      </button>
    ),
    [onBack, project.name],
  );
  useSetBreadcrumb(breadcrumbNode);

  const api = useEditorApi();
  const { getFileBlocks, getWordCount: getWordCountApi } = api;
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
    void loadBlocks();
    void loadWordCount();
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
  const progress =
    translatableBlocks.length > 0
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
  const handlePreviewBlockSelect = useCallback((blockId: string) => {
    const index = filteredBlocksRef.current.findIndex((b) => b.id === blockId);
    if (index >= 0) {
      setSelectedIndex(index);
      startEditingRef.current(index);
    }
  }, []);

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (layoutMode === "visual") return;
      if (editingIndex !== null) {
        if (e.key === "Escape") {
          setEditingIndex(null);
        } else if (e.key === "Enter" && !e.shiftKey) {
          e.preventDefault();
          void handleSaveEdit();
        }
        return;
      }

      // Cmd+E: mark selected text as entity
      if (e.key === "e" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        const sel = window.getSelection();
        if (sel && sel.toString().trim().length > 0 && selectedIndex >= 0) {
          const block = filteredBlocks[selectedIndex];
          const selectedText = sel.toString().trim();
          const sourceText = block?.source ?? "";
          const startIdx = sourceText.indexOf(selectedText);
          if (startIdx >= 0) {
            const range = sel.getRangeAt(0);
            const rect = range.getBoundingClientRect();
            setEntityMarkState({
              text: selectedText,
              start: startIdx,
              end: startIdx + selectedText.length,
              position: { x: rect.left, y: rect.bottom },
            });
          }
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
  }, [editingIndex, selectedIndex, filteredBlocks.length, layoutMode]);

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

  // Load TM and term matches when selected block changes (if panel open or visual mode)
  useEffect(() => {
    if (!showContextPanel && layoutMode !== "visual") return;
    const block = filteredBlocks[selectedIndex];
    if (!block || !block.translatable) {
      setTmMatches([]);
      setTermMatches([]);
      return;
    }
    setContextLoading(true);
    setAppliedTMIndex(null);
    // TM lookup
    const tmPromise = api
      .lookupTMForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTmMatches(m || []))
      .catch(() => setTmMatches([]));
    // Term lookup
    const termPromise = api
      .lookupTermsForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTermMatches(m || []))
      .catch(() => setTermMatches([]));
    void Promise.all([tmPromise, termPromise]).finally(() => setContextLoading(false));
  }, [
    showContextPanel,
    layoutMode,
    selectedIndex,
    filteredBlocks,
    targetLocale,
    project.id,
    fileName,
    api,
  ]);

  // Load QA issues, history, and notes for current block in visual mode
  useEffect(() => {
    if (layoutMode !== "visual") return;
    const block = filteredBlocks[selectedIndex];
    if (!block) return;
    // QA: run per-block check
    api
      .runQACheck(project.id, block.id, targetLocale)
      .then((issues) => setBlockQAIssues(issues || []))
      .catch(() => setBlockQAIssues([]));
    // History
    api
      .getBlockHistory(project.id, block.id, targetLocale, 20)
      .then((h) => setBlockHistory(h || []))
      .catch(() => setBlockHistory([]));
    // Notes (enrich mode loads these)
    api
      .listBlockNotes(project.id, block.id)
      .then((n) => setBlockNotes(n || []))
      .catch(() => setBlockNotes([]));
  }, [layoutMode, selectedIndex, filteredBlocks, targetLocale, project.id, api]);

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

  const handleCreateEntity = useCallback(
    async (type: string, dnt: boolean) => {
      if (!entityMarkState || selectedIndex < 0) return;
      const block = filteredBlocks[selectedIndex];
      if (!block) return;
      try {
        const created = await fullApi.createEntity(wsSlug, project.id, fileName, block.id, {
          text: entityMarkState.text,
          type,
          start: entityMarkState.start,
          end: entityMarkState.end,
          dnt,
          source: "manual",
        });
        // Update block entities in local state.
        setBlocks((prev) =>
          prev.map((b) =>
            b.id === block.id ? { ...b, entities: [...(b.entities ?? []), created] } : b,
          ),
        );
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to create entity");
      }
      setEntityMarkState(null);
    },
    [entityMarkState, selectedIndex, filteredBlocks, fullApi, wsSlug, project.id, fileName],
  );

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
          b.id === block.id ? { ...b, targets: { ...b.targets, [targetLocale]: editValue } } : b,
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
                targets_coded: { ...b.targets_coded, [targetLocale]: codedText },
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
          b.id === block.id ? { ...b, targets: { ...b.targets, [targetLocale]: block.source } } : b,
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

  // Visual mode handlers
  const handleVisualSave = useCallback(
    async (codedText: string, spans: SpanInfo[]) => {
      if (editingIndex === null) return;
      const block = filteredBlocks[editingIndex];
      if (!block) return;

      if (block.has_spans) {
        await handleSaveCodedEdit(codedText, spans);
      } else {
        // For non-coded blocks, codedText is plain text
        const plainText = codedText.replace(/[\uE001-\uE003]/g, "");
        try {
          await api.updateBlockTarget({
            project_id: project.id,
            item_name: fileName,
            block_id: block.id,
            target_locale: targetLocale,
            text: plainText,
          });
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === block.id
                ? { ...b, targets: { ...b.targets, [targetLocale]: plainText } }
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
      }
    },
    [editingIndex, filteredBlocks, handleSaveCodedEdit, api, project.id, fileName, targetLocale],
  );

  const handleVisualApprove = useCallback(() => {
    handleMarkReviewed();
    // Advance to next block
    if (selectedIndex < filteredBlocks.length - 1) {
      setSelectedIndex(selectedIndex + 1);
    }
  }, [selectedIndex, filteredBlocks.length]);

  const handleVisualReject = useCallback(() => {
    const block = filteredBlocks[selectedIndex];
    if (!block) return;
    setBlocks((prev) =>
      prev.map((b) =>
        b.id === block.id
          ? { ...b, properties: { ...b.properties, "translation-status": "draft" } }
          : b,
      ),
    );
  }, [selectedIndex, filteredBlocks]);

  const handleVisualApplyTM = useCallback(
    (index: number) => {
      const match = tmMatches[index];
      const block = filteredBlocks[selectedIndex];
      if (!match || !block || !block.translatable) return;
      void api
        .updateBlockTarget({
          project_id: project.id,
          item_name: fileName,
          block_id: block.id,
          target_locale: targetLocale,
          text: match.target,
        })
        .then(() => {
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === block.id
                ? {
                    ...b,
                    targets: { ...b.targets, [targetLocale]: match.target },
                    properties: { ...b.properties, "translation-origin": "tm" },
                  }
                : b,
            ),
          );
          setAppliedTMIndex(index);
        });
    },
    [tmMatches, filteredBlocks, selectedIndex, api, project.id, fileName, targetLocale],
  );

  const handleVisualInsertTerm = useCallback((_text: string) => {
    // Term insertion is handled by the VisualEditorCard's TargetCellEditor
  }, []);

  const handleRunFileQA = useCallback(() => {
    setQaLoading(true);
    api
      .runFileQACheck(project.id, fileName, targetLocale)
      .then((results) => setFileQAResults(results || []))
      .catch(() => setFileQAResults([]))
      .finally(() => setQaLoading(false));
  }, [api, project.id, fileName, targetLocale]);

  const handleRevertHistory = useCallback(
    (entry: BlockHistoryEntry) => {
      const block = filteredBlocks[selectedIndex];
      if (!block) return;
      api
        .updateBlockTarget({
          project_id: project.id,
          item_name: fileName,
          block_id: block.id,
          target_locale: targetLocale,
          text: entry.text,
        })
        .then(() => {
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === block.id
                ? { ...b, targets: { ...b.targets, [targetLocale]: entry.text } }
                : b,
            ),
          );
        })
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to revert"));
    },
    [filteredBlocks, selectedIndex, api, project.id, fileName, targetLocale],
  );

  const handleAddNote = useCallback(
    (text: string) => {
      const block = filteredBlocks[selectedIndex];
      if (!block) return;
      api
        .addBlockNote(project.id, block.id, text)
        .then((note) => setBlockNotes((prev) => [...prev, note]))
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to add note"));
    },
    [filteredBlocks, selectedIndex, api, project.id],
  );

  const handleDeleteNote = useCallback(
    (noteId: string) => {
      api
        .deleteBlockNote(project.id, noteId)
        .then(() => setBlockNotes((prev) => prev.filter((n) => n.id !== noteId)))
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to delete note"));
    },
    [api, project.id],
  );

  const handleTermCreate = useCallback(
    async (req: AddConceptRequest) => {
      try {
        await fullApi.addConcept(wsSlug, req);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to create term");
      }
    },
    [fullApi, wsSlug],
  );

  const handleVisualNavigate = useCallback((index: number) => {
    setSelectedIndex(index);
    setEditingIndex(null);
  }, []);

  // Current block for focus view
  const currentBlock = filteredBlocks[selectedIndex] || null;

  // All layout modes are always available — DocumentPreview is used as fallback
  // when renderPreview is not provided
  const availableLayouts: LayoutMode[] = ["grid", "focus", "split-h", "split-v", "visual"];

  const blockGrid = (
    <div
      ref={blockListRef}
      className="flex-1 overflow-auto border border-border rounded-lg bg-card"
      data-testid="block-grid"
    >
      {/* Header row */}
      <div className="flex px-3 py-2 text-xs font-semibold text-muted-foreground border-b border-border uppercase tracking-wider sticky top-0 bg-card backdrop-blur-sm z-[1]">
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
              selectedIndex === index ? "bg-muted/50 border-l-primary" : statusBorderClass[status],
            )}
          >
            <span className="w-10 text-center text-xs text-muted-foreground pt-0.5 shrink-0">
              {index + 1}
            </span>
            <span
              className={cn("w-2 h-2 rounded-full shrink-0 mt-1.5 mr-2", statusDotClass[status])}
              data-testid={`status-dot-${index}`}
              title={status}
            />
            <div className="flex-1 text-sm leading-relaxed pr-4 break-words">
              {block.has_spans && block.source_coded && block.source_spans ? (
                <FormattedSourceDisplay codedText={block.source_coded} spans={block.source_spans} />
              ) : showContextPanel &&
                selectedIndex === index &&
                (termMatches.length > 0 || (block.entities && block.entities.length > 0)) ? (
                <HighlightedSource
                  text={block.source}
                  termMatches={termMatches}
                  entities={block.entities}
                />
              ) : (
                block.source
              )}
              {!block.translatable && (
                <span className="ml-2 px-1.5 py-px bg-muted rounded text-[10px] text-muted-foreground align-middle">
                  non-translatable
                </span>
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
                    block.targets[targetLocale]
                      ? "text-foreground"
                      : "text-muted-foreground italic",
                  )}
                  data-testid={`target-text-${index}`}
                >
                  {block.has_spans && block.targets_coded?.[targetLocale] ? (
                    <>
                      <FormattedSourceDisplay
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
                    block.targets[targetLocale] ||
                    (block.translatable ? "Click to translate..." : "")
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

  const previewComponent = renderPreview
    ? renderPreview({
        projectId: project.id,
        itemName: fileName,
        targetLocale,
        selectedBlockId,
        onBlockSelect: handlePreviewBlockSelect,
        blocks,
      })
    : null;

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
            className={cn(
              "ml-2 px-2 py-0.5 rounded text-[11px] font-semibold",
              statusBadgeClass[getBlockStatus(currentBlock, targetLocale)],
            )}
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
        <div className="mb-2 text-xs font-semibold text-muted-foreground uppercase">Source</div>
        <div className="text-base leading-relaxed" data-testid="focus-source">
          {currentBlock.has_spans && currentBlock.source_coded && currentBlock.source_spans ? (
            <FormattedSourceDisplay
              codedText={currentBlock.source_coded}
              spans={currentBlock.source_spans}
            />
          ) : showContextPanel &&
            (termMatches.length > 0 ||
              (currentBlock.entities && currentBlock.entities.length > 0)) ? (
            <HighlightedSource
              text={currentBlock.source}
              termMatches={termMatches}
              entities={currentBlock.entities}
            />
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
            <Button
              variant="outline"
              size="sm"
              className="text-[11px] h-6 px-2"
              onClick={handleCopySource}
              data-testid="focus-copy-source"
            >
              Copy Source
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-[11px] h-6 px-2"
              onClick={handleMarkReviewed}
              data-testid="focus-mark-reviewed"
            >
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
          style={{
            width: `${(statusCounts.reviewed / Math.max(translatableBlocks.length, 1)) * 100}%`,
          }}
        />
      )}
      {statusCounts.translated > 0 && (
        <div
          data-testid="progress-translated"
          className="bg-blue-500 opacity-40"
          style={{
            width: `${(statusCounts.translated / Math.max(translatableBlocks.length, 1)) * 100}%`,
          }}
        />
      )}
      {statusCounts.draft > 0 && (
        <div
          data-testid="progress-draft"
          className="bg-amber-500 opacity-40"
          style={{
            width: `${(statusCounts.draft / Math.max(translatableBlocks.length, 1)) * 100}%`,
          }}
        />
      )}
    </div>
  );

  // Build progress text with status breakdown
  const progressBreakdown: string[] = [];
  if (statusCounts.reviewed > 0) progressBreakdown.push(`${statusCounts.reviewed} reviewed`);
  if (statusCounts.translated > 0) progressBreakdown.push(`${statusCounts.translated} translated`);
  if (statusCounts.draft > 0) progressBreakdown.push(`${statusCounts.draft} draft`);
  if (statusCounts["not-started"] > 0)
    progressBreakdown.push(`${statusCounts["not-started"]} pending`);

  // Split mode fallback preview: use DocumentPreview when renderPreview is not provided
  const splitPreview = previewComponent ?? (
    <DocumentPreview
      projectId={project.id}
      itemName={fileName}
      targetLocale={targetLocale}
      selectedBlockId={selectedBlockId}
      onBlockSelect={handlePreviewBlockSelect}
      blocks={blocks}
    />
  );

  // In visual mode, render the full-screen visual editor layout
  if (layoutMode === "visual") {
    return (
      <div className="flex flex-col flex-1 min-h-0">
        {/* Header */}
        <div className="flex items-center gap-3 mb-3">
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
                title={
                  mode === "grid"
                    ? "Grid View"
                    : mode === "focus"
                      ? "Focus View"
                      : mode === "split-h"
                        ? "Horizontal Split"
                        : mode === "split-v"
                          ? "Vertical Split"
                          : "Visual Mode"
                }
              >
                {mode === "grid"
                  ? "Grid"
                  : mode === "focus"
                    ? "Focus"
                    : mode === "split-h"
                      ? "H-Split"
                      : mode === "split-v"
                        ? "V-Split"
                        : "Visual"}
              </button>
            ))}
          </div>
          <Select value={targetLocale} onValueChange={setTargetLocale}>
            <SelectTrigger className="w-[180px]" data-testid="locale-selector">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {project.target_languages.map((l) => (
                <SelectItem key={l} value={l}>
                  {getDisplayName(l)} ({l})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Messages */}
        {error && (
          <Alert variant="destructive" className="mb-2">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {/* Visual Editor Layout */}
        <div className="flex-1 min-h-0 relative">
          <VisualEditorLayout
            project={project}
            fileName={fileName}
            blocks={filteredBlocks}
            selectedIndex={selectedIndex}
            editingIndex={editingIndex}
            targetLocale={targetLocale}
            editorMode={editorMode}
            onEditorModeChange={setEditorMode}
            previewContentMode={previewContentMode}
            onPreviewContentModeChange={setPreviewContentMode}
            onNavigate={handleVisualNavigate}
            onStartEditing={() => startEditing(selectedIndex)}
            onSave={handleVisualSave}
            onCancelEditing={() => setEditingIndex(null)}
            onApprove={handleVisualApprove}
            onReject={handleVisualReject}
            tmMatches={tmMatches}
            termMatches={termMatches}
            onApplyTM={handleVisualApplyTM}
            onInsertTerm={handleVisualInsertTerm}
            presenceSlot={presenceSlot}
            qaIssues={blockQAIssues}
            fileQAResults={fileQAResults}
            qaLoading={qaLoading}
            onRunFileQA={handleRunFileQA}
            history={blockHistory}
            onRevertHistory={handleRevertHistory}
            notes={blockNotes}
            onAddNote={handleAddNote}
            onDeleteNote={handleDeleteNote}
            onTermCreate={handleTermCreate}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col flex-1 min-h-0">
      {/* Header */}
      <div className="flex items-center gap-3 mb-3">
        <span className="text-base font-semibold flex-1">{fileName}</span>
        {presenceSlot}
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
              title={
                mode === "grid"
                  ? "Grid View"
                  : mode === "focus"
                    ? "Focus View"
                    : mode === "split-h"
                      ? "Horizontal Split"
                      : mode === "split-v"
                        ? "Vertical Split"
                        : "Visual Mode"
              }
            >
              {mode === "grid"
                ? "Grid"
                : mode === "focus"
                  ? "Focus"
                  : mode === "split-h"
                    ? "H-Split"
                    : mode === "split-v"
                      ? "V-Split"
                      : "Visual"}
            </button>
          ))}
        </div>
        <Select value={targetLocale} onValueChange={setTargetLocale}>
          <SelectTrigger className="w-[180px]" data-testid="locale-selector">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {project.target_languages.map((l) => (
              <SelectItem key={l} value={l}>
                {getDisplayName(l)} ({l})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button size="sm" onClick={handleExport} disabled={loading} data-testid="export-btn">
          Export
        </Button>
      </div>

      {/* Toolbar */}
      <div className="flex gap-2 py-2 items-center flex-wrap backdrop-blur-sm">
        <Button
          variant="outline"
          size="sm"
          onClick={handleTMTranslate}
          disabled={loading}
          data-testid="tm-btn"
        >
          TM Lookup
        </Button>
        <div className="w-px h-5 bg-border" />
        <Button
          variant="outline"
          size="sm"
          onClick={handleCopySource}
          disabled={loading || selectedIndex < 0}
          data-testid="copy-source-btn"
        >
          Copy Source
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={handleMarkReviewed}
          disabled={loading || selectedIndex < 0}
          data-testid="mark-reviewed-btn"
        >
          Reviewed
        </Button>
        <div className="w-px h-5 bg-border" />
        <Button
          variant="outline"
          size="sm"
          onClick={handlePrevUntranslated}
          data-testid="prev-untranslated-btn"
        >
          <ArrowLeft className="w-3 h-3 mr-1" /> Untranslated
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={handleNextUntranslated}
          data-testid="next-untranslated-btn"
        >
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
      <div
        className="relative h-6 bg-muted rounded overflow-hidden mb-2"
        data-testid="progress-bar"
      >
        {progressSegments}
        <span
          className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-xs font-semibold text-foreground whitespace-nowrap"
          data-testid="progress-text"
        >
          {progress}% ({translatedCount}/{translatableBlocks.length} translated)
          {progressBreakdown.length > 0 && ` \u2014 ${progressBreakdown.join(", ")}`}
        </span>
      </div>

      {/* Messages */}
      {error && (
        <Alert variant="destructive" className="mb-2">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {message && (
        <Alert className="mb-2 border-green-200 text-green-800 dark:border-green-800 dark:text-green-400">
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}

      {/* Main content area with optional context panel */}
      <div className="flex flex-1 overflow-hidden min-h-0">
        <div className="flex-1 flex flex-col overflow-hidden min-h-0">
          {layoutMode === "grid" && blockGrid}
          {layoutMode === "focus" && focusView}
          {layoutMode === "split-h" && (
            <div className="flex flex-col flex-1 gap-3 overflow-hidden">
              <div className="flex-1 min-h-0 flex flex-col overflow-hidden">{blockGrid}</div>
              <div className="h-[40%] min-h-[200px] overflow-hidden" data-testid="split-h-preview">
                {splitPreview}
              </div>
            </div>
          )}
          {layoutMode === "split-v" && (
            <div className="flex flex-1 gap-3 overflow-hidden" data-testid="split-layout">
              <div className="flex-1 min-w-0 overflow-hidden">{splitPreview}</div>
              <div className="flex-1 min-w-0 flex flex-col overflow-hidden">{blockGrid}</div>
            </div>
          )}
        </div>

        {/* Context Panel - TM & Terminology */}
        {showContextPanel && (
          <div
            className="w-[280px] min-w-[280px] border-l border-border bg-card overflow-auto p-3 shrink-0"
            data-testid="context-panel"
          >
            {contextLoading && (
              <div className="text-center py-3 text-xs text-muted-foreground">Loading...</div>
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
                <div className="text-xs text-muted-foreground italic py-2">
                  No TM matches for this block
                </div>
              ) : (
                tmMatches.map((m, i) => (
                  <div
                    key={i}
                    className={cn(
                      "p-2 bg-muted rounded-md mb-1.5 border border-border",
                      appliedTMIndex === i && "border-green-500 bg-green-500/5",
                    )}
                    data-testid={`tm-match-${i}`}
                  >
                    <div className="flex justify-between mb-1">
                      <span
                        className={cn(
                          "text-[11px] font-bold px-1.5 py-px rounded",
                          tmScoreClass(m.score),
                        )}
                      >
                        {Math.round(m.score * 100)}%
                      </span>
                      <span className="text-[10px] text-muted-foreground">
                        {m.match_type.replace(/-/g, " ")}
                      </span>
                      {m.project_id && (
                        <span
                          className={cn(
                            "text-[10px] px-1 py-px rounded ml-1",
                            m.project_id === project.id
                              ? "text-green-600 dark:text-green-400 bg-green-500/10"
                              : "text-blue-600 dark:text-blue-400 bg-blue-500/10",
                          )}
                        >
                          {m.project_id === project.id ? "same project" : "cross-project"}
                        </span>
                      )}
                    </div>
                    <div className="text-xs mb-1 text-muted-foreground">{m.source}</div>
                    <div className="text-xs font-medium">{m.target}</div>
                    <Button
                      size="sm"
                      className={cn(
                        "mt-1.5 text-[11px] h-6 px-2",
                        appliedTMIndex === i &&
                          "bg-green-500 hover:bg-green-500 opacity-80 cursor-default",
                      )}
                      onClick={() => {
                        const block = filteredBlocks[selectedIndex];
                        if (!block || !block.translatable) return;
                        void api
                          .updateBlockTarget({
                            project_id: project.id,
                            item_name: fileName,
                            block_id: block.id,
                            target_locale: targetLocale,
                            text: m.target,
                          })
                          .then(() => {
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
                <div className="text-xs text-muted-foreground italic py-2">
                  No terms found in this block
                </div>
              ) : (
                termMatches.map((m, i) => (
                  <div
                    key={i}
                    className="p-2 bg-muted rounded-md mb-1.5 border border-border"
                    data-testid={`term-match-${i}`}
                  >
                    <div className="flex items-center gap-1.5 mb-1">
                      <span className="text-[13px] font-semibold">{m.source_term}</span>
                      <span
                        className={cn(
                          "text-[10px] font-semibold px-1.5 py-px rounded",
                          termStatusClass(m.status),
                        )}
                      >
                        {m.status}
                      </span>
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
                      <span className="inline-block mt-1 text-[10px] text-muted-foreground px-1.5 py-px rounded bg-card border border-border">
                        {m.domain}
                      </span>
                    )}
                  </div>
                ))
              )}
            </div>

            {/* Entities */}
            {(() => {
              const currentEntities = filteredBlocks[selectedIndex]?.entities ?? [];
              return (
                <div className="mt-4">
                  <div className="text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 pb-1 border-b border-border">
                    Entities
                    {currentEntities.length > 0 && (
                      <span className="ml-1.5 font-normal text-[10px]">
                        ({currentEntities.length})
                      </span>
                    )}
                  </div>
                  {!contextLoading && currentEntities.length === 0 ? (
                    <div className="text-xs text-muted-foreground italic py-2">
                      No entities in this block
                    </div>
                  ) : (
                    currentEntities.map((e: EntityInfo, i: number) => (
                      <div
                        key={e.key}
                        className="p-2 bg-muted rounded-md mb-1.5 border border-border"
                        data-testid={`entity-${i}`}
                      >
                        <div className="flex items-center gap-1.5 mb-1">
                          <span className="text-[13px] font-semibold">{e.text}</span>
                          {e.dnt && (
                            <span className="text-[10px] font-semibold px-1.5 py-px rounded bg-red-500/10 text-red-500">
                              DNT
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-1.5">
                          <span className="text-[10px] px-1.5 py-px rounded bg-card border border-border text-muted-foreground">
                            {entityLabel(e.type)}
                          </span>
                          {e.source && (
                            <span className="text-[10px] text-muted-foreground">{e.source}</span>
                          )}
                        </div>
                      </div>
                    ))
                  )}
                </div>
              );
            })()}
          </div>
        )}
      </div>

      {/* Status bar */}
      <div
        className="flex justify-between py-2 text-xs text-muted-foreground"
        data-testid="status-bar"
      >
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
          Enter: edit | Esc: cancel | <ArrowUp className="w-3 h-3 inline-block" />
          <ArrowDown className="w-3 h-3 inline-block" />: navigate
          {editingIndex !== null && filteredBlocks[editingIndex]?.has_spans && (
            <> | Ctrl+1..9: insert tag</>
          )}
          {editingIndex === null && <> | {"\u2318"}E: mark entity</>}
        </span>
      </div>

      {/* Entity mark popover */}
      {entityMarkState && (
        <EntityMarkPopover
          text={entityMarkState.text}
          start={entityMarkState.start}
          end={entityMarkState.end}
          position={entityMarkState.position}
          onConfirm={handleCreateEntity}
          onCancel={() => setEntityMarkState(null)}
        />
      )}
    </div>
  );
}

/** Row-level validation indicator for tag mismatches. */
function RowTagWarning({
  sourceSpans,
  targetCodedText,
}: {
  sourceSpans: SpanInfo[];
  targetCodedText: string;
}) {
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
      data-testid="tag-warning"
      className={cn(
        "ml-1 cursor-help inline-flex",
        validation.errors.length > 0 ? "text-red-600" : "text-amber-700",
      )}
    >
      <AlertTriangle className="w-3.5 h-3.5" />
    </span>
  );
}
