import {
  Alert,
  AlertDescription,
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
} from "@neokapi/ui-primitives";
import { VirtualList, type VirtualListHandle } from "@neokapi/editor-grid";
import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { ErrorNotice } from "../errors";
import type {
  ProjectInfo,
  BlockInfo,
  BlockCounts,
  FileQAResult,
  ReviewDemotion,
} from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useLocales } from "../hooks/useLocales";
import { useAnalytics } from "../context/AnalyticsContext";
import { AnalyticsEvents } from "../analytics-events";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";
import { FormattedSourceDisplay } from "./editor/FormattedSourceDisplay";
import { CollapsedTargetCell } from "./editor/GridTargetRenderer";
import { ProblemsPanel } from "./editor/ProblemsPanel";
import {
  captureTargetStatus,
  getBlockStatus,
  getTargetText,
  rollbackTargetStatus,
  statusBadgeClass,
  statusLabel,
  withTargetStatus,
  type BlockStatus,
  type TargetStatusSnapshot,
} from "./editor/blockStatus";
import { ArrowLeft, Check, X, AlertTriangle } from "./icons";

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

/** The four buckets, all zero — the shape shown before the counts arrive. */
const EMPTY_COUNTS: BlockCounts = {
  total: 0,
  translatable: 0,
  status: { "not-started": 0, draft: 0, translated: 0, reviewed: 0 },
};

/**
 * ReviewSurface is the block-level translation review/QA surface — a sibling
 * route to the Translate editor. The status filter and the histogram beside it
 * are server queries: the list holds the blocks in the selected bucket for the
 * selected locale, and the counts describe the whole file rather than the page.
 * Bulk actions (mark reviewed, apply content memory) are one request each, with
 * per-block outcomes; per-block approve/reject and a QA findings panel (reused
 * ProblemsPanel) complete the surface. (Brand-rule promotion lives in the
 * separate brand-review surface; this is about translation review.)
 */
export function ReviewSurface({
  project,
  fileName,
  onBack,
  presenceSlot,
  surfaceTabs,
}: ReviewSurfaceProps) {
  const [blocks, setBlocks] = useState<BlockInfo[]>([]);
  const [targetLocale, setTargetLocale] = useState(project.target_languages[0] || "");
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [error, setError] = useState<{ title: string; cause?: unknown; retry?: boolean } | null>(
    null,
  );
  const [message, setMessage] = useState<string | null>(null);
  const [fileQAResults, setFileQAResults] = useState<FileQAResult[]>([]);
  const [qaLoading, setQaLoading] = useState(false);
  const [showProblems, setShowProblems] = useState(false);

  const [counts, setCounts] = useState<BlockCounts>(EMPTY_COUNTS);

  const { getDisplayName } = useLocales();
  const api = useEditorApi();
  const { capture } = useAnalytics();
  const { getFileBlocks, getBlockCounts } = api;

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

  // The list is the selected bucket for the selected locale, asked for as
  // such: the filter is a query parameter, not a pass over a full download.
  const loadBlocks = useCallback(async () => {
    try {
      const b = await getFileBlocks(project.id, fileName, {
        locale: targetLocale,
        translatable: true,
        // A bucket partitions one locale's targets, so it travels only with a
        // locale — a project with no target language filters by nothing.
        status: filter === "all" || !targetLocale ? undefined : filter,
      });
      setBlocks(b || []);
    } catch (e) {
      setError({ title: "Couldn't load the blocks", cause: e, retry: true });
    }
  }, [getFileBlocks, project.id, fileName, targetLocale, filter]);

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

  const allVisibleSelected = visible.length > 0 && visible.every((b) => selected.has(b.id));

  // A selection names blocks in the loaded bucket; a different bucket or locale
  // is a different set, so the selection does not survive the switch.
  useEffect(() => {
    setSelected(new Set());
  }, [filter, targetLocale]);

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    setSelected((prev) => {
      if (visible.every((b) => prev.has(b.id))) return new Set();
      return new Set(visible.map((b) => b.id));
    });
  }, [visible]);

  // Persist a per-block review decision: optimistic per-locale Target.Status
  // write (matches what a reload fetches), server call, rollback on failure.
  // The rollback snapshot is captured inside the setBlocks updater — from the
  // state the write actually replaced — and restores only the status field, so
  // a target save that lands while the review call is in flight is preserved.
  // `demoteTo` picks the rung a clearing call (reviewed=false) lands on:
  // "draft" for a reviewer rejection, otherwise translated.
  const setStatus = useCallback(
    async (block: BlockInfo, reviewed: boolean, demoteTo?: ReviewDemotion) => {
      // Clearing the review state of a locale with no translation is a no-op:
      // the server treats it as an idempotent 200 success, so an optimistic
      // write here would fabricate a phantom {text: "", status} entry that no
      // rollback ever removes (approval is guarded by the button and the
      // server's 422).
      if (!reviewed && !getTargetText(block, targetLocale).trim()) return;
      capture(AnalyticsEvents.reviewDecisionClicked, {
        decision: reviewed ? "approve" : demoteTo === "draft" ? "reject" : "clear",
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
            reviewed ? "reviewed" : (demoteTo ?? "translated"),
          );
        }),
      );
      try {
        await api.reviewBlock(project.id, fileName, block.id, targetLocale, reviewed, demoteTo);
        void loadCounts();
      } catch (e) {
        setBlocks((prev) =>
          prev.map((b) =>
            b.id === block.id ? rollbackTargetStatus(b, targetLocale, snapshot) : b,
          ),
        );
        setError({
          title: reviewed
            ? "Couldn't mark the block as reviewed"
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

  // One request carries the whole selection. Each block reports its own
  // outcome, so an approval a block refuses is named rather than sinking the
  // batch, and only the blocks that took the decision move.
  const bulkMarkReviewed = useCallback(async () => {
    if (bulkInFlight.current || selected.size === 0) return;
    bulkInFlight.current = true;
    setBulkBusy(true);
    try {
      // Skip untranslated blocks: the server categorically refuses an approval
      // of an empty translation, so sending them in would only fill the result
      // with guaranteed failures (select-all on filter=all includes them).
      const targetIds = blocks
        .filter(
          (b) => selected.has(b.id) && b.translatable && getTargetText(b, targetLocale).trim(),
        )
        .map((b) => b.id);
      setSelected(new Set());
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
  }, [selected, blocks, api, capture, project.id, fileName, targetLocale, loadCounts]);

  // Exact content-memory leverage across the selection: one request, and the
  // server decides which blocks clear the threshold.
  const bulkApplyMemory = useCallback(async () => {
    if (bulkInFlight.current || selected.size === 0) return;
    bulkInFlight.current = true;
    setBulkBusy(true);
    try {
      const targetIds = blocks.filter((b) => selected.has(b.id) && b.translatable).map((b) => b.id);
      setSelected(new Set());
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
  }, [selected, blocks, api, project.id, targetLocale, loadBlocks, loadCounts]);

  const runQA = useCallback(() => {
    setQaLoading(true);
    setShowProblems(true);
    api
      .runFileQACheck(project.id, fileName, targetLocale)
      .then((r) => setFileQAResults(r || []))
      .catch(() => setFileQAResults([]))
      .finally(() => setQaLoading(false));
  }, [api, project.id, fileName, targetLocale]);

  const qaIssueCount = useMemo(
    () => fileQAResults.reduce((acc, r) => acc + r.issues.length, 0),
    [fileQAResults],
  );

  // Map QA issues to blocks for inline detail.
  const qaByBlock = useMemo(() => {
    const m = new Map<string, FileQAResult>();
    for (const r of fileQAResults) m.set(r.blockId, r);
    return m;
  }, [fileQAResults]);

  // Virtualize the block list via the shared @neokapi/editor-grid VirtualList:
  // large files can hold thousands of blocks, so only the rows near the viewport
  // are mounted. Rows measure themselves (source/target text and QA findings
  // vary in height). The handle drives the QA "jump to block" scroll below.
  const gridRef = useRef<VirtualListHandle>(null);

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
          onClick={runQA}
          data-testid="run-qa-btn"
        >
          <AlertTriangle className="w-3.5 h-3.5 mr-1" />
          Run QA
          {qaIssueCount > 0 && (
            <span className="ml-1 text-[10px] px-1 rounded-full bg-destructive/15 text-destructive font-bold">
              {qaIssueCount}
            </span>
          )}
        </Button>
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

      {/* Status filters */}
      <div className="flex items-center gap-1.5 mb-2 flex-wrap" data-testid="status-filters">
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
              {f === "all" ? "All" : statusLabel[f]} ({count})
            </button>
          );
        })}
      </div>

      {/* Bulk action bar */}
      <div className="flex items-center gap-2 mb-2">
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground cursor-pointer">
          <input
            type="checkbox"
            checked={allVisibleSelected}
            onChange={toggleSelectAll}
            data-testid="select-all"
          />
          Select all
        </label>
        <span className="text-xs text-muted-foreground">{selected.size} selected</span>
        <div className="flex-1" />
        <Button
          variant="outline"
          size="sm"
          onClick={bulkApplyMemory}
          disabled={selected.size === 0 || bulkBusy}
          data-testid="bulk-apply-tm"
        >
          Apply exact Memory
        </Button>
        <Button
          size="sm"
          onClick={bulkMarkReviewed}
          disabled={selected.size === 0 || bulkBusy}
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

      {/* Block list (virtualized — only rows near the viewport are mounted) */}
      <VirtualList<BlockInfo>
        ref={gridRef}
        items={visible}
        estimateSize={96}
        overscan={12}
        getItemKey={(_, block) => block.id}
        initialRect={{ width: 800, height: 600 }}
        className="flex-1 overflow-auto border border-border rounded-lg bg-card"
        dataTestId="review-list"
        empty={
          <div className="p-6 text-center text-muted-foreground">No blocks for this filter</div>
        }
        renderRow={(block, { key, rowProps }) => {
          const status = getBlockStatus(block, targetLocale);
          const qa = qaByBlock.get(block.id);
          return (
            <div
              key={key}
              {...rowProps}
              data-testid={`review-row-${block.id}`}
              className="flex items-start gap-3 px-3 py-2.5 border-b border-border"
            >
              <input
                type="checkbox"
                checked={selected.has(block.id)}
                onChange={() => toggleSelect(block.id)}
                className="mt-1.5"
                data-testid={`review-select-${block.id}`}
              />
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span
                    className={cn(
                      "px-1.5 py-0.5 rounded text-[10px] font-semibold",
                      statusBadgeClass[status],
                    )}
                    data-testid={`review-status-${block.id}`}
                  >
                    {statusLabel[status]}
                  </span>
                  {qa && qa.issues.length > 0 && (
                    <span className="inline-flex items-center gap-0.5 text-[10px] font-bold text-destructive bg-destructive/10 px-1.5 py-0.5 rounded">
                      <AlertTriangle className="w-2.5 h-2.5" />
                      {qa.issues.length}
                    </span>
                  )}
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="text-sm leading-relaxed break-words text-muted-foreground">
                    {block.has_spans && block.source_coded && block.source_spans ? (
                      <FormattedSourceDisplay
                        codedText={block.source_coded}
                        spans={block.source_spans}
                      />
                    ) : (
                      block.source
                    )}
                  </div>
                  <div className="text-sm leading-relaxed break-words">
                    <CollapsedTargetCell
                      block={block}
                      locale={targetLocale}
                      testId={`review-target-${block.id}`}
                    />
                  </div>
                </div>
                {/* QA findings detail */}
                {qa && qa.issues.length > 0 && (
                  <div className="mt-1.5 rounded-md border border-border bg-muted/30 p-2 space-y-1">
                    {qa.issues.map((issue, i) => (
                      <div key={i} className="flex items-start gap-1.5 text-xs">
                        <AlertTriangle
                          className={cn(
                            "w-3 h-3 shrink-0 mt-0.5",
                            issue.severity === "error" ? "text-destructive" : "text-warning",
                          )}
                        />
                        <span
                          className={
                            issue.severity === "error"
                              ? "text-destructive"
                              : "text-warning dark:text-warning"
                          }
                        >
                          <span className="font-medium">{issue.type}:</span> {issue.message}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              {/* Approve / reject */}
              <div className="flex flex-col gap-1 shrink-0">
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 text-[11px] px-2"
                  onClick={() => void setStatus(block, true)}
                  disabled={
                    status === "reviewed" ||
                    bulkBusy ||
                    // No non-empty translation → the server 422s the approval.
                    !getTargetText(block, targetLocale).trim()
                  }
                  data-testid={`approve-${block.id}`}
                >
                  <Check className="w-3.5 h-3.5 mr-1" /> Approve
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 text-[11px] px-2 text-destructive hover:text-destructive"
                  // A rejection demotes the target to draft so the unit
                  // re-enters the work queue (host's rejected → draft
                  // mapping), not merely back to translated.
                  onClick={() => void setStatus(block, false, "draft")}
                  disabled={
                    bulkBusy ||
                    // Nothing to reject on an untranslated block (and a
                    // clearing call for it is a server-side no-op).
                    !getTargetText(block, targetLocale).trim()
                  }
                  data-testid={`reject-${block.id}`}
                >
                  <X className="w-3.5 h-3.5 mr-1" /> Reject
                </Button>
              </div>
            </div>
          );
        }}
      />

      {/* QA problems panel (reused) */}
      {showProblems && (
        <ProblemsPanel
          issues={fileQAResults}
          loading={qaLoading}
          onNavigateToBlock={(blockId) => {
            // Rows are virtualized, so scroll via the shared list handle (the
            // target row may not be mounted yet).
            const index = visible.findIndex((b) => b.id === blockId);
            if (index >= 0) gridRef.current?.scrollToIndex(index, { align: "center" });
          }}
          onClose={() => setShowProblems(false)}
        />
      )}
    </div>
  );
}
