import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button, Alert, AlertDescription, cn } from "@neokapi/ui-primitives";
import { ConfirmDialog } from "../ConfirmDialog";
import type {
  ProjectInfo,
  TranslationDashboardStats,
  BlockInfo,
  QAIssue,
  LocaleTranslationStats,
} from "../../types/api";
import { useEditorApi } from "../../hooks/useEditorApi";
import { useLocales } from "../../hooks/useLocales";
import { useWorkspace } from "../../context/WorkspaceContext";
import { ErrorNotice } from "../../errors";
import { getTargetText } from "../editor/blockStatus";
import { type UnifiedSaveResult } from "../UnifiedTargetEditor";
import { ArrowLeft, Rocket, CircleCheck, Sparkles, RefreshCw, FileText } from "../icons";
import { ReviewQueueList } from "./ReviewQueueList";
import { FocusedReviewer, type ReviewerOnBrand } from "./FocusedReviewer";
import { MarkSourceTermDialog } from "./MarkSourceTermDialog";
import { SuggestBrandRuleDialog } from "./SuggestBrandRuleDialog";
import { ProposeSourceChangeDialog } from "./ProposeSourceChangeDialog";
import { SourceProposalsDialog } from "./SourceProposalsDialog";
import { useSourceProposals } from "../../hooks/useSourceProposalsApi";
import {
  entryKey,
  filterEntries,
  isPendingReview,
  noFailingChecksCount,
  queueCounts,
  type ReviewEntry,
  type ReviewGroupBy,
  type ReviewQueueFilter,
  type ReviewCheckStatus,
} from "./reviewQueue";

export interface ReviewSessionProps {
  project: ProjectInfo;
  /** The project's translation dashboard stats (source of pending counts). */
  dashboardStats: TranslationDashboardStats;
  /** The active stream/ref. */
  stream: string;
  /** Navigate to the delivery dashboard (completion CTA + delivery link). */
  onOpenDelivery?: () => void;
  /** Navigate to the runs page (watch the delivering run). */
  onOpenRuns?: () => void;
  /** Navigate to a governance change-set (governed term marks open one). */
  onOpenChangeset?: (changesetId: string) => void;
  /** Optional back affordance in the header. */
  onBack?: () => void;
}

/** One (item, locale) scope that may hold pending-review blocks. */
interface ReviewScope {
  itemId: string;
  itemName: string;
  format?: string;
  collectionId?: string;
  locale: string;
  /** Dashboard says this scope has error-severity findings → fetch QA. */
  failing: boolean;
}

const scopeKey = (s: { itemName: string; locale: string }) => `${s.itemName}::${s.locale}`;

/**
 * Derive the (item, locale) scopes that may hold pending-review blocks from the
 * dashboard stats — a scope is a candidate when it has translated blocks not
 * yet approved, or any failing check. `isPendingReview` is the authoritative
 * per-block filter once the blocks are fetched.
 */
function scopesFromStats(stats: TranslationDashboardStats, targetLocales: string[]): ReviewScope[] {
  const targets = new Set(targetLocales);
  const scopes: ReviewScope[] = [];
  for (const item of stats.item_stats) {
    for (const ls of item.locales) {
      if (!targets.has(ls.locale)) continue;
      const approved = ls.approved_blocks ?? 0;
      const failing = (ls.failing_checks ?? 0) > 0;
      if (ls.translated_blocks > approved || failing) {
        scopes.push({
          itemId: item.item_id,
          itemName: item.item_name,
          format: item.format,
          collectionId: item.collection_id,
          locale: ls.locale,
          failing,
        });
      }
    }
  }
  return scopes;
}

/** Resolve the bound brand profile from the active stream, if any. */
function resolveBrandProfile(project: ProjectInfo, stream: string): string | undefined {
  const s = project.streams?.find((x) => x.name === stream);
  return (
    s?.properties?.brand_voice_profile_id || project.properties?.brand_voice_profile_id || undefined
  );
}

/** Per-locale on-brand context for the reviewer header, from the dashboard. */
function onBrandForLocale(
  stats: TranslationDashboardStats,
  locale: string,
): ReviewerOnBrand | undefined {
  const ls: LocaleTranslationStats | undefined = stats.locale_stats.find(
    (l) => l.locale === locale,
  );
  if (!ls || ls.on_brand_rate == null || ls.on_brand_basis == null) return undefined;
  return {
    rate: ls.on_brand_rate,
    basis: ls.on_brand_basis,
    onBrandBlocks: ls.on_brand_blocks,
    translatedBlocks: ls.translated_blocks,
  };
}

/**
 * ReviewSession is the governed review experience: one focused flow that walks
 * every pending block across a project's items and locales. The left rail is
 * the grouped/filterable queue; the right pane is the bidirectional focused
 * reviewer; the header carries live counts, filters, and the solo-founder
 * "Approve all passing" fast path. It is keyboard-first (j/k move, a approve,
 * r reject, e edit) and, when the queue empties, reflects the server's
 * auto-continue to delivery rather than dead-ending.
 */
export function ReviewSession({
  project,
  dashboardStats,
  stream,
  onOpenDelivery,
  onOpenRuns,
  onOpenChangeset,
  onBack,
}: ReviewSessionProps) {
  const api = useEditorApi();
  const { getDisplayName } = useLocales();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const sourceLocale = project.default_source_language;
  const brandProfileId = useMemo(() => resolveBrandProfile(project, stream), [project, stream]);

  const scopes = useMemo(
    () => scopesFromStats(dashboardStats, project.target_languages),
    [dashboardStats, project.target_languages],
  );

  // The queue comes from the server's pending-review pages — one indexed
  // query, not a blocks fetch per item (978 items once meant minutes of
  // "gathering", over only the first dashboard page of scopes). The QA pass
  // still runs per scope the dashboard flags as failing.
  const buildQueue = useCallback(async (): Promise<ReviewEntry[]> => {
    const pageSize = 200;
    const blocksByItem = new Map<string, BlockInfo[]>();
    const seenBlocks = new Set<string>();
    let offset = 0;
    for (;;) {
      const page = await api
        .getPendingReview(project.id, { limit: pageSize, offset })
        .catch(() => null);
      if (!page) break;
      for (const e of page.entries) {
        if (!e.block || seenBlocks.has(`${e.item_name}:${e.block_id}`)) continue;
        seenBlocks.add(`${e.item_name}:${e.block_id}`);
        const list = blocksByItem.get(e.item_name) ?? [];
        list.push(e.block);
        blocksByItem.set(e.item_name, list);
      }
      offset += page.entries.length;
      if (offset >= page.total || page.entries.length === 0) break;
    }
    const qaByScope = new Map<string, Map<string, QAIssue[]>>();
    await Promise.all(
      scopes
        .filter((s) => s.failing)
        .map(async (s) => {
          const results = await api
            .runFileQACheck(project.id, s.itemName, s.locale)
            .catch(() => []);
          qaByScope.set(scopeKey(s), new Map(results.map((r) => [r.blockId, r.issues])));
        }),
    );
    const entries: ReviewEntry[] = [];
    for (const s of scopes) {
      const blocks = blocksByItem.get(s.itemName) ?? [];
      const issues = qaByScope.get(scopeKey(s));
      for (const b of blocks) {
        if (!isPendingReview(b, s.locale)) continue;
        entries.push({
          id: entryKey(s.itemId, b.id, s.locale),
          itemId: s.itemId,
          itemName: s.itemName,
          format: s.format,
          collectionId: s.collectionId,
          locale: s.locale,
          block: b,
          issues: issues?.get(b.id) ?? [],
        });
      }
    }
    return entries;
  }, [api, project.id, scopes]);

  const {
    data,
    isLoading,
    error: loadError,
    refetch,
    isFetching,
  } = useQuery({
    queryKey: ["review-session", ws, project.id, stream, scopes.length],
    queryFn: buildQueue,
    staleTime: 15_000,
  });

  // Local mirror of the queue so approve/reject/edit apply optimistically.
  const [entries, setEntries] = useState<ReviewEntry[]>([]);
  useEffect(() => {
    if (data) setEntries(data);
  }, [data]);

  const [filter, setFilter] = useState<ReviewQueueFilter>({});
  const [groupBy, setGroupBy] = useState<ReviewGroupBy>("item");
  const [currentIndex, setCurrentIndex] = useState(0);
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [recheckingId, setRecheckingId] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [delivering, setDelivering] = useState(false);
  const actedRef = useRef(false);

  // Dialog state for the source-side affordances.
  const [markTermText, setMarkTermText] = useState<string | null>(null);
  const [brandRule, setBrandRule] = useState<{ original: string; blockId?: string } | null>(null);
  // Back-to-source review (RV-F): the block whose source is being proposed for a
  // change, and whether the source-owner review surface is open.
  const [proposeSource, setProposeSource] = useState<{
    text: string;
    blockId: string;
    itemName: string;
    locale: string;
  } | null>(null);
  const [showProposals, setShowProposals] = useState(false);
  const { data: openProposals = [] } = useSourceProposals(project.id);
  const localeCount = project.target_languages?.length ?? 0;

  const visible = useMemo(() => filterEntries(entries, filter), [entries, filter]);
  const counts = useMemo(() => queueCounts(entries), [entries]);
  // The queue payload carries check findings only, so this is an upper bound on
  // what the server's approve-passing will take (it also applies the term and
  // brand-voice bars). The response's Approved/Skipped split is what happened.
  const noFailing = useMemo(() => noFailingChecksCount(entries), [entries]);

  // Clamp the cursor whenever the visible list shrinks (filter change, removal).
  useEffect(() => {
    setCurrentIndex((i) => (visible.length === 0 ? 0 : Math.min(i, visible.length - 1)));
  }, [visible.length]);

  const current = visible[currentIndex] ?? null;

  const move = useCallback(
    (delta: number) => {
      setEditing(false);
      setCurrentIndex((i) => Math.min(Math.max(i + delta, 0), Math.max(visible.length - 1, 0)));
    },
    [visible.length],
  );

  const selectId = useCallback(
    (id: string) => {
      const idx = visible.findIndex((e) => e.id === id);
      if (idx >= 0) {
        setEditing(false);
        setCurrentIndex(idx);
      }
    },
    [visible],
  );

  // Approve / reject: optimistic removal (the next pending block slides into
  // place → "advance"); on failure, refetch to resync with server truth.
  const decide = useCallback(
    async (entry: ReviewEntry, reviewed: boolean) => {
      if (busy) return;
      setBusy(true);
      setEditing(false);
      actedRef.current = true;
      setEntries((prev) => prev.filter((e) => e.id !== entry.id));
      try {
        await api.reviewBlock(
          project.id,
          entry.itemName,
          entry.block.id,
          entry.locale,
          reviewed,
          reviewed ? undefined : "draft",
        );
      } catch (e) {
        setActionError(e);
        void refetch();
      } finally {
        setBusy(false);
      }
    },
    [api, busy, project.id, refetch],
  );

  const approve = useCallback(() => {
    if (current) void decide(current, true);
  }, [current, decide]);
  const reject = useCallback(() => {
    if (current) void decide(current, false);
  }, [current, decide]);

  // Re-check a single block after an edit (or on demand), refreshing its
  // findings in place.
  const recheck = useCallback(
    async (entry: ReviewEntry) => {
      setRecheckingId(entry.id);
      try {
        const issues = await api.runQACheck(project.id, entry.block.id, entry.locale);
        setEntries((prev) => prev.map((e) => (e.id === entry.id ? { ...e, issues } : e)));
      } catch {
        // Non-fatal: leave the prior findings.
      } finally {
        setRecheckingId(null);
      }
    },
    [api, project.id],
  );

  // Promote a marked source entity to a terms store concept (RV-F piece 3). The
  // resulting concept.created flows into the governed terminology re-check.
  const promoteEntity = useCallback(
    async (entry: ReviewEntry, entityKey: string) => {
      try {
        await api.promoteEntityToConcept(project.id, entry.itemName, entry.block.id, entityKey);
        setMessage("Promoted to a terms store concept.");
      } catch (e) {
        setActionError(e);
      }
    },
    [api, project.id],
  );

  // Persist an inline edit, then reload the item's blocks so the reviewer shows
  // server truth (status demotes to translated on a content change), and
  // re-check the block.
  const saveEdit = useCallback(
    async (entry: ReviewEntry, result: UnifiedSaveResult) => {
      setBusy(true);
      try {
        if (result.kind === "flat") {
          await api.updateBlockTargetCoded({
            project_id: project.id,
            item_name: entry.itemName,
            block_id: entry.block.id,
            target_locale: entry.locale,
            coded_text: result.codedText,
            spans: result.spans,
          });
        } else {
          await api.updateBlockTarget({
            project_id: project.id,
            item_name: entry.itemName,
            block_id: entry.block.id,
            target_locale: entry.locale,
            text: result.text,
          });
        }
        setEditing(false);
        const blocks = await api.getFileBlocks(project.id, entry.itemName);
        const fresh = blocks.find((b) => b.id === entry.block.id);
        if (fresh) {
          setEntries((prev) => prev.map((e) => (e.id === entry.id ? { ...e, block: fresh } : e)));
          void recheck({ ...entry, block: fresh });
        }
      } catch (e) {
        setActionError(e);
      } finally {
        setBusy(false);
      }
    },
    [api, project.id, recheck],
  );

  // Bulk "Approve all passing": server promotes every pending block passing
  // checks + the on-brand bar. review_completed → the queue emptied and
  // delivery kicked off.
  const [confirmBulk, setConfirmBulk] = useState(false);
  const runBulkApprove = useCallback(async () => {
    setConfirmBulk(false);
    setBusy(true);
    actedRef.current = true;
    try {
      const locales = filter.locale ? [filter.locale] : undefined;
      const result = await api.approvePassing(project.id, locales);
      setMessage(
        `Approved ${result.approved} block${result.approved === 1 ? "" : "s"}` +
          (result.skipped > 0
            ? ` · ${result.skipped} skipped for failing checks, terminology, or the brand bar`
            : ""),
      );
      if (result.review_completed) setDelivering(true);
      await refetch();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(false);
    }
  }, [api, project.id, filter.locale, refetch]);

  // Auto-continue: when an action empties the queue, reflect the server's
  // completing run + delivery rather than dead-ending.
  useEffect(() => {
    if (!isLoading && !isFetching && entries.length === 0 && actedRef.current) {
      setDelivering(true);
    }
  }, [entries.length, isLoading, isFetching]);

  // Keyboard model: j/k (or ↓/↑) move, a approve, r reject, e edit. Suppressed
  // while editing or when focus is in a field, so typing is never intercepted.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (editing || busy) return;
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || el?.isContentEditable) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          move(1);
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          move(-1);
          break;
        case "a":
          e.preventDefault();
          approve();
          break;
        case "r":
          e.preventDefault();
          reject();
          break;
        case "e":
          if (current) {
            e.preventDefault();
            setEditing(true);
          }
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [editing, busy, move, approve, reject, current]);

  const localeName = useCallback((code: string) => getDisplayName(code), [getDisplayName]);

  // ── Filter controls (locale + check-status pills, item select) ────────────
  const localesWithPending = Object.keys(counts.byLocale);
  const checkFilters: { value: ReviewCheckStatus; label: string; count: number }[] = [
    { value: "failing", label: "Failing", count: counts.failing },
    { value: "clean", label: "No failing checks", count: counts.clean },
  ];

  // ── Render ────────────────────────────────────────────────────────────────
  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> Gathering pending review…
      </div>
    );
  }

  if (loadError) {
    return (
      <div className="p-6">
        <ErrorNotice
          error={loadError}
          title="Couldn't load the review queue"
          variant="inline"
          onRetry={() => void refetch()}
        />
      </div>
    );
  }

  const allClear = entries.length === 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="review-session">
      {/* Header: title, live counts, filters, bulk approve */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
        {onBack && (
          <button
            onClick={onBack}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
            data-testid="review-back"
          >
            <ArrowLeft className="h-3.5 w-3.5" /> {project.name}
          </button>
        )}
        <span className="text-base font-semibold">Review</span>
        <span
          className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium tabular-nums text-muted-foreground"
          data-testid="review-pending-count"
        >
          {counts.total} pending
        </span>
        <div className="flex-1" />
        {openProposals.length > 0 && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setShowProposals(true)}
            data-testid="open-source-proposals"
          >
            <FileText className="mr-1 h-4 w-4" /> Source proposals
            <span className="ml-1.5 rounded-full bg-muted px-1.5 text-[11px] tabular-nums">
              {openProposals.length}
            </span>
          </Button>
        )}
        {!allClear && (
          <Button
            size="sm"
            onClick={() => setConfirmBulk(true)}
            disabled={busy || noFailing === 0}
            data-testid="approve-all-passing"
          >
            <Sparkles className="mr-1 h-4 w-4" /> Approve all passing
            <span className="ml-1.5 rounded-full bg-white/20 px-1.5 text-[11px] tabular-nums">
              {noFailing}
            </span>
          </Button>
        )}
      </div>

      {/* Filter bar */}
      {!allClear && (
        <div
          className="flex flex-wrap items-center gap-1.5 border-b border-border px-4 py-2"
          data-testid="review-filters"
        >
          <span className="text-[11px] uppercase tracking-wide text-muted-foreground">Locale</span>
          {localesWithPending.map((loc) => (
            <button
              key={loc}
              type="button"
              onClick={() =>
                setFilter((f) => ({ ...f, locale: f.locale === loc ? undefined : loc }))
              }
              data-testid={`filter-locale-${loc}`}
              className={cn(
                "rounded-md border px-2 py-0.5 text-xs transition-colors",
                filter.locale === loc
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-card text-muted-foreground hover:text-foreground",
              )}
            >
              {localeName(loc)} ({counts.byLocale[loc]})
            </button>
          ))}
          <span className="ml-3 text-[11px] uppercase tracking-wide text-muted-foreground">
            Checks
          </span>
          {checkFilters.map((cf) => (
            <button
              key={cf.value}
              type="button"
              onClick={() =>
                setFilter((f) => ({ ...f, check: f.check === cf.value ? undefined : cf.value }))
              }
              data-testid={`filter-check-${cf.value}`}
              className={cn(
                "rounded-md border px-2 py-0.5 text-xs transition-colors",
                filter.check === cf.value
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-card text-muted-foreground hover:text-foreground",
              )}
            >
              {cf.label} ({cf.count})
            </button>
          ))}
          {(filter.locale || filter.check || filter.itemId) && (
            <button
              type="button"
              onClick={() => setFilter({})}
              className="ml-2 text-xs text-muted-foreground underline-offset-2 hover:underline"
              data-testid="filter-clear"
            >
              Clear
            </button>
          )}
        </div>
      )}

      {/* Messages */}
      {actionError != null && (
        <ErrorNotice
          error={actionError}
          title="The review action didn't go through"
          variant="inline"
          className="mx-4 mt-2"
        />
      )}
      {message && (
        <Alert className="mx-4 mt-2 border-success/25 text-success">
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}

      {/* Body */}
      {allClear ? (
        <CompletionState
          delivering={delivering}
          onOpenDelivery={onOpenDelivery}
          onOpenRuns={onOpenRuns}
        />
      ) : (
        <div className="flex min-h-0 flex-1">
          <ReviewQueueList
            entries={visible}
            groupBy={groupBy}
            onGroupByChange={setGroupBy}
            currentId={current?.id ?? null}
            onSelect={selectId}
            localeName={localeName}
          />
          {current ? (
            <FocusedReviewer
              entry={current}
              sourceLocale={sourceLocale}
              position={{ index: currentIndex + 1, total: visible.length }}
              localeName={localeName}
              onBrand={onBrandForLocale(dashboardStats, current.locale)}
              editing={editing}
              busy={busy}
              reChecking={recheckingId === current.id}
              brandProfileId={brandProfileId}
              onApprove={approve}
              onReject={reject}
              onEditToggle={() => setEditing((v) => !v)}
              onSaveEdit={(result) => saveEdit(current, result)}
              onCancelEdit={() => setEditing(false)}
              onReCheck={() => void recheck(current)}
              onMarkTerm={(text) => setMarkTermText(text)}
              onSuggestBrandRule={(text) =>
                setBrandRule({ original: text, blockId: current.block.id })
              }
              onMakeRule={() =>
                setBrandRule({
                  original: getTargetText(current.block, current.locale),
                  blockId: current.block.id,
                })
              }
              onProposeSourceChange={(text) =>
                setProposeSource({
                  text,
                  blockId: current.block.id,
                  itemName: current.itemName,
                  locale: current.locale,
                })
              }
              onEntityPromote={(entityKey) => void promoteEntity(current, entityKey)}
            />
          ) : (
            <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
              Select a block to review.
            </div>
          )}
        </div>
      )}

      {/* Bulk approve confirmation */}
      <ConfirmDialog
        open={confirmBulk}
        onOpenChange={setConfirmBulk}
        title="Approve all passing?"
        description={`${noFailing} block${noFailing === 1 ? "" : "s"}${
          filter.locale ? ` in ${localeName(filter.locale)}` : ""
        } carry no failing check. The server applies the terminology and brand bars on approve, so it may take fewer; the rest stay for review.`}
        confirmLabel="Approve passing"
        onConfirm={() => void runBulkApprove()}
        loading={busy}
      />

      {/* Source-side affordances */}
      <MarkSourceTermDialog
        open={markTermText !== null}
        onOpenChange={(o) => !o && setMarkTermText(null)}
        projectId={project.id}
        sourceLocale={sourceLocale}
        initialTerm={markTermText ?? ""}
        onDone={setMessage}
        onOpenChangeset={onOpenChangeset}
      />
      {brandProfileId && (
        <SuggestBrandRuleDialog
          open={brandRule !== null}
          onOpenChange={(o) => !o && setBrandRule(null)}
          projectId={project.id}
          stream={stream}
          profileId={brandProfileId}
          initialOriginal={brandRule?.original ?? ""}
          blockId={brandRule?.blockId}
          onDone={setMessage}
        />
      )}

      {/* Back-to-source review (RV-F): propose a source change, and the source
          owner's surface for approving/rejecting open proposals. */}
      {proposeSource && (
        <ProposeSourceChangeDialog
          open={proposeSource !== null}
          onOpenChange={(o) => !o && setProposeSource(null)}
          projectId={project.id}
          blockId={proposeSource.blockId}
          itemName={proposeSource.itemName}
          sourceLocale={sourceLocale}
          foundInLocale={proposeSource.locale}
          initialSource={proposeSource.text}
          localeCount={localeCount}
          onDone={setMessage}
        />
      )}
      <SourceProposalsDialog
        open={showProposals}
        onOpenChange={setShowProposals}
        projectId={project.id}
        localeName={localeName}
        onDone={setMessage}
      />
    </div>
  );
}

/** Terminal state of the session: nothing pending, or all approved + delivering. */
function CompletionState({
  delivering,
  onOpenDelivery,
  onOpenRuns,
}: {
  delivering: boolean;
  onOpenDelivery?: () => void;
  onOpenRuns?: () => void;
}) {
  return (
    <div
      className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center"
      data-testid={delivering ? "review-delivering" : "review-all-clear"}
    >
      <div
        className={cn(
          "flex h-14 w-14 items-center justify-center rounded-full",
          delivering ? "bg-primary/10 text-primary" : "bg-success/10 text-success",
        )}
      >
        {delivering ? <Rocket className="h-7 w-7" /> : <CircleCheck className="h-7 w-7" />}
      </div>
      <div className="space-y-1">
        <p className="text-lg font-semibold">
          {delivering ? "All approved · delivering…" : "Nothing to review"}
        </p>
        <p className="max-w-sm text-sm text-muted-foreground">
          {delivering
            ? "Every pending block is approved. The loop is completing and delivery is on its way."
            : "This project has no blocks awaiting review right now."}
        </p>
      </div>
      {delivering && (
        <div className="flex items-center gap-2">
          {onOpenDelivery && (
            <Button size="sm" onClick={onOpenDelivery} data-testid="completion-delivery">
              Delivery dashboard
            </Button>
          )}
          {onOpenRuns && (
            <Button size="sm" variant="outline" onClick={onOpenRuns} data-testid="completion-runs">
              View the run
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
