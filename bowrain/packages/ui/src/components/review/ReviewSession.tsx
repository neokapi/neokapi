import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button, Alert, AlertDescription, cn } from "@neokapi/ui-primitives";
import { ConfirmDialog } from "../ConfirmDialog";
import type {
  ApprovePassingResult,
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
import { getTargetText, statusAfterEdit, withTargetEntry } from "../editor/blockStatus";
import { type UnifiedSaveResult } from "../UnifiedTargetEditor";
import { ArrowLeft, Rocket, CircleCheck, Sparkles, RefreshCw, FileText } from "../icons";
import { ReviewQueueList } from "./ReviewQueueList";
import { FocusedReviewer, type ReviewerCompliance } from "./FocusedReviewer";
import { MarkSourceTermDialog } from "./MarkSourceTermDialog";
import { SuggestVoiceRuleDialog } from "./SuggestVoiceRuleDialog";
import { ProposeSourceChangeDialog } from "./ProposeSourceChangeDialog";
import { SourceProposalsDialog } from "./SourceProposalsDialog";
import { useSourceProposals } from "../../hooks/useSourceProposalsApi";
import {
  entryKey,
  filterEntries,
  isPendingReview,
  passingCount,
  queueCounts,
  VERDICT_LABELS,
  type ReviewEntry,
  type ReviewGroupBy,
  type ReviewQueueFilter,
  type ReviewQueueVerdict,
} from "./reviewQueue";

export interface ReviewSessionProps {
  project: ProjectInfo;
  /** The project's translation dashboard stats (source of pending counts). */
  dashboardStats: TranslationDashboardStats;
  /** The active stream/ref. */
  stream: string;
  /**
   * Filter the session opens on — how a collection carries into review from the
   * project overview. The reviewer can clear it like any other filter.
   *
   * The collection scope is applied by the server, so the queue loaded is the
   * collection's queue and its reported total is the collection's total. The
   * item and locale filters narrow the loaded slice in place.
   */
  initialFilter?: ReviewQueueFilter;
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
  locale: string;
  /** Dashboard says this scope has error-severity findings → fetch QA. */
  failing: boolean;
}

const scopeKey = (s: { itemName: string; locale: string }) => `${s.itemName}::${s.locale}`;

/** How many pending entries one page of the queue carries. */
const QUEUE_PAGE_SIZE = 200;

/**
 * How many pending entries one session loads. The header reports the server's
 * total, so a queue larger than this is named in full and worked a slice at a
 * time — emptying the slice refetches the next one.
 */
const QUEUE_MAX_ENTRIES = 1000;

/** One page of the queue, plus the size of the queue it came from. */
interface QueueLoad {
  entries: ReviewEntry[];
  total: number;
}

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
          locale: ls.locale,
          failing,
        });
      }
    }
  }
  return scopes;
}

/**
 * Name the bars a bulk pass's skipped blocks missed, in the server's own
 * counts. "Skipped for failing checks, terminology, or the brand bar" named all
 * three every time and therefore none of them; the response now attributes each
 * skip to exactly one.
 */
function skipReasons(result: ApprovePassingResult): string {
  const parts: string[] = [];
  if (result.skipped_failing_checks) parts.push(`${result.skipped_failing_checks} failing checks`);
  if (result.skipped_term_violations) parts.push(`${result.skipped_term_violations} terminology`);
  if (result.skipped_below_voice_bar) {
    parts.push(`${result.skipped_below_voice_bar} below the voice bar`);
  }
  return parts.length > 0 ? parts.join(", ") : "no reason given";
}

/** Resolve the bound voice profile from the active stream, if any. */
function resolveVoiceProfile(project: ProjectInfo, stream: string): string | undefined {
  const s = project.streams?.find((x) => x.name === stream);
  return s?.properties?.voice_profile_id || project.properties?.voice_profile_id || undefined;
}

/** Per-locale compliance context for the reviewer header, from the dashboard. */
function complianceForLocale(
  stats: TranslationDashboardStats,
  locale: string,
): ReviewerCompliance | undefined {
  const ls: LocaleTranslationStats | undefined = stats.locale_stats.find(
    (l) => l.locale === locale,
  );
  if (!ls || ls.compliance_rate == null || ls.compliance_basis == null) return undefined;
  return {
    rate: ls.compliance_rate,
    basis: ls.compliance_basis,
    compliantBlocks: ls.compliant_blocks,
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
  initialFilter,
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
  const voiceProfileId = useMemo(() => resolveVoiceProfile(project, stream), [project, stream]);

  const targetLocales = useMemo(() => project.target_languages ?? [], [project.target_languages]);

  const scopes = useMemo(
    () => scopesFromStats(dashboardStats, targetLocales),
    [dashboardStats, targetLocales],
  );

  // Item metadata for the entries the server hands back — the dashboard names
  // the id and format a queue row is grouped by. The collection is NOT read
  // from here: the queue payload carries it, from the same join the server's
  // collection filter tests, so a row can never be filed under a collection the
  // filter would not have selected it for.
  const itemMeta = useMemo(() => {
    const m = new Map<string, { itemId: string; format?: string }>();
    for (const item of dashboardStats.item_stats) {
      m.set(item.item_name, { itemId: item.item_id, format: item.format });
    }
    return m;
  }, [dashboardStats]);

  // The (item, locale) scopes the dashboard reports error-severity findings
  // for — the only ones worth a QA request.
  const failingScopes = useMemo(
    () => new Set(scopes.filter((s) => s.failing).map(scopeKey)),
    [scopes],
  );

  const [filter, setFilter] = useState<ReviewQueueFilter>(initialFilter ?? {});
  // The collection the queue is loaded for. It is a query scope, not a view
  // filter: the server pages that collection's queue and reports its total, so
  // a collection larger than the session's slice is worked a slice at a time
  // instead of being sieved out of the project's first thousand entries.
  const collectionScope = filter.collectionId;

  // The queue comes from the server's pending-review pages — one indexed
  // query, not a blocks fetch per item (978 items once meant minutes of
  // "gathering"). Both passes are bounded: the pages stop at the session's
  // slice, and QA runs only for the flagged scopes the slice actually holds.
  const buildQueue = useCallback(async (): Promise<QueueLoad> => {
    const loaded: (Omit<ReviewEntry, "id" | "itemId" | "format" | "issues"> & {
      itemName: string;
    })[] = [];
    const seen = new Set<string>();
    let total = 0;
    for (let offset = 0; offset < QUEUE_MAX_ENTRIES; offset += QUEUE_PAGE_SIZE) {
      const page = await api
        .getPendingReview(project.id, {
          locales: targetLocales,
          collectionId: collectionScope,
          limit: QUEUE_PAGE_SIZE,
          offset,
        })
        .catch(() => null);
      if (!page) break;
      total = page.total;
      const pageEntries = page.entries ?? [];
      for (const e of pageEntries) {
        const key = `${e.item_name}::${e.block_id}::${e.locale}`;
        if (!e.block || seen.has(key)) continue;
        seen.add(key);
        loaded.push({
          itemName: e.item_name,
          locale: e.locale,
          collectionId: e.collection_id ?? "",
          block: e.block,
          // The bars beyond QA, as the server judges them. Absent fields mean
          // "not applied", never "cleared".
          termCompliance: e.term_compliance ?? "",
          voiceScore: e.voice_score,
          voiceBar: e.voice_bar,
        });
      }
      if (pageEntries.length < QUEUE_PAGE_SIZE || offset + pageEntries.length >= page.total) break;
    }

    const qaScopes = new Set(
      loaded.map((e) => scopeKey(e)).filter((key) => failingScopes.has(key)),
    );
    const qaByScope = new Map<string, Map<string, QAIssue[]>>();
    await Promise.all(
      [...qaScopes].map(async (key) => {
        const [itemName, locale] = key.split("::");
        const results = await api.runFileQACheck(project.id, itemName, locale).catch(() => []);
        qaByScope.set(key, new Map((results ?? []).map((r) => [r.blockId, r.issues])));
      }),
    );

    const entries: ReviewEntry[] = [];
    for (const e of loaded) {
      if (!isPendingReview(e.block, e.locale)) continue;
      const meta = itemMeta.get(e.itemName);
      const itemId = meta?.itemId ?? e.itemName;
      entries.push({
        id: entryKey(itemId, e.block.id, e.locale),
        itemId,
        itemName: e.itemName,
        format: meta?.format,
        collectionId: e.collectionId,
        locale: e.locale,
        block: e.block,
        issues: qaByScope.get(scopeKey(e))?.get(e.block.id) ?? [],
        termCompliance: e.termCompliance,
        voiceScore: e.voiceScore,
        voiceBar: e.voiceBar,
      });
    }
    return { entries, total: Math.max(total, entries.length) };
  }, [api, project.id, targetLocales, collectionScope, failingScopes, itemMeta]);

  const {
    data,
    isLoading,
    error: loadError,
    refetch,
    isFetching,
  } = useQuery({
    // The collection scope keys the query: clearing or changing it loads a
    // different queue from the server rather than re-sieving the loaded one.
    queryKey: ["review-session", ws, project.id, stream, scopes.length, collectionScope ?? null],
    queryFn: buildQueue,
    staleTime: 15_000,
  });

  // Local mirror of the queue so approve/reject/edit apply optimistically, and
  // the server's queue size beside it — the loaded slice may be smaller.
  const [entries, setEntries] = useState<ReviewEntry[]>([]);
  const [pendingTotal, setPendingTotal] = useState(0);
  // One refill per emptied slice: a queue whose reported total never matches a
  // page must not spin the session in refetches.
  const sliceRefillRef = useRef(false);
  useEffect(() => {
    if (data) {
      setEntries(data.entries);
      setPendingTotal(data.total);
      if (data.entries.length > 0) sliceRefillRef.current = false;
    }
  }, [data]);

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
  const [voiceRule, setVoiceRule] = useState<{ original: string; blockId?: string } | null>(null);
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
  // What "Approve all passing" will take, counted over the entries that pass
  // actually covers: the queue as loaded, narrowed by the locale filter it
  // forwards. The queue payload carries all three bars the server applies —
  // check findings, the terminology verdict, and the voice score against its
  // profile's bar — so this is the count, not an upper bound on it. The
  // response still reports the split that happened: the queue is a snapshot,
  // and a re-check in between can move a block.
  const passing = useMemo(
    () => passingCount(filter.locale ? entries.filter((e) => e.locale === filter.locale) : entries),
    [entries, filter.locale],
  );

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
      setPendingTotal((t) => Math.max(0, t - 1));
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

  // Persist an inline edit and carry the saved block into the entry.
  //
  // The block is RE-READ rather than rebuilt: what a write leaves behind is the
  // server's decision (a content change invalidates a review decision and
  // demotes to translated, plus whatever else the write recomputed), and the
  // client predicting it means a second copy of that rule which can disagree.
  //
  // The local reconstruction stays as the fallback, because the read is not
  // part of the save: a save that succeeded must not be reported as failed
  // because the follow-up GET did not land \u2014 offline, or against a server that
  // predates the single-block route.
  const saveEdit = useCallback(
    async (entry: ReviewEntry, result: UnifiedSaveResult) => {
      setBusy(true);
      try {
        let text: string;
        let coded: string | undefined;
        if (result.kind === "flat") {
          await api.updateBlockTargetCoded({
            project_id: project.id,
            item_name: entry.itemName,
            block_id: entry.block.id,
            target_locale: entry.locale,
            coded_text: result.codedText,
            spans: result.spans,
          });
          coded = result.codedText;
          text = result.codedText.replace(/[\uE001-\uE003]/g, "");
        } else {
          await api.updateBlockTarget({
            project_id: project.id,
            item_name: entry.itemName,
            block_id: entry.block.id,
            target_locale: entry.locale,
            text: result.text,
          });
          text = result.text;
        }
        setEditing(false);

        let saved: BlockInfo;
        try {
          saved = await api.getBlock(project.id, entry.block.id);
        } catch {
          saved = {
            ...withTargetEntry(entry.block, entry.locale, {
              text,
              status: statusAfterEdit(entry.block, entry.locale, text, coded),
            }),
            targets_coded: { ...entry.block.targets_coded, [entry.locale]: coded ?? "" },
          };
        }
        setEntries((prev) => prev.map((e) => (e.id === entry.id ? { ...e, block: saved } : e)));
        void recheck({ ...entry, block: saved });
      } catch (e) {
        setActionError(e);
      } finally {
        setBusy(false);
      }
    },
    [api, project.id, recheck],
  );

  // Bulk "Approve all passing": server promotes every pending block passing
  // checks + the compliance bar. review_completed → the queue emptied and
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
          (result.skipped > 0 ? ` · ${result.skipped} skipped (${skipReasons(result)})` : ""),
      );
      if (result.review_completed) setDelivering(true);
      await refetch();
    } catch (e) {
      setActionError(e);
    } finally {
      setBusy(false);
    }
  }, [api, project.id, filter.locale, refetch]);

  // Auto-continue: when an action empties the loaded slice, either pull the
  // next one or — with nothing left pending anywhere — reflect the server's
  // completing run + delivery rather than dead-ending.
  useEffect(() => {
    if (isLoading || isFetching || entries.length > 0 || !actedRef.current) return;
    if (pendingTotal > 0 && !sliceRefillRef.current) {
      sliceRefillRef.current = true;
      void refetch();
      return;
    }
    setDelivering(true);
  }, [entries.length, isLoading, isFetching, pendingTotal, refetch]);

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

  // ── Filter controls (locale + verdict pills, item select) ─────────────────
  const localesWithPending = Object.keys(counts.byLocale);
  // The dashboard's rollups name the collection a scope filter came in with;
  // the empty id is the collection-less bucket, which has no name of its own.
  const collectionFilterName =
    filter.collectionId === ""
      ? "In no collection"
      : (dashboardStats.collection_stats.find((c) => c.collection_id === filter.collectionId)
          ?.collection_name ?? filter.collectionId);
  const verdictFilters: { value: ReviewQueueVerdict; label: string; count: number }[] = [
    { value: "failing", label: VERDICT_LABELS.failing, count: counts.failing },
    { value: "passing", label: VERDICT_LABELS.passing, count: counts.passing },
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
          {pendingTotal} pending
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
            disabled={busy || passing === 0}
            data-testid="approve-all-passing"
          >
            <Sparkles className="mr-1 h-4 w-4" /> Approve all passing
            <span className="ml-1.5 rounded-full bg-white/20 px-1.5 text-[11px] tabular-nums">
              {passing}
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
            Verdict
          </span>
          {verdictFilters.map((cf) => (
            <button
              key={cf.value}
              type="button"
              onClick={() =>
                setFilter((f) => ({ ...f, verdict: f.verdict === cf.value ? undefined : cf.value }))
              }
              data-testid={`filter-verdict-${cf.value}`}
              className={cn(
                "rounded-md border px-2 py-0.5 text-xs transition-colors",
                filter.verdict === cf.value
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-card text-muted-foreground hover:text-foreground",
              )}
            >
              {cf.label} ({cf.count})
            </button>
          ))}
          {filter.collectionId !== undefined && (
            <>
              <span className="ml-3 text-[11px] uppercase tracking-wide text-muted-foreground">
                Collection
              </span>
              <span
                className="rounded-md border border-primary bg-primary px-2 py-0.5 text-xs text-primary-foreground"
                data-testid="filter-collection"
              >
                {collectionFilterName}
              </span>
            </>
          )}
          {(filter.locale ||
            filter.verdict ||
            filter.itemId ||
            filter.collectionId !== undefined) && (
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
            sourceLocale={sourceLocale}
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
              compliance={complianceForLocale(dashboardStats, current.locale)}
              editing={editing}
              busy={busy}
              reChecking={recheckingId === current.id}
              voiceProfileId={voiceProfileId}
              onApprove={approve}
              onReject={reject}
              onEditToggle={() => setEditing((v) => !v)}
              onSaveEdit={(result) => saveEdit(current, result)}
              onCancelEdit={() => setEditing(false)}
              onReCheck={() => void recheck(current)}
              onMarkTerm={(text) => setMarkTermText(text)}
              onSuggestVoiceRule={(text) =>
                setVoiceRule({ original: text, blockId: current.block.id })
              }
              onMakeRule={() =>
                setVoiceRule({
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
        description={
          `${passing} block${passing === 1 ? "" : "s"}${
            filter.locale ? ` in ${localeName(filter.locale)}` : ""
          } in this queue clear the checks, the terminology and the voice bar. The rest stay for review.` +
          // The bulk pass takes a stream and locales, not a collection, so with
          // a collection scope open the count above describes a narrower set
          // than the act. Saying so beats a count that quietly means something
          // else than the button does.
          (collectionScope !== undefined
            ? ` This pass covers the whole project, not just ${collectionFilterName}.`
            : "")
        }
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
      {voiceProfileId && (
        <SuggestVoiceRuleDialog
          open={voiceRule !== null}
          onOpenChange={(o) => !o && setVoiceRule(null)}
          projectId={project.id}
          stream={stream}
          profileId={voiceProfileId}
          initialOriginal={voiceRule?.original ?? ""}
          blockId={voiceRule?.blockId}
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
