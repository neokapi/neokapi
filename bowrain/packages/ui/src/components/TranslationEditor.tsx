import {
  Alert,
  AlertDescription,
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Tabs,
  TabsList,
  TabsTrigger,
} from "@neokapi/ui-primitives";
import { useState, useEffect, useCallback, useMemo } from "react";
import type {
  ProjectInfo,
  BlockInfo,
  WordCountResult,
  TMMatchInfo,
  BlockTermMatch,
  BlockNote,
  QAIssue,
  BlockHistoryEntry,
  FileQAResult,
  AddConceptRequest,
} from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { useLocales } from "../hooks/useLocales";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";
import { EntityMarkPopover } from "./editor/EntityMarkPopover";
import { VisualEditorLayout } from "./editor/VisualEditorLayout";
import { TableView } from "./editor/TableView";
import { getBlockStatus } from "./editor/blockStatus";
import { ArrowLeft, ArrowUp, ArrowDown } from "./icons";
import { type UnifiedSaveResult } from "./UnifiedTargetEditor";

/** The Translate editor exposes two views the user toggles between. */
export type TranslateView = "visual" | "table";

interface TranslationEditorProps {
  project: ProjectInfo;
  fileName: string;
  onBack: () => void;
  /** Optional export handler override. If not provided, browser file download is used. */
  onExport?: (blob: Blob, fileName: string) => void;
  /** Optional presence slot rendered in the editor toolbar. */
  presenceSlot?: React.ReactNode;
  /**
   * Optional callback fired when the focused/selected block changes. Used by
   * presence collaboration (Yjs awareness) to broadcast the local user's
   * cursor position — see useCollaboration().setSelectedBlock. Undefined is
   * passed when no block is selected.
   */
  onSelectedBlockChange?: (blockId: string | undefined) => void;
  /** Initial view; defaults to "visual". */
  defaultView?: TranslateView;
  /** Optional slot for the cross-surface switcher (Pre-process/Translate/Review). */
  surfaceTabs?: React.ReactNode;
  /**
   * Monotonic counter that forces a reload of the editor's blocks + word count
   * when it changes. Both the web (EventSource → invalidate) and desktop
   * (gRPC WatchProject → events) freshness layers bump this when an external
   * change touches this file's project, so the open editor never shows stale
   * targets after another user's edit, a kapi push, or a flow/sync completion.
   */
  reloadSignal?: number;
}

export function TranslationEditor({
  project,
  fileName,
  onBack,
  onExport,
  presenceSlot,
  onSelectedBlockChange,
  defaultView = "visual",
  surfaceTabs,
  reloadSignal,
}: TranslationEditorProps) {
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [targetLocale, setTargetLocale] = useState(project.target_languages[0] || "");
  const [wordCount, setWordCount] = useState<WordCountResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [view, setView] = useState<TranslateView>(defaultView);

  // Per-block linguistic context (TM + terms) loaded on selection.
  const [tmMatches, setTmMatches] = useState<TMMatchInfo[]>([]);
  const [termMatches, setTermMatches] = useState<BlockTermMatch[]>([]);

  // Visual-card extended state (QA, history, notes) — loaded for the selected
  // block; only surfaced in the Visual view's card.
  const [blockQAIssues, setBlockQAIssues] = useState<QAIssue[]>([]);
  const [fileQAResults, setFileQAResults] = useState<FileQAResult[] | undefined>(undefined);
  const [qaLoading, setQaLoading] = useState(false);
  const [blockHistory, setBlockHistory] = useState<BlockHistoryEntry[]>([]);
  const [blockNotes, setBlockNotes] = useState<BlockNote[]>([]);

  // Entity marking state (Cmd+E)
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
    // reloadSignal is an external freshness trigger: bumping it re-runs this
    // effect to pull authoritative state after an out-of-band change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loadBlocks, loadWordCount, reloadSignal]);

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

  // Selected block ID for preview synchronization + presence.
  const selectedBlockId = filteredBlocks[selectedIndex]?.id;

  useEffect(() => {
    onSelectedBlockChange?.(selectedBlockId);
  }, [selectedBlockId, onSelectedBlockChange]);

  const startEditing = useCallback(
    (index: number) => {
      const block = filteredBlocks[index];
      if (!block || !block.translatable) return;
      setEditingIndex(index);
    },
    [filteredBlocks],
  );

  // Keyboard navigation + Cmd+E entity marking (active in both views; the
  // Visual card additionally owns approve/reject + j/k via its own hook, which
  // ignores keystrokes while focus is in an input/editor).
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;
      if (editingIndex !== null) {
        if (e.key === "Escape") setEditingIndex(null);
        return;
      }

      // Cmd+E: mark selected source text as entity.
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

      // Table-view navigation (Visual view drives its own j/k via the hook).
      if (view !== "table") return;
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
  }, [editingIndex, selectedIndex, filteredBlocks, view, startEditing]);

  // Load TM + term matches for the selected block.
  useEffect(() => {
    const block = filteredBlocks[selectedIndex];
    if (!block || !block.translatable) {
      setTmMatches([]);
      setTermMatches([]);
      return;
    }
    const tmPromise = api
      .lookupTMForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTmMatches(m || []))
      .catch(() => setTmMatches([]));
    const termPromise = api
      .lookupTermsForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTermMatches(m || []))
      .catch(() => setTermMatches([]));
    void Promise.all([tmPromise, termPromise]);
  }, [selectedIndex, filteredBlocks, targetLocale, project.id, fileName, api]);

  // Load QA issues, history, and notes for the selected block (Visual card).
  useEffect(() => {
    if (view !== "visual") return;
    const block = filteredBlocks[selectedIndex];
    if (!block) return;
    api
      .runQACheck(project.id, block.id, targetLocale)
      .then((issues) => setBlockQAIssues(issues || []))
      .catch(() => setBlockQAIssues([]));
    api
      .getBlockHistory(project.id, block.id, targetLocale, 20)
      .then((h) => setBlockHistory(h || []))
      .catch(() => setBlockHistory([]));
    api
      .listBlockNotes(project.id, block.id)
      .then((n) => setBlockNotes(n || []))
      .catch(() => setBlockNotes([]));
  }, [view, selectedIndex, filteredBlocks, targetLocale, project.id, api]);

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

  // Single dispatcher for the UnifiedTargetEditor — flat results go through
  // `updateBlockTargetCoded`; plural results write the ICU string to
  // `targets[locale]` and clear `targets_coded[locale]`. See AD #408 / #409.
  const handleUnifiedSave = useCallback(
    async (index: number, result: UnifiedSaveResult) => {
      const block = filteredBlocks[index];
      if (!block) return;
      try {
        if (result.kind === "flat") {
          await api.updateBlockTargetCoded({
            project_id: project.id,
            item_name: fileName,
            block_id: block.id,
            target_locale: targetLocale,
            coded_text: result.codedText,
            spans: result.spans,
          });
          const plainText = result.codedText.replace(/[\uE001-\uE003]/g, "");
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === block.id
                ? {
                    ...b,
                    targets: { ...b.targets, [targetLocale]: plainText },
                    targets_coded: { ...b.targets_coded, [targetLocale]: result.codedText },
                  }
                : b,
            ),
          );
        } else {
          await api.updateBlockTargetCoded({
            project_id: project.id,
            item_name: fileName,
            block_id: block.id,
            target_locale: targetLocale,
            coded_text: "",
            spans: [],
          });
          await api.updateBlockTarget({
            project_id: project.id,
            item_name: fileName,
            block_id: block.id,
            target_locale: targetLocale,
            text: result.text,
          });
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === block.id
                ? {
                    ...b,
                    targets: { ...b.targets, [targetLocale]: result.text },
                    targets_coded: { ...b.targets_coded, [targetLocale]: "" },
                  }
                : b,
            ),
          );
        }
        const nextIndex = index + 1;
        setEditingIndex(null);
        if (nextIndex < filteredBlocks.length) setSelectedIndex(nextIndex);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to save target");
      }
    },
    [filteredBlocks, api, project.id, fileName, targetLocale],
  );

  const handleExport = async () => {
    setLoading(true);
    setError(null);
    try {
      const blob = await api.exportTranslatedFile(project.id, fileName, targetLocale);
      if (onExport) {
        onExport(blob, fileName);
      } else {
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

  const handleMarkReviewed = useCallback(() => {
    const block = filteredBlocks[selectedIndex];
    if (!block) return;
    setBlocks((prev) =>
      prev.map((b) =>
        b.id === block.id
          ? { ...b, properties: { ...b.properties, "translation-status": "reviewed" } }
          : b,
      ),
    );
  }, [filteredBlocks, selectedIndex]);

  // Visual card handlers.
  const handleVisualSave = useCallback(
    async (result: UnifiedSaveResult) => {
      if (editingIndex === null) return;
      await handleUnifiedSave(editingIndex, result);
    },
    [editingIndex, handleUnifiedSave],
  );

  const handleVisualApprove = useCallback(() => {
    handleMarkReviewed();
    if (selectedIndex < filteredBlocks.length - 1) setSelectedIndex(selectedIndex + 1);
  }, [handleMarkReviewed, selectedIndex, filteredBlocks.length]);

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

  const handleApplyTM = useCallback(
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
        });
    },
    [tmMatches, filteredBlocks, selectedIndex, api, project.id, fileName, targetLocale],
  );

  // Insert a target term: append it to the selected block's target and persist.
  // Works whether or not a cell is being edited — the appended text lands in the
  // saved target and reloads into the editor on next open.
  const handleInsertTerm = useCallback(
    (text: string) => {
      const block = filteredBlocks[selectedIndex];
      if (!block || !block.translatable) return;
      const existing = block.targets[targetLocale] || "";
      const next = existing ? `${existing} ${text}` : text;
      void api
        .updateBlockTarget({
          project_id: project.id,
          item_name: fileName,
          block_id: block.id,
          target_locale: targetLocale,
          text: next,
        })
        .then(() => {
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === block.id ? { ...b, targets: { ...b.targets, [targetLocale]: next } } : b,
            ),
          );
        })
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to insert term"));
    },
    [filteredBlocks, selectedIndex, api, project.id, fileName, targetLocale],
  );

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

  const handleNavigate = useCallback((index: number) => {
    setSelectedIndex(index);
    setEditingIndex(null);
  }, []);

  // Build progress bar segments
  const progressSegments = (
    <div className="flex h-full w-full absolute top-0 left-0">
      {statusCounts.reviewed > 0 && (
        <div
          data-testid="progress-reviewed"
          className="bg-success opacity-40"
          style={{
            width: `${(statusCounts.reviewed / Math.max(translatableBlocks.length, 1)) * 100}%`,
          }}
        />
      )}
      {statusCounts.translated > 0 && (
        <div
          data-testid="progress-translated"
          className="bg-info opacity-40"
          style={{
            width: `${(statusCounts.translated / Math.max(translatableBlocks.length, 1)) * 100}%`,
          }}
        />
      )}
      {statusCounts.draft > 0 && (
        <div
          data-testid="progress-draft"
          className="bg-warning opacity-40"
          style={{
            width: `${(statusCounts.draft / Math.max(translatableBlocks.length, 1)) * 100}%`,
          }}
        />
      )}
    </div>
  );

  const progressBreakdown: string[] = [];
  if (statusCounts.reviewed > 0) progressBreakdown.push(`${statusCounts.reviewed} reviewed`);
  if (statusCounts.translated > 0) progressBreakdown.push(`${statusCounts.translated} translated`);
  if (statusCounts.draft > 0) progressBreakdown.push(`${statusCounts.draft} draft`);
  if (statusCounts["not-started"] > 0)
    progressBreakdown.push(`${statusCounts["not-started"]} pending`);

  return (
    <div className="flex flex-col flex-1 min-h-0">
      {/* Header */}
      <div className="flex items-center gap-3 mb-3">
        {surfaceTabs}
        <span className="text-base font-semibold flex-1 truncate">{fileName}</span>
        {presenceSlot}
        {/* View toggle: Visual ↔ Table */}
        <Tabs value={view} onValueChange={(v: string) => setView(v as TranslateView)}>
          <TabsList className="h-8" data-testid="view-switcher">
            <TabsTrigger value="visual" className="text-[11px] px-3 h-7" data-testid="view-visual">
              Visual
            </TabsTrigger>
            <TabsTrigger value="table" className="text-[11px] px-3 h-7" data-testid="view-table">
              Table
            </TabsTrigger>
          </TabsList>
        </Tabs>
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

      {/* Toolbar: search (Table view only — Visual drives navigation inline) */}
      {view === "table" && (
        <div className="flex gap-2 py-2 items-center flex-wrap backdrop-blur-sm">
          <div className="flex-1" />
          <input
            type="text"
            placeholder="Search blocks..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="px-3 py-1.5 bg-muted border border-border rounded-md text-foreground text-sm outline-none w-[200px]"
            data-testid="search-input"
          />
        </div>
      )}

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
          {progressBreakdown.length > 0 && ` — ${progressBreakdown.join(", ")}`}
        </span>
      </div>

      {/* Messages */}
      {error && (
        <Alert variant="destructive" className="mb-2">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {message && (
        <Alert className="mb-2 border-success/25 text-success dark:border-success/40 dark:text-success">
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}

      {/* View body */}
      <div className="flex flex-1 overflow-hidden min-h-0">
        <div className="flex-1 flex flex-col overflow-hidden min-h-0">
          {view === "visual" ? (
            <div className="flex-1 min-h-0 relative">
              <VisualEditorLayout
                project={project}
                fileName={fileName}
                blocks={filteredBlocks}
                selectedIndex={selectedIndex}
                editingIndex={editingIndex}
                targetLocale={targetLocale}
                onNavigate={handleNavigate}
                onStartEditing={() => startEditing(selectedIndex)}
                onSave={handleVisualSave}
                onCancelEditing={() => setEditingIndex(null)}
                onApprove={handleVisualApprove}
                onReject={handleVisualReject}
                tmMatches={tmMatches}
                termMatches={termMatches}
                onApplyTM={handleApplyTM}
                onInsertTerm={handleInsertTerm}
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
          ) : (
            <TableView
              blocks={filteredBlocks}
              targetLocale={targetLocale}
              targetLocaleLabel={getDisplayName(targetLocale)}
              selectedIndex={selectedIndex}
              editingIndex={editingIndex}
              searchQuery={searchQuery}
              selectedTermMatches={termMatches}
              onSelect={setSelectedIndex}
              onStartEditing={startEditing}
              onCancelEditing={() => setEditingIndex(null)}
              onSave={handleUnifiedSave}
            />
          )}
        </div>
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
          {editingIndex === null && <> | {"⌘"}E: mark entity</>}
        </span>
      </div>

      {/* Entity mark popover (Cmd+E) */}
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
