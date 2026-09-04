import {
  Alert,
  AlertDescription,
  Button,
  LocaleLabel,
  cn,
  statusMeta,
} from "@neokapi/ui-primitives";
import { FormatPreview, extOf } from "@neokapi/ui-primitives/preview";
import type { BlockAttrs } from "@neokapi/ui-primitives/preview";
import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { ErrorNotice } from "../errors";
import type {
  ProjectInfo,
  AppliedMemory,
  BlockInfo,
  BlockCounts,
  BlockTermMatch,
  FileCheckResult,
  ReviewContext,
  ReviewRung,
} from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useLocales } from "../hooks/useLocales";
import { useCallerPermissions } from "../hooks/useCallerPermissions";
import { useAnalytics } from "../context/AnalyticsContext";
import { AnalyticsEvents } from "../analytics-events";
import { ProblemsPanel } from "./editor/ProblemsPanel";
import { ApplyMemoryDialog } from "./review/ApplyMemoryDialog";
import { LanguageScopeSelect } from "./review/LanguageScopeSelect";
import { ReviewInspector } from "./review/ReviewInspector";
import { blocksToContentTree, type BlockEvidence } from "../preview/toContentTree";
import type { UnifiedSaveResult } from "./UnifiedTargetEditor";
import {
  captureTargetStatus,
  getBlockStatus,
  getTargetText,
  rollbackTargetStatus,
  statusAfterEdit,
  statusRuleClass,
  withTargetEntry,
  withTargetStatus,
  type BlockStatus,
  type TargetStatusSnapshot,
} from "./editor/blockStatus";
import { Check, AlertTriangle } from "./icons";

interface ReviewSurfaceProps {
  project: ProjectInfo;
  fileName: string;
  onBack: () => void;
  /** Optional presence slot rendered in the toolbar. */
  presenceSlot?: React.ReactNode;
  /** Optional slot for the cross-surface switcher (Pre-process/Translate/Review). */
  surfaceTabs?: React.ReactNode;
}

type StatusFilter = "all" | BlockStatus;

const FILTERS: StatusFilter[] = ["all", "not-started", "draft", "translated", "reviewed"];

/** How many blocks one page of the reading pane carries. */
const PAGE_SIZE = 50;

/** The four buckets, all zero — the shape shown before the counts arrive. */
const EMPTY_COUNTS: BlockCounts = {
  total: 0,
  translatable: 0,
  status: { "not-started": 0, draft: 0, translated: 0, reviewed: 0 },
};

/** Whether a keystroke belongs to a text field rather than the surface. */
function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  return ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName);
}

/**
 * ReviewSurface is the block-level review surface — a sibling route to the
 * Translate editor, and document-first: the item is rendered as the document it
 * is, through the shared preview kit, with term, entity and voice findings
 * marked inline where they occur. A block is chosen by reading, not by scanning
 * a source/target grid; its particulars and the decision itself arrive in a
 * slide-in inspector beside the text.
 *
 * What the document cannot state, it does not fake. The status filter and the
 * histogram beside it are server queries over the whole file (the pane holds one
 * page of the selected bucket, and says so). Findings the payload gives no
 * position for tint their block and are listed in the inspector rather than
 * being guessed onto a span. Terms are a per-block lookup, so they are asked for
 * when a block is opened — one request, not one per block of the document.
 *
 * Bulk actions carry a batch the reader marks up as they read (M, or the
 * inspector's button); each is one request with per-block outcomes.
 * (Brand-rule promotion lives in the separate brand-review surface; this is
 * about translation review.)
 */
export function ReviewSurface({
  project,
  fileName,
  onBack: _onBack,
  presenceSlot,
  surfaceTabs,
}: ReviewSurfaceProps) {
  const sourceLocale = project.default_source_language;
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [targetLocale, setTargetLocale] = useState(project.target_languages[0] || "");
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [marked, setMarked] = useState<Set<string>>(new Set());
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [limit, setLimit] = useState(PAGE_SIZE);
  const [error, setError] = useState<{ title: string; cause?: unknown; retry?: boolean } | null>(
    null,
  );
  const [message, setMessage] = useState<string | null>(null);
  const [fileCheckResults, setFileCheckResults] = useState<FileCheckResult[]>([]);
  const [checksLoading, setChecksLoading] = useState(false);
  const [showProblems, setShowProblems] = useState(false);
  const [termsByBlock, setTermsByBlock] = useState<Record<string, BlockTermMatch[]>>({});
  const [termsLoading, setTermsLoading] = useState(false);
  const [contextByBlock, setContextByBlock] = useState<Record<string, ReviewContext>>({});
  // What a bulk content-memory pass would write, as the pass itself answers.
  // Null closes the confirm step; an empty list is a real answer (the batch
  // matches nothing) and says so.
  const [memoryPreview, setMemoryPreview] = useState<AppliedMemory[] | null>(null);

  const [counts, setCounts] = useState<BlockCounts>(EMPTY_COUNTS);

  const { getDisplayName } = useLocales();
  const api = useEditorApi();
  const { capture } = useAnalytics();
  const { getFileBlocks, getBlockCounts } = api;
  // Approving is the `review` permission, per language, so a translator gets a
  // disabled button rather than a 403 on click.
  const { can } = useCallerPermissions(project.id);
  const canApprove = can("review", targetLocale || undefined);

  // The language the document is read in. One selector holds every language of
  // the project, the source among them, so choosing English reads the source
  // and choosing French reads the French translation. A lane toggle beside a
  // target dropdown asked the same question twice.
  //
  // Picking the source leaves `targetLocale` where it was: the buckets, the
  // batch and the decisions all judge one translation, and reading the source
  // is a way of judging it rather than a different subject.
  const [language, setLanguage] = useState(project.target_languages[0] || sourceLocale);
  const readingSource = language === sourceLocale || !targetLocale;
  const readingSide = readingSource ? "source" : targetLocale;
  const changeLanguage = useCallback(
    (next: string) => {
      setLanguage(next);
      if (next !== sourceLocale) setTargetLocale(next);
    },
    [sourceLocale],
  );
  const languageOptions = useMemo(
    () => [
      { locale: sourceLocale, source: true },
      ...project.target_languages.filter((l) => l !== sourceLocale).map((locale) => ({ locale })),
    ],
    [sourceLocale, project.target_languages],
  );

  // The pane is the selected bucket for the selected locale, asked for as such:
  // the filter is a query parameter, not a pass over a full download, and the
  // page size is explicit so the footer can say what is being shown.
  const loadBlocks = useCallback(async () => {
    try {
      const b = await getFileBlocks(project.id, fileName, {
        locale: targetLocale,
        translatable: true,
        // A bucket partitions one locale's targets, so it travels only with a
        // locale — a project with no target language filters by nothing.
        status: filter === "all" || !targetLocale ? undefined : filter,
        limit,
      });
      setBlocks(b || []);
    } catch (e) {
      setError({ title: "Couldn't load the blocks", cause: e, retry: true });
    }
  }, [getFileBlocks, project.id, fileName, targetLocale, filter, limit]);

  // The histogram describes the file, so it is its own count query — a filtered
  // page could only report itself.
  const loadCounts = useCallback(async () => {
    try {
      setCounts(await getBlockCounts(project.id, fileName, targetLocale));
    } catch {
      setCounts(EMPTY_COUNTS);
    }
  }, [getBlockCounts, project.id, fileName, targetLocale]);

  useEffect(() => {
    void loadBlocks();
  }, [loadBlocks]);

  useEffect(() => {
    void loadCounts();
  }, [loadCounts]);

  const visible = blocks;
  const selectedBlock = useMemo(
    () => visible.find((b) => b.id === selectedId) ?? null,
    [visible, selectedId],
  );

  // A batch names blocks in the loaded bucket; a different bucket or locale is a
  // different set, so neither the batch nor the open block survives the switch.
  useEffect(() => {
    setMarked(new Set());
    setSelectedId(null);
    setEditing(false);
    setLimit(PAGE_SIZE);
  }, [filter, targetLocale]);

  const toggleMark = useCallback((id: string) => {
    setMarked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const markAllShown = useCallback(() => {
    setMarked((prev) => {
      if (visible.every((b) => prev.has(b.id))) return new Set();
      return new Set(visible.map((b) => b.id));
    });
  }, [visible]);

  // What the surface has already asked for, keyed by (block, locale). It is a
  // ref rather than a dependency because the answers land in state: an effect
  // that lists its own cache as a dependency re-runs the moment the answer
  // arrives, and its cleanup then cancels the very request that answered it.
  // That is how the inspector came to sit on "Looking up terms…" over a block
  // whose terms the server had already returned.
  const asked = useRef(new Set<string>());
  const blockKey = selectedId && targetLocale ? `${selectedId}::${targetLocale}` : "";

  // Terms are looked up for the block being read, once, and cached for the
  // session: the projection then shows them inline on that block as well as in
  // the inspector. There is no file-level term query, and a request per block of
  // the document would be one too many.
  useEffect(() => {
    if (!selectedId || !targetLocale || asked.current.has(`terms:${blockKey}`)) return;
    asked.current.add(`terms:${blockKey}`);
    setTermsLoading(true);
    api
      .lookupTermsForBlock(project.id, fileName, selectedId, targetLocale)
      .then((t) => setTermsByBlock((prev) => ({ ...prev, [selectedId]: t ?? [] })))
      .catch(() => setTermsByBlock((prev) => ({ ...prev, [selectedId]: [] })))
      .finally(() => setTermsLoading(false));
  }, [api, selectedId, blockKey, project.id, fileName, targetLocale]);

  // The rest of what governs and precedes the open block: the content-memory
  // match with its wording, the last decision and its note, the target's
  // origin, and the findings behind its voice score. Asked for once per block
  // opened, like the term lookup beside it, and cached for the session. Keyed
  // by block AND locale, because the memory match, the decision and the origin
  // are all per-locale — a locale switch asks again rather than showing the
  // previous language's answer.
  useEffect(() => {
    if (!selectedId || !targetLocale || asked.current.has(`context:${blockKey}`)) return;
    asked.current.add(`context:${blockKey}`);
    api
      .getReviewContext(project.id, fileName, selectedId, targetLocale)
      .then((ctx) => setContextByBlock((prev) => ({ ...prev, [blockKey]: ctx })))
      .catch(() => {
        // Non-fatal: the inspector draws its empty states and the decision
        // buttons keep working. The key stays marked as asked, so a failure is
        // not retried on every render.
      });
  }, [api, selectedId, blockKey, project.id, fileName, targetLocale]);

  // Persist a per-block review decision: optimistic per-locale Target.Status
  // write (matches what a reload fetches), server call, rollback on failure.
  // The rollback snapshot is captured inside the setBlocks updater — from the
  // state the write actually replaced — and restores only the status field, so
  // a target save that lands while the review call is in flight is preserved.
  // `rung` picks where the call lands within its direction: "signed-off" on an
  // approval, "draft" on a clearing call for a reviewer rejection, and either
  // default at reviewed or translated.
  const setStatus = useCallback(
    async (block: BlockInfo, reviewed: boolean, rung?: ReviewRung) => {
      // Clearing the review state of a locale with no translation is a no-op:
      // the server treats it as an idempotent 200 success, so an optimistic
      // write here would fabricate a phantom {text: "", status} entry that no
      // rollback ever removes (approval is guarded by the button and the
      // server's 422).
      if (!reviewed && !getTargetText(block, targetLocale).trim()) return;
      capture(AnalyticsEvents.reviewDecisionClicked, {
        decision: reviewed
          ? rung === "signed-off"
            ? "sign_off"
            : "approve"
          : rung === "draft"
            ? "reject"
            : "clear",
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
            reviewed ? (rung === "signed-off" ? "signed-off" : "reviewed") : (rung ?? "translated"),
          );
        }),
      );
      try {
        await api.reviewBlock(project.id, fileName, block.id, targetLocale, reviewed, rung);
        void loadCounts();
      } catch (e) {
        setBlocks((prev) =>
          prev.map((b) =>
            b.id === block.id ? rollbackTargetStatus(b, targetLocale, snapshot) : b,
          ),
        );
        setError({
          title: reviewed
            ? rung === "signed-off"
              ? "Couldn't sign the block off"
              : "Couldn't mark the block as reviewed"
            : "Couldn't update the review status",
          cause: e,
        });
      }
    },
    [api, capture, project.id, fileName, targetLocale, loadCounts],
  );

  // While a bulk request is in flight, single approve/reject clicks and a
  // second bulk click are disabled: an interleaved single call would race the
  // batch's writes. The ref guards re-entrancy synchronously (state updates are
  // async); the state drives the disabled buttons.
  const bulkInFlight = useRef(false);
  const [bulkBusy, setBulkBusy] = useState(false);

  // One request carries the whole batch. Each block reports its own outcome, so
  // an approval a block refuses is named rather than sinking the batch, and only
  // the blocks that took the decision move.
  const bulkMarkReviewed = useCallback(async () => {
    if (bulkInFlight.current || marked.size === 0) return;
    bulkInFlight.current = true;
    setBulkBusy(true);
    try {
      // Skip untranslated blocks: the server categorically refuses an approval
      // of an empty translation, so sending them in would only fill the result
      // with guaranteed failures (marking all on filter=all includes them).
      const targetIds = blocks
        .filter((b) => marked.has(b.id) && b.translatable && getTargetText(b, targetLocale).trim())
        .map((b) => b.id);
      setMarked(new Set());
      if (targetIds.length === 0) return;
      capture(AnalyticsEvents.reviewDecisionClicked, {
        decision: "approve",
        locale: targetLocale,
        bulk: true,
      });
      const result = await api.bulkReviewBlocks({
        project_id: project.id,
        item_name: fileName,
        block_ids: targetIds,
        target_locale: targetLocale,
        approve: true,
      });
      const approved = new Set((result.results ?? []).filter((r) => r.ok).map((r) => r.block_id));
      setBlocks((prev) =>
        prev.map((b) => (approved.has(b.id) ? withTargetStatus(b, targetLocale, "reviewed") : b)),
      );
      if (result.succeeded > 0) setMessage(`Marked ${result.succeeded} block(s) as reviewed`);
      if (result.failed > 0) {
        const firstError = (result.results ?? []).find((r) => !r.ok)?.error;
        setError({
          title: `Couldn't mark ${result.failed} block(s) as reviewed`,
          cause: firstError ? new Error(firstError) : undefined,
        });
      }
      void loadCounts();
    } catch (e) {
      setError({ title: "Couldn't mark the selected blocks as reviewed", cause: e });
    } finally {
      bulkInFlight.current = false;
      setBulkBusy(false);
    }
  }, [marked, blocks, api, capture, project.id, fileName, targetLocale, loadCounts]);

  /** The blocks a bulk pass would carry: marked, and translatable. */
  const memoryBatchIds = useCallback(
    () => blocks.filter((b) => marked.has(b.id) && b.translatable).map((b) => b.id),
    [blocks, marked],
  );

  // What the pass would write, asked of the pass itself. The reviewer reads the
  // wording before it lands: a match the corpus holds for a different context
  // otherwise arrives in the document with no step at which anyone could notice.
  const previewApplyMemory = useCallback(async () => {
    if (bulkInFlight.current || marked.size === 0) return;
    const targetIds = memoryBatchIds();
    if (targetIds.length === 0) return;
    setBulkBusy(true);
    try {
      const result = await api.bulkApplyMemory({
        project_id: project.id,
        block_ids: targetIds,
        target_locale: targetLocale,
        preview: true,
      });
      setMemoryPreview(result.applied ?? []);
    } catch (e) {
      setError({ title: "Couldn't read what the content memory would apply", cause: e });
    } finally {
      setBulkBusy(false);
    }
  }, [marked, memoryBatchIds, api, project.id, targetLocale]);

  // Exact content-memory leverage across the batch: one request, and the server
  // decides which blocks clear the threshold.
  const bulkApplyMemory = useCallback(async () => {
    if (bulkInFlight.current || marked.size === 0) return;
    bulkInFlight.current = true;
    setBulkBusy(true);
    setMemoryPreview(null);
    try {
      const targetIds = memoryBatchIds();
      setMarked(new Set());
      if (targetIds.length === 0) return;
      const result = await api.bulkApplyMemory({
        project_id: project.id,
        block_ids: targetIds,
        target_locale: targetLocale,
      });
      const applied = result.applied ?? [];
      setMessage(`Applied ${applied.length} exact content-memory match(es)`);
      if (applied.length > 0) {
        // The blocks now carry text, so their bucket changed: re-ask for the
        // page and the histogram rather than guessing where they landed.
        await loadBlocks();
        void loadCounts();
      }
    } catch (e) {
      setError({ title: "Couldn't apply the content memory", cause: e });
    } finally {
      bulkInFlight.current = false;
      setBulkBusy(false);
    }
  }, [marked, memoryBatchIds, api, project.id, targetLocale, loadBlocks, loadCounts]);

  // A correction made while reviewing is the same write the Translate editor
  // makes: coded text plus spans, with the status the server would derive.
  const saveTarget = useCallback(
    async (block: BlockInfo, result: UnifiedSaveResult) => {
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
          // Inline-code placeholders are private-use characters; the plain text a
          // reload would fetch is the coded text without them.
          const plainText = result.codedText.replace(/[\uE001-\uE003]/g, "");
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === block.id
                ? {
                    ...withTargetEntry(b, targetLocale, {
                      text: plainText,
                      status: statusAfterEdit(b, targetLocale, plainText, result.codedText),
                    }),
                    targets_coded: { ...b.targets_coded, [targetLocale]: result.codedText },
                  }
                : b,
            ),
          );
        } else {
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
                ? withTargetEntry(b, targetLocale, {
                    text: result.text,
                    status: statusAfterEdit(b, targetLocale, result.text),
                  })
                : b,
            ),
          );
        }
        capture(AnalyticsEvents.translationSaved, { locale: targetLocale, method: "editor" });
        setEditing(false);
        void loadCounts();
      } catch (e) {
        setError({ title: "Couldn't save the translation", cause: e });
      }
    },
    [api, capture, project.id, fileName, targetLocale, loadCounts],
  );

  const runChecks = useCallback(() => {
    setChecksLoading(true);
    setShowProblems(true);
    api
      .runFileCheck(project.id, fileName, targetLocale)
      .then((r) => setFileCheckResults(r || []))
      .catch(() => setFileCheckResults([]))
      .finally(() => setChecksLoading(false));
  }, [api, project.id, fileName, targetLocale]);

  const checkIssueCount = useMemo(
    () => fileCheckResults.reduce((acc, r) => acc + r.issues.length, 0),
    [fileCheckResults],
  );

  const checksByBlock = useMemo(() => {
    const m = new Map<string, FileCheckResult>();
    for (const r of fileCheckResults) m.set(r.blockId, r);
    return m;
  }, [fileCheckResults]);

  // ── The document ──────────────────────────────────────────────────────────

  // Evidence for the projection: a finding that carries a position becomes a
  // span over the text, and one that does not becomes a block annotation, since
  // a position the payload never reported must not be guessed at. Term hits
  // carry their own offsets and become inline marks on the blocks that have
  // been opened. Entities ride along on the blocks themselves.
  const evidence = useMemo(() => {
    const out: Record<string, BlockEvidence> = {};
    for (const r of fileCheckResults) {
      out[r.blockId] = { issues: r.issues, issueLocale: targetLocale };
    }
    for (const [id, terms] of Object.entries(termsByBlock)) {
      if (terms.length > 0) out[id] = { ...out[id], terms };
    }
    // The findings behind an opened block's voice score. They carry their own
    // run anchor, so they mark the target where the scoring pass raised them
    // rather than sitting in a panel beside the text.
    for (const [key, ctx] of Object.entries(contextByBlock)) {
      if (!key.endsWith(`::${targetLocale}`) || ctx.voice_findings.length === 0) continue;
      out[ctx.block_id] = {
        ...out[ctx.block_id],
        findings: ctx.voice_findings.map((finding) => ({ ...finding, side: targetLocale })),
      };
    }
    return out;
  }, [fileCheckResults, termsByBlock, contextByBlock, targetLocale]);

  const tree = useMemo(
    () =>
      blocksToContentTree(visible, {
        format: extOf(fileName),
        name: fileName,
        evidence,
        locales: targetLocale ? [targetLocale] : undefined,
        sourceLocale: project.default_source_language,
      }),
    [visible, fileName, evidence, targetLocale, project.default_source_language],
  );

  const blockAttrs = useCallback(
    (id: string): BlockAttrs => {
      const block = visible.find((b) => b.id === id);
      const status = block ? getBlockStatus(block, targetLocale) : "not-started";
      const flagged = (checksByBlock.get(id)?.issues.length ?? 0) > 0;
      return {
        className: cn(
          statusRuleClass(status),
          flagged && "bg-warning/5",
          marked.has(id) && "outline outline-1 outline-dashed outline-primary/50",
        ),
        "data-testid": `review-block-${id}`,
        "data-status": status,
        "data-marked": marked.has(id) ? "true" : undefined,
        "data-flagged": flagged ? "true" : undefined,
      };
    },
    [visible, targetLocale, checksByBlock, marked],
  );

  const docRef = useRef<HTMLDivElement>(null);

  const revealBlock = useCallback((id: string) => {
    docRef.current
      ?.querySelector(`[data-block-id="${CSS.escape(id)}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, []);

  const selectBlock = useCallback(
    (id: string) => {
      setSelectedId(id);
      setEditing(false);
      revealBlock(id);
    },
    [revealBlock],
  );

  const step = useCallback(
    (delta: number) => {
      if (visible.length === 0) return;
      const current = visible.findIndex((b) => b.id === selectedId);
      const next = Math.min(Math.max(current + delta, 0), visible.length - 1);
      selectBlock(visible[current < 0 ? 0 : next].id);
    },
    [visible, selectedId, selectBlock],
  );

  // The reading pane's keyboard model: move through the document, decide on the
  // block being read (a approve, s sign off, r reject), open it for correction,
  // hold it for the batch. Held while a text field has focus, so typing a
  // translation is never a decision.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTypingTarget(e.target)) return;
      const block = visible.find((b) => b.id === selectedId) ?? null;
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          step(1);
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          step(-1);
          break;
        case "a":
          if (block && !bulkBusy && getTargetText(block, targetLocale).trim()) {
            e.preventDefault();
            void setStatus(block, true);
          }
          break;
        case "s":
          if (block && !bulkBusy && getTargetText(block, targetLocale).trim()) {
            e.preventDefault();
            void setStatus(block, true, "signed-off");
          }
          break;
        case "r":
          if (block && !bulkBusy && getTargetText(block, targetLocale).trim()) {
            e.preventDefault();
            void setStatus(block, false, "draft");
          }
          break;
        case "e":
          if (block) {
            e.preventDefault();
            setEditing((prev) => !prev);
          }
          break;
        case "m":
          if (block) {
            e.preventDefault();
            toggleMark(block.id);
          }
          break;
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [visible, selectedId, step, setStatus, toggleMark, bulkBusy, targetLocale]);

  const shownCount = visible.length;
  const bucketTotal = filter === "all" ? counts.translatable : counts.status[filter];
  const hasMore = shownCount >= limit && bucketTotal > shownCount;

  const selectedNode = useMemo(() => {
    if (!selectedId) return null;
    const walk = (nodes: typeof tree.root): (typeof tree.root)[number] | null => {
      for (const node of nodes) {
        if (node.kind === "block" && node.id === selectedId) return node;
        const found = node.children ? walk(node.children) : null;
        if (found) return found;
      }
      return null;
    };
    return walk(tree.root);
  }, [tree, selectedId]);

  return (
    <div className="flex flex-col flex-1 min-h-0" data-testid="review-surface">
      {/* Header */}
      <div className="flex items-center gap-3 mb-3">
        {surfaceTabs}
        <span className="text-base font-semibold flex-1 truncate">Review · {fileName}</span>
        {presenceSlot}
        <Button
          variant={showProblems ? "default" : "outline"}
          size="sm"
          onClick={runChecks}
          data-testid="run-check-btn"
        >
          <AlertTriangle className="w-3.5 h-3.5 mr-1" />
          Run checks
          {checkIssueCount > 0 && (
            <span className="ml-1 text-[10px] px-1 rounded-full bg-destructive/15 text-destructive font-bold">
              {checkIssueCount}
            </span>
          )}
        </Button>
        <LanguageScopeSelect
          value={language}
          options={languageOptions}
          onChange={changeLanguage}
          label="Language to review"
          data-testid="language-scope"
        />
      </div>

      {/* Status filters. They partition one translation, so they say which. */}
      <div className="flex items-center gap-1.5 mb-2 flex-wrap" data-testid="status-filters">
        {targetLocale && (
          <LocaleLabel
            locale={targetLocale}
            displayName={getDisplayName(targetLocale)}
            className="mr-1 text-xs text-muted-foreground"
            data-testid="filtered-language"
          />
        )}
        {FILTERS.map((f) => {
          const count = f === "all" ? counts.translatable : counts.status[f];
          return (
            <button
              key={f}
              type="button"
              onClick={() => setFilter(f)}
              data-testid={`filter-${f}`}
              className={cn(
                "px-2.5 py-1 rounded-md text-xs border transition-colors",
                filter === f
                  ? "bg-primary text-primary-foreground border-primary font-semibold"
                  : "bg-card text-muted-foreground border-border hover:text-foreground",
              )}
            >
              {f === "all" ? "All" : statusMeta("content", f).label} ({count})
            </button>
          );
        })}
      </div>

      {/* Batch actions */}
      <div className="flex items-center gap-2 mb-2 flex-wrap">
        <Button
          variant="ghost"
          size="sm"
          onClick={markAllShown}
          data-testid="mark-all-shown"
          className="text-xs"
        >
          Mark all shown
        </Button>
        <span className="text-xs text-muted-foreground" data-testid="marked-count">
          {marked.size} in batch
        </span>
        <div className="flex-1" />
        <Button
          variant="outline"
          size="sm"
          onClick={previewApplyMemory}
          disabled={marked.size === 0 || bulkBusy}
          data-testid="bulk-apply-tm"
        >
          Apply exact Memory
        </Button>
        <Button
          size="sm"
          variant="success"
          onClick={bulkMarkReviewed}
          disabled={marked.size === 0 || bulkBusy || !canApprove}
          title={canApprove ? undefined : "Approving needs the review permission for this language"}
          data-testid="bulk-mark-reviewed"
        >
          <Check className="w-3.5 h-3.5 mr-1" /> Mark reviewed
        </Button>
      </div>

      {/* Messages */}
      {error != null && (
        <ErrorNotice
          error={error.cause}
          title={error.title}
          variant="inline"
          className="mb-2"
          onRetry={
            error.retry
              ? () => {
                  setError(null);
                  void loadBlocks();
                }
              : undefined
          }
        />
      )}
      {message && (
        <Alert className="mb-2 border-success/25 text-success dark:border-success/40 dark:text-success">
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}

      {/* The document. Terms, entities and positioned voice findings are marked
          in the text; the block's status is the rule in its margin. */}
      {/* The pane is the page: the kit paints the document surface itself, so
          the preview fills the frame rather than sitting on a card inside it. */}
      <div
        ref={docRef}
        className="flex-1 overflow-auto rounded-lg border border-border"
        data-testid="review-document"
      >
        {visible.length === 0 ? (
          <p className="p-6 text-center text-muted-foreground">No blocks for this filter</p>
        ) : (
          <FormatPreview
            className="min-h-full px-7 py-6"
            tree={tree}
            side={readingSide}
            onSelectBlock={selectBlock}
            selectedBlockId={selectedId ?? undefined}
            blockAttrs={blockAttrs}
            // The kit's mark animations belong to a demo, not to a working
            // surface: a decision or a term lookup re-renders the pane, and a
            // term that re-scrambles itself each time is noise over the text.
            reducedMotion
          />
        )}
      </div>

      {/* Footer: what the pane holds of the file, and how to drive it */}
      <div className="mt-2 flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
        <span data-testid="page-summary">
          Showing {shownCount} of {bucketTotal}
        </span>
        {hasMore && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-2 text-[11px]"
            onClick={() => setLimit((l) => l + PAGE_SIZE)}
            data-testid="load-more"
          >
            Load more
          </Button>
        )}
        <div className="flex-1" />
        <span className="hidden items-center gap-3 sm:flex" aria-hidden>
          <span>
            <kbd className="rounded border border-border px-1">J</kbd>/
            <kbd className="rounded border border-border px-1">K</kbd> move
          </span>
          <span>
            <kbd className="rounded border border-border px-1">A</kbd> approve
          </span>
          <span>
            <kbd className="rounded border border-border px-1">R</kbd> reject
          </span>
          <span>
            <kbd className="rounded border border-border px-1">E</kbd> edit
          </span>
          <span>
            <kbd className="rounded border border-border px-1">M</kbd> batch
          </span>
        </span>
      </div>

      <ReviewInspector
        block={selectedBlock}
        node={selectedNode}
        itemName={fileName}
        locale={targetLocale}
        localeDisplayName={targetLocale ? getDisplayName(targetLocale) : undefined}
        issues={selectedId ? (checksByBlock.get(selectedId)?.issues ?? []) : []}
        terms={selectedId ? (termsByBlock[selectedId] ?? []) : []}
        termsLoading={termsLoading}
        context={selectedId ? (contextByBlock[`${selectedId}::${targetLocale}`] ?? null) : null}
        editing={editing}
        busy={bulkBusy}
        canApprove={canApprove}
        marked={selectedId ? marked.has(selectedId) : false}
        onClose={() => {
          setSelectedId(null);
          setEditing(false);
        }}
        onApprove={() => selectedBlock && void setStatus(selectedBlock, true)}
        // Signing off promotes the target to the rung above reviewed, the one
        // the ship gates keyed on "at least signed-off" coverage read.
        onSignOff={() => selectedBlock && void setStatus(selectedBlock, true, "signed-off")}
        // A rejection demotes the target to draft so the unit re-enters the work
        // queue (host's rejected → draft mapping), not merely back to translated.
        onReject={() => selectedBlock && void setStatus(selectedBlock, false, "draft")}
        onEditToggle={() => setEditing((prev) => !prev)}
        onSaveEdit={(result) => {
          if (selectedBlock) void saveTarget(selectedBlock, result);
        }}
        onCancelEdit={() => setEditing(false)}
        onToggleMark={() => selectedId && toggleMark(selectedId)}
      />

      {/* What a bulk content-memory pass would write, before it writes it. */}
      <ApplyMemoryDialog
        preview={memoryPreview}
        blocks={blocks}
        locale={targetLocale}
        busy={bulkBusy}
        onCancel={() => setMemoryPreview(null)}
        onConfirm={() => void bulkApplyMemory()}
      />

      {/* Problems panel (reused) */}
      {showProblems && (
        <ProblemsPanel
          issues={fileCheckResults}
          loading={checksLoading}
          onNavigateToBlock={selectBlock}
          onClose={() => setShowProblems(false)}
        />
      )}
    </div>
  );
}
