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
import { useState, useEffect, useCallback, useMemo } from "react";
import type { ProjectInfo, BlockInfo, FileQAResult } from "../types/api";
import { useEditorApi } from "../hooks/useEditorApi";
import { useLocales } from "../hooks/useLocales";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";
import { FormattedSourceDisplay } from "./editor/FormattedSourceDisplay";
import { CollapsedTargetCell } from "./editor/GridTargetRenderer";
import { ProblemsPanel } from "./editor/ProblemsPanel";
import {
  getBlockStatus,
  statusBadgeClass,
  statusLabel,
  type BlockStatus,
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

/**
 * ReviewSurface is the block-level translation review/QA surface — a sibling
 * route to the Translate editor. It lists translatable blocks by status with
 * filtering, bulk actions (mark reviewed, apply TM), per-block approve/reject,
 * and a QA findings panel (reused ProblemsPanel). Mark-reviewed and QA were
 * moved out of the Translate toolbar into here. (Brand-rule promotion lives in
 * the separate brand-review surface; this is about translation review.)
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
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [fileQAResults, setFileQAResults] = useState<FileQAResult[]>([]);
  const [qaLoading, setQaLoading] = useState(false);
  const [showProblems, setShowProblems] = useState(false);

  const { getDisplayName } = useLocales();
  const api = useEditorApi();
  const { getFileBlocks } = api;

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

  const loadBlocks = useCallback(async () => {
    try {
      const b = await getFileBlocks(project.id, fileName);
      setBlocks(b || []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load blocks");
    }
  }, [getFileBlocks, project.id, fileName]);

  useEffect(() => {
    void loadBlocks();
  }, [loadBlocks]);

  const translatable = useMemo(() => blocks.filter((b) => b.translatable), [blocks]);

  const counts = useMemo(() => {
    const c: Record<BlockStatus, number> = {
      "not-started": 0,
      draft: 0,
      translated: 0,
      reviewed: 0,
    };
    for (const b of translatable) c[getBlockStatus(b, targetLocale)]++;
    return c;
  }, [translatable, targetLocale]);

  const visible = useMemo(
    () =>
      filter === "all"
        ? translatable
        : translatable.filter((b) => getBlockStatus(b, targetLocale) === filter),
    [translatable, filter, targetLocale],
  );

  const allVisibleSelected = visible.length > 0 && visible.every((b) => selected.has(b.id));

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

  const setStatus = useCallback((block: BlockInfo, status: "reviewed" | "draft") => {
    setBlocks((prev) =>
      prev.map((b) =>
        b.id === block.id
          ? { ...b, properties: { ...b.properties, "translation-status": status } }
          : b,
      ),
    );
  }, []);

  const bulkMarkReviewed = useCallback(() => {
    if (selected.size === 0) return;
    setBlocks((prev) =>
      prev.map((b) =>
        selected.has(b.id)
          ? { ...b, properties: { ...b.properties, "translation-status": "reviewed" } }
          : b,
      ),
    );
    setMessage(`Marked ${selected.size} block(s) as reviewed`);
    setSelected(new Set());
  }, [selected]);

  const bulkApplyTM = useCallback(async () => {
    if (selected.size === 0) return;
    let applied = 0;
    for (const block of blocks) {
      if (!selected.has(block.id) || !block.translatable) continue;
      try {
        const matches = await api.lookupTMForBlock(project.id, fileName, block.id, targetLocale);
        const best = matches?.[0];
        if (best && best.score >= 1) {
          await api.updateBlockTarget({
            project_id: project.id,
            item_name: fileName,
            block_id: block.id,
            target_locale: targetLocale,
            text: best.target,
          });
          applied++;
        }
      } catch {
        // skip individual failures; continue the batch
      }
    }
    setMessage(`Applied ${applied} exact TM match(es)`);
    setSelected(new Set());
    await loadBlocks();
  }, [selected, blocks, api, project.id, fileName, targetLocale, loadBlocks]);

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
          const count = f === "all" ? translatable.length : counts[f];
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
          onClick={bulkApplyTM}
          disabled={selected.size === 0}
          data-testid="bulk-apply-tm"
        >
          Apply exact TM
        </Button>
        <Button
          size="sm"
          onClick={bulkMarkReviewed}
          disabled={selected.size === 0}
          data-testid="bulk-mark-reviewed"
        >
          <Check className="w-3.5 h-3.5 mr-1" /> Mark reviewed
        </Button>
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

      {/* Block list */}
      <div
        className="flex-1 overflow-auto border border-border rounded-lg bg-card"
        data-testid="review-list"
      >
        {visible.map((block) => {
          const status = getBlockStatus(block, targetLocale);
          const qa = qaByBlock.get(block.id);
          return (
            <div
              key={block.id}
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
                  onClick={() => setStatus(block, "reviewed")}
                  disabled={status === "reviewed"}
                  data-testid={`approve-${block.id}`}
                >
                  <Check className="w-3.5 h-3.5 mr-1" /> Approve
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 text-[11px] px-2 text-destructive hover:text-destructive"
                  onClick={() => setStatus(block, "draft")}
                  data-testid={`reject-${block.id}`}
                >
                  <X className="w-3.5 h-3.5 mr-1" /> Reject
                </Button>
              </div>
            </div>
          );
        })}
        {visible.length === 0 && (
          <div className="p-6 text-center text-muted-foreground">No blocks for this filter</div>
        )}
      </div>

      {/* QA problems panel (reused) */}
      {showProblems && (
        <ProblemsPanel
          issues={fileQAResults}
          loading={qaLoading}
          onNavigateToBlock={(blockId) => {
            const el = document.querySelector(`[data-testid="review-row-${blockId}"]`);
            el?.scrollIntoView({ block: "center", behavior: "smooth" });
          }}
          onClose={() => setShowProblems(false)}
        />
      )}
    </div>
  );
}
