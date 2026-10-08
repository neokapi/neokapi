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
import type { Run } from "@neokapi/kapi-format";
import { useState, useEffect, useCallback, useRef } from "react";
import { ErrorNotice } from "../errors";
import type {
  ProjectInfo,
  BlockInfo,
  BlockCounts,
  ReviewRung,
  WordCountResult,
  MemoryMatchInfo,
  BlockTermMatch,
  BlockNote,
  CheckIssue,
  BlockHistoryEntry,
  FileCheckResult,
  AddConceptRequest,
} from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useContentChanges } from "../hooks/useContentChanges";
import { useApi } from "../context/ApiContext";
import { useAnalytics } from "../context/AnalyticsContext";
import { AnalyticsEvents } from "../analytics-events";
import { useWorkspace } from "../context/WorkspaceContext";
import { useLocales } from "../hooks/useLocales";
import {
  addNote,
  appendText,
  decideTranslation,
  decisionOutcome,
  markEntity,
  removeNote,
  renderedRevision,
  setTranslation,
  translationRuns,
} from "../api/contentChanges";
import { EntityMarkPopover } from "./editor/EntityMarkPopover";
import { VisualEditorLayout } from "./editor/VisualEditorLayout";
import { TableView } from "./editor/TableView";
import { StaleChangeDialog } from "./editor/StaleChangeDialog";
import { CheckFindingsDialog } from "./editor/CheckFindingsDialog";
import {
  captureTargetStatus,
  getTargetText,
  rollbackTargetStatus,
  withTargetStatus,
  type TargetStatusSnapshot,
} from "./editor/blockStatus";
import {
  savedRuns,
  savedTranslation,
  withSavedTranslation,
  type SavedTranslation,
} from "./editor/savedTranslation";
import { ArrowUp, ArrowDown } from "./icons";
import { type UnifiedSaveResult, type UnifiedTargetEditorHandle } from "./UnifiedTargetEditor";

/** The Translate editor exposes two views the user toggles between. */
export type TranslateView = "visual" | "table";

/** How long the search box waits, in ms, before it becomes a server query. */
const SEARCH_DEBOUNCE_MS = 250;

/** The four buckets, all zero — the shape shown before the counts arrive. */
const EMPTY_COUNTS: BlockCounts = {
  total: 0,
  translatable: 0,
  status: { "not-started": 0, draft: 0, translated: 0, established: 0 },
};

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
  onBack: _onBack,
  onExport,
  presenceSlot,
  onSelectedBlockChange,
  defaultView = "visual",
  surfaceTabs,
  reloadSignal,
}: TranslationEditorProps) {
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [counts, setCounts] = useState<BlockCounts>(EMPTY_COUNTS);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [targetLocale, setTargetLocale] = useState(project.target_languages[0] || "");
  const [wordCount, setWordCount] = useState<WordCountResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<{ title: string; cause?: unknown } | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [view, setView] = useState<TranslateView>(defaultView);

  // Per-block linguistic context (content memory + terms) loaded on selection.
  const [memoryMatches, setTmMatches] = useState<MemoryMatchInfo[]>([]);
  const [termMatches, setTermMatches] = useState<BlockTermMatch[]>([]);

  // Visual-card extended state (findings, history, notes) — loaded for the selected
  // block; only surfaced in the Visual view's card.
  const [blockCheckIssues, setBlockCheckIssues] = useState<CheckIssue[]>([]);
  const [fileCheckResults, setFileCheckResults] = useState<FileCheckResult[] | undefined>(
    undefined,
  );
  const [fileCheckError, setFileCheckError] = useState<unknown>(undefined);
  const [checksLoading, setChecksLoading] = useState(false);
  const [blockHistory, setBlockHistory] = useState<BlockHistoryEntry[]>([]);
  const [blockNotes, setBlockNotes] = useState<BlockNote[]>([]);

  // Entity marking state (Cmd+E)
  const [entityMarkState, setEntityMarkState] = useState<{
    text: string;
    start: number;
    end: number;
    position: { x: number; y: number };
  } | null>(null);

  // Imperative handle of the currently-open target editor (Visual card or
  // Table cell — only one mounts at a time). React nulls it on unmount, so
  // `targetEditorRef.current` doubles as an "is an editor open" probe for
  // the term-insert-at-cursor path.
  const targetEditorRef = useRef<UnifiedTargetEditorHandle | null>(null);

  const { getDisplayName } = useLocales();
  const fullApi = useApi();
  const { activeWorkspace } = useWorkspace();
  const wsSlug = activeWorkspace?.slug ?? "";

  // Register breadcrumb in the top bar area

  const api = useEditorApi();
  const changes = useContentChanges(project.id);
  const { capture } = useAnalytics();
  const { getFileBlocks, getBlockCounts, getWordCount: getWordCountApi } = api;

  // The search box is a server query, so the keystrokes settle before it runs.
  const [query, setQuery] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setQuery(searchQuery.trim()), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [searchQuery]);

  // The editor holds one item and one locale at a time, and asks for exactly
  // that: the search runs over the runs server-side, so the list is the answer
  // rather than a filtered download.
  const loadBlocks = useCallback(async () => {
    try {
      const b = await getFileBlocks(project.id, fileName, {
        locale: targetLocale,
        q: query || undefined,
      });
      setBlocks(b || []);
    } catch (e) {
      setError({ title: "Couldn't load the blocks", cause: e });
    }
  }, [getFileBlocks, project.id, fileName, targetLocale, query]);

  // The progress bar reports the histogram for the same query, counted in SQL.
  const loadCounts = useCallback(async () => {
    try {
      setCounts(
        await getBlockCounts(project.id, fileName, targetLocale, { q: query || undefined }),
      );
    } catch {
      setCounts(EMPTY_COUNTS);
    }
  }, [getBlockCounts, project.id, fileName, targetLocale, query]);

  // Read one block back as the server holds it: after a change the person
  // chose not to apply over someone else's, and after a server action that
  // rewrote it. The page reload is the fallback when the read fails.
  const refreshBlock = useCallback(
    async (blockId: string) => {
      try {
        const fresh = await api.getBlock(project.id, blockId);
        setBlocks((prev) => prev.map((b) => (b.id === blockId ? fresh : b)));
      } catch {
        await loadBlocks();
      }
    },
    [api, project.id, loadBlocks],
  );

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
    void loadCounts();
    void loadWordCount();
    // reloadSignal is an external freshness trigger: bumping it re-runs this
    // effect to pull authoritative state after an out-of-band change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loadBlocks, loadCounts, loadWordCount, reloadSignal]);

  // Keep the selection inside the loaded range (a narrowing query would
  // otherwise leave selectedIndex dangling past the end and blank the
  // Visual card).
  useEffect(() => {
    setSelectedIndex((i) => Math.max(0, Math.min(i, blocks.length - 1)));
  }, [blocks.length]);

  // Changing the query reshuffles which block sits at each index, so close
  // any open editor rather than let it silently re-seed on another block.
  const handleSearchChange = useCallback((value: string) => {
    setSearchQuery(value);
    setEditingIndex(null);
  }, []);

  const statusCounts = counts.status;
  const translatableCount = counts.translatable;
  const translatedCount = translatableCount - statusCounts["not-started"];
  const progress =
    translatableCount > 0 ? Math.round((translatedCount / translatableCount) * 100) : 0;

  // Selected block ID for preview synchronization + presence.
  const selectedBlockId = blocks[selectedIndex]?.id;

  useEffect(() => {
    onSelectedBlockChange?.(selectedBlockId);
  }, [selectedBlockId, onSelectedBlockChange]);

  const startEditing = useCallback(
    (index: number) => {
      const block = blocks[index];
      if (!block || !block.translatable) return;
      setEditingIndex(index);
    },
    [blocks],
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
          const block = blocks[selectedIndex];
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
        setSelectedIndex((i) => Math.min(i + 1, blocks.length - 1));
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
  }, [editingIndex, selectedIndex, blocks, view, startEditing]);

  // Load content memory + term matches for the selected block.
  useEffect(() => {
    const block = blocks[selectedIndex];
    if (!block || !block.translatable) {
      setTmMatches([]);
      setTermMatches([]);
      return;
    }
    const memoryPromise = api
      .lookupMemoryForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTmMatches(m || []))
      .catch(() => setTmMatches([]));
    const termPromise = api
      .lookupTermsForBlock(project.id, fileName, block.id, targetLocale)
      .then((m) => setTermMatches(m || []))
      .catch(() => setTermMatches([]));
    void Promise.all([memoryPromise, termPromise]);
  }, [selectedIndex, blocks, targetLocale, project.id, fileName, api]);

  // Load findings, history, and notes for the selected block (Visual card).
  useEffect(() => {
    if (view !== "visual") return;
    const block = blocks[selectedIndex];
    if (!block) return;
    api
      .runCheck(project.id, block.id, targetLocale)
      .then((issues) => setBlockCheckIssues(issues || []))
      .catch(() => setBlockCheckIssues([]));
    api
      .getBlockHistory(project.id, block.id, targetLocale, 20)
      .then((h) => setBlockHistory(h || []))
      .catch(() => setBlockHistory([]));
    api
      .listBlockNotes(project.id, block.id)
      .then((n) => setBlockNotes(n || []))
      .catch(() => setBlockNotes([]));
  }, [view, selectedIndex, blocks, targetLocale, project.id, api]);

  const handleCreateEntity = useCallback(
    async (type: string, dnt: boolean) => {
      if (!entityMarkState || selectedIndex < 0) return;
      const block = blocks[selectedIndex];
      if (!block) return;
      try {
        await changes.apply([
          markEntity(fileName, block, {
            text: entityMarkState.text,
            type,
            start: entityMarkState.start,
            end: entityMarkState.end,
            dnt,
            source: "manual",
          }),
        ]);
        // The server anchors the entity and names it; the block read back
        // carries it the way the editor lists entities.
        await refreshBlock(block.id);
      } catch (err) {
        setError({ title: "Couldn't create the entity", cause: err });
      }
      setEntityMarkState(null);
    },
    [entityMarkState, selectedIndex, blocks, changes, fileName, refreshBlock],
  );

  /**
   * Save a translation the person wrote, as runs, on the revision the editor
   * showed. A translation someone changed since is shown to them before
   * anything is written; when they keep it, the block is read back as it
   * stands. A save a check refuses shows the findings, and the person saves
   * anyway or goes back to their wording (`revise`). A save that lands is
   * written into the block as a reload would fetch it.
   */
  const saveTranslation = useCallback(
    async (
      block: BlockInfo,
      saved: SavedTranslation,
      extra?: Partial<BlockInfo>,
    ): Promise<"saved" | "kept" | "revise"> => {
      const outcome = await changes.commit(
        (ifMatch) => setTranslation(fileName, block, targetLocale, { runs: saved.runs }, ifMatch),
        renderedRevision(block, targetLocale),
        { action: "save", locale: targetLocale, mine: saved.mine },
      );
      if (outcome.status === "kept") {
        await refreshBlock(block.id);
        return "kept";
      }
      if (outcome.status === "revise") return "revise";
      setBlocks((prev) =>
        prev.map((b) =>
          b.id === block.id
            ? { ...withSavedTranslation(b, targetLocale, saved, outcome.after), ...extra }
            : b,
        ),
      );
      return "saved";
    },
    [changes, fileName, targetLocale, refreshBlock],
  );

  // Single dispatcher for the UnifiedTargetEditor: a flat result and a plural
  // one are both saved as runs, the plural as one plural run with its forms.
  // See AD #408 / #409.
  const handleUnifiedSave = useCallback(
    async (index: number, result: UnifiedSaveResult) => {
      const block = blocks[index];
      if (!block) return;
      try {
        const done = await saveTranslation(block, savedTranslation(result));
        // A save the person takes back to revise keeps the editor open on
        // their wording.
        if (done === "revise") return;
        if (done === "kept") {
          setEditingIndex(null);
          return;
        }
        capture(AnalyticsEvents.translationSaved, { locale: targetLocale, method: "editor" });
        // The block changed bucket, so the histogram is re-asked for.
        void loadCounts();
        const nextIndex = index + 1;
        setEditingIndex(null);
        if (nextIndex < blocks.length) setSelectedIndex(nextIndex);
      } catch (e) {
        setError({ title: "Couldn't save the translation", cause: e });
      }
    },
    [blocks, saveTranslation, capture, targetLocale, loadCounts],
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
      setError({ title: "Couldn't export the file", cause: e });
    } finally {
      setLoading(false);
    }
  };

  // Persist a review decision: optimistic per-locale Edition.Status write
  // (the shape a reload would fetch), server call, rollback + error on failure.
  // The rollback snapshot is captured inside the setBlocks updater — from the
  // state the write actually replaced, not a possibly stale component-scope
  // block — and restores only the status field, so a target save that lands
  // while the review call is in flight is preserved. `rung` picks where the
  // call lands on a clearing call: "draft" for a reviewer rejection,
  // translated by default. An approval always lands on established.
  const applyReview = useCallback(
    async (block: BlockInfo, reviewed: boolean, rung?: ReviewRung): Promise<boolean> => {
      // Clearing the review state of a locale with no translation is a no-op:
      // the server treats it as an idempotent 200 success, so an optimistic
      // write here would fabricate a phantom {text: "", status} entry that no
      // rollback ever removes (approval is guarded by the callers and the
      // server's 422).
      if (!reviewed && !getTargetText(block, targetLocale).trim()) return true;
      capture(AnalyticsEvents.reviewDecisionClicked, {
        decision: reviewed ? "approve" : rung === "draft" ? "reject" : "clear",
        locale: targetLocale,
      });
      let snapshot: TargetStatusSnapshot = { existed: false, status: "" };
      setBlocks((prev) =>
        prev.map((b) => {
          if (b.id !== block.id) return b;
          snapshot = captureTargetStatus(b, targetLocale);
          return withTargetStatus(
            b,
            targetLocale,
            reviewed ? "established" : (rung ?? "translated"),
          );
        }),
      );
      const outcome = decisionOutcome(reviewed, rung);
      try {
        // The decision binds to the wording the reviewer read: a translation
        // someone changed since is shown before anything is recorded.
        const done = await changes.commit(
          (ifMatch) => decideTranslation(fileName, block, targetLocale, outcome, ifMatch),
          renderedRevision(block, targetLocale),
          { action: outcome, locale: targetLocale },
        );
        if (done.status === "kept") {
          await refreshBlock(block.id);
          return false;
        }
        void loadCounts();
        return true;
      } catch (e) {
        setBlocks((prev) =>
          prev.map((b) =>
            b.id === block.id ? rollbackTargetStatus(b, targetLocale, snapshot) : b,
          ),
        );
        setError({
          title: reviewed
            ? "Couldn't mark the block as approved"
            : "Couldn't update the review status",
          cause: e,
        });
        return false;
      }
    },
    [changes, capture, fileName, targetLocale, loadCounts, refreshBlock],
  );

  // Visual card handlers.
  const handleVisualSave = useCallback(
    async (result: UnifiedSaveResult) => {
      if (editingIndex === null) return;
      await handleUnifiedSave(editingIndex, result);
    },
    [editingIndex, handleUnifiedSave],
  );

  const handleVisualApprove = useCallback(() => {
    const block = blocks[selectedIndex];
    // An untranslated block has nothing to review — the server categorically
    // 422s it, so don't fire a call that can only fail (mirrors the server's
    // no-empty-translation rule client-side; the card also disables Approve).
    if (!block || !getTargetText(block, targetLocale).trim()) return;
    const index = selectedIndex;
    const total = blocks.length;
    // Only advance once the review persisted: advancing optimistically would
    // bounce the reviewer to the next block and then surface the rollback +
    // error for a block no longer on screen.
    void applyReview(block, true).then((ok) => {
      if (ok) setSelectedIndex((i) => (i === index && i < total - 1 ? i + 1 : i));
    });
  }, [blocks, selectedIndex, targetLocale, applyReview]);

  const handleVisualReject = useCallback(() => {
    const block = blocks[selectedIndex];
    if (!block) return;
    // A rejection demotes the target to draft so the unit re-enters the work
    // queue (host's rejected → draft mapping) — not merely back to translated,
    // which would leave the rejected text passing translated-coverage gates.
    void applyReview(block, false, "draft");
  }, [selectedIndex, blocks, applyReview]);

  const handleApplyMemory = useCallback(
    (index: number) => {
      const match = memoryMatches[index];
      const block = blocks[selectedIndex];
      if (!match || !block || !block.translatable) return;
      // The match's runs carry its codes; its text alone would drop them.
      const runs: Run[] =
        match.target_runs && match.target_runs.length > 0
          ? (match.target_runs as Run[])
          : [{ text: match.target }];
      void saveTranslation(block, savedRuns(runs), {
        properties: { ...block.properties, "translation-origin": "memory" },
      })
        .then((done) => {
          if (done !== "saved") return;
          capture(AnalyticsEvents.translationSaved, { locale: targetLocale, method: "tm" });
          void loadCounts();
        })
        .catch((e) => setError({ title: "Couldn't apply the match", cause: e }));
    },
    [memoryMatches, blocks, selectedIndex, saveTranslation, capture, targetLocale, loadCounts],
  );

  // Insert a target term. When a target editor is open for the selected
  // block, insert at its Lexical cursor — the term joins the user's
  // in-progress edit and persists on their explicit save. With no editor
  // open, fall back to appending to the stored target and persisting.
  const handleInsertTerm = useCallback(
    (text: string) => {
      const block = blocks[selectedIndex];
      if (!block || !block.translatable) return;
      if (editingIndex !== null && targetEditorRef.current) {
        targetEditorRef.current.insertText(text);
        return;
      }
      // The term joins the translation's runs, so its codes stay where they are.
      const runs = appendText(translationRuns(block, targetLocale), text);
      void saveTranslation(block, savedRuns(runs)).catch((e) =>
        setError({ title: "Couldn't insert the term", cause: e }),
      );
    },
    [blocks, selectedIndex, editingIndex, saveTranslation, targetLocale],
  );

  const handleRunFileCheck = useCallback(() => {
    setChecksLoading(true);
    setFileCheckError(undefined);
    api
      .runFileCheck(project.id, fileName, targetLocale)
      .then((results) => setFileCheckResults(results || []))
      .catch((e: unknown) => {
        // A check that did not complete has no results to show.
        setFileCheckResults(undefined);
        setFileCheckError(e);
      })
      .finally(() => setChecksLoading(false));
  }, [api, project.id, fileName, targetLocale]);

  const handleRevertHistory = useCallback(
    (entry: BlockHistoryEntry) => {
      const block = blocks[selectedIndex];
      if (!block) return;
      // Use the audited server rollback: it restores the prior version
      // (including inline markup) non-destructively and records the rollback.
      // The block is read back, so the next change names the revision the
      // rollback left.
      api
        .rollbackBlock(project.id, block.id, entry.seq, targetLocale)
        .then(() => refreshBlock(block.id))
        .catch((e) => setError({ title: "Couldn't revert the change", cause: e }));
    },
    [blocks, selectedIndex, api, project.id, targetLocale, refreshBlock],
  );

  const handleAddNote = useCallback(
    (text: string) => {
      const block = blocks[selectedIndex];
      if (!block) return;
      // The server stamps a note with its author and time; the list read back
      // shows it as everyone else sees it. The note is added once the change
      // lands, read back or not: the desktop queues it while the server is out
      // of reach, and the list cannot be read until it returns.
      changes.apply([addNote(fileName, block.id, text)]).then(
        async (result) => {
          try {
            setBlockNotes((await api.listBlockNotes(project.id, block.id)) || []);
          } catch {
            const id = result.ops[0]?.id ?? `pending-${Date.now()}`;
            setBlockNotes((prev) => [
              ...prev,
              { id, blockId: block.id, author: "You", text, createdAt: new Date().toISOString() },
            ]);
          }
        },
        (e) => setError({ title: "Couldn't add the note", cause: e }),
      );
    },
    [blocks, selectedIndex, changes, api, project.id, fileName],
  );

  const handleDeleteNote = useCallback(
    (noteId: string) => {
      const block = blocks[selectedIndex];
      if (!block) return;
      changes
        .apply([removeNote(fileName, block.id, noteId)])
        .then(() => setBlockNotes((prev) => prev.filter((n) => n.id !== noteId)))
        .catch((e) => setError({ title: "Couldn't delete the note", cause: e }));
    },
    [blocks, selectedIndex, changes, fileName],
  );

  const handleTermCreate = useCallback(
    async (req: AddConceptRequest) => {
      try {
        await fullApi.addConcept(wsSlug, req);
      } catch (e) {
        setError({ title: "Couldn't create the term", cause: e });
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
      {statusCounts.established > 0 && (
        <div
          data-testid="progress-established"
          className="bg-success opacity-40"
          style={{
            width: `${(statusCounts.established / Math.max(translatableCount, 1)) * 100}%`,
          }}
        />
      )}
      {statusCounts.translated > 0 && (
        <div
          data-testid="progress-translated"
          className="bg-info opacity-40"
          style={{
            width: `${(statusCounts.translated / Math.max(translatableCount, 1)) * 100}%`,
          }}
        />
      )}
      {statusCounts.draft > 0 && (
        <div
          data-testid="progress-draft"
          className="bg-warning opacity-40"
          style={{
            width: `${(statusCounts.draft / Math.max(translatableCount, 1)) * 100}%`,
          }}
        />
      )}
    </div>
  );

  const progressBreakdown: string[] = [];
  if (statusCounts.established > 0) progressBreakdown.push(`${statusCounts.established} approved`);
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

      {/* Toolbar: search — one box, one filter state, shared by both views
          (Table rows and the Visual card list navigate the same filtered set). */}
      <div className="flex gap-2 py-2 items-center flex-wrap backdrop-blur-sm">
        <div className="flex-1" />
        <input
          type="text"
          placeholder="Search blocks..."
          value={searchQuery}
          onChange={(e) => handleSearchChange(e.target.value)}
          className="px-3 py-1.5 bg-muted border border-border rounded-md text-foreground text-sm outline-none w-[200px]"
          data-testid="search-input"
        />
      </div>

      {/* Progress bar. The track clips its coloured segments, and the label
          sits outside it so a long breakdown is centred over the bar rather
          than cut off at its edge. */}
      <div className="relative h-6 mb-2">
        <div
          className="absolute inset-0 bg-muted rounded overflow-hidden"
          data-testid="progress-bar"
        >
          {progressSegments}
        </div>
        <span
          className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-xs font-semibold text-foreground whitespace-nowrap"
          data-testid="progress-text"
        >
          {progress}% ({translatedCount}/{translatableCount} translated)
          {progressBreakdown.length > 0 && ` · ${progressBreakdown.join(", ")}`}
        </span>
      </div>

      {/* Messages */}
      {error && (
        <ErrorNotice title={error.title} error={error.cause} variant="inline" className="mb-2" />
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
                blocks={blocks}
                selectedIndex={selectedIndex}
                editingIndex={editingIndex}
                targetLocale={targetLocale}
                onNavigate={handleNavigate}
                onStartEditing={() => startEditing(selectedIndex)}
                onSave={handleVisualSave}
                onCancelEditing={() => setEditingIndex(null)}
                onApprove={handleVisualApprove}
                onReject={handleVisualReject}
                memoryMatches={memoryMatches}
                termMatches={termMatches}
                onApplyMemory={handleApplyMemory}
                onInsertTerm={handleInsertTerm}
                presenceSlot={presenceSlot}
                checkIssues={blockCheckIssues}
                fileCheckResults={fileCheckResults}
                fileCheckError={fileCheckError}
                checksLoading={checksLoading}
                onRunFileCheck={handleRunFileCheck}
                history={blockHistory}
                onRevertHistory={handleRevertHistory}
                notes={blockNotes}
                onAddNote={handleAddNote}
                onDeleteNote={handleDeleteNote}
                onTermCreate={handleTermCreate}
                targetEditorRef={targetEditorRef}
              />
            </div>
          ) : (
            <TableView
              blocks={blocks}
              sourceLocale={project.default_source_language}
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
              targetEditorRef={targetEditorRef}
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
          Block {selectedIndex + 1} of {blocks.length}
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
          {editingIndex !== null && blocks[editingIndex]?.has_spans && (
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
      <StaleChangeDialog state={changes.staleDialog} />
      <CheckFindingsDialog state={changes.findingsDialog} />
    </div>
  );
}
