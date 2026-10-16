// The workspace's concepts as a list beside a detail pane: the platform's twin
// of the Terms page in Kapi Desktop.
//
// The list has one row per concept: the term it is headed by, that term's
// status, and how many languages the concept has terms in. The search and the
// status and market pickers at the top of the list ask the server, a page at a
// time, so a workspace with thousands of concepts reads as lightly as one with
// ten. The detail pane holds the selected concept (ConceptDetail).
//
// A governed change never appears in the list: a proposed concept is not a
// concept until its change-set merges. The strip above the split names what is
// waiting (PendingConceptChanges), and the empty page says which of its two
// meanings it has: nothing here yet, or everything here waiting on a reviewer.
import { useCallback, useMemo, useState } from "react";
import { keepPreviousData, useInfiniteQuery } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import {
  TERM_STATUSES,
  primaryName,
  type ConceptNaming,
  type TermStatus,
} from "@neokapi/concept-ui";
import {
  Button,
  EmptyState,
  Input,
  ListDetail,
  ListDetailEmpty,
  ListDetailItem,
  ListDetailList,
  LoadingSpinner,
  PageHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
  useDebounced,
} from "@neokapi/ui-primitives";
import { Layers, Search, Shield } from "../../components/icons";
import { ErrorNotice } from "../../errors";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { useConceptStatusCounts } from "../../hooks/useConceptsApi";
import { useMarkets } from "../../hooks/useMarketsApi";
import type { ConceptInfo } from "../../types/api";
import type { ListConceptsParams } from "../../types/brand-graph";
import {
  conceptSummaryFromInfo,
  createRestConceptSource,
  type GovernedEditError,
} from "./restConceptSource";
import { PendingConceptChanges, usePendingConceptChanges } from "./PendingConceptChanges";
import { useWorkspaceConceptNaming } from "./workspace-naming";
import { headlineTerm, languageCount } from "./conceptFacts";
import { ConceptDetail } from "./ConceptDetail";
import { GovernedEditDialog } from "./GovernedEditDialog";
import { TermStatusChip, termStatusLabel } from "./TermStatusChip";

/** How many concepts one read of the list carries. */
export const CONCEPT_PAGE = 100;

/** The picker value that leaves a filter off. */
const ALL = "all";

export interface ConceptsSectionProps {
  /**
   * The concept to show, by id. The route carries it, so a link opens the list
   * with that concept selected. Absent, the section keeps the selection itself.
   */
  selectedId?: string | null;
  /** The concept to open on when the section keeps the selection itself. */
  defaultSelected?: string;
  /** A concept was selected: in the list, or by following a relation. */
  onSelect?: (conceptId: string) => void;
  /** Open a change-set's review page. */
  onOpenChangeSet?: (changesetId: string) => void;
  /** Open the Changes section, where a governed edit goes into a change-set. */
  onOpenExperiments?: () => void;
}

export function ConceptsSection({
  selectedId: controlled,
  defaultSelected,
  onSelect,
  onOpenChangeSet,
  onOpenExperiments,
}: ConceptsSectionProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const naming = useWorkspaceConceptNaming();
  const { pending } = usePendingConceptChanges();
  const counts = useConceptStatusCounts();
  const { data: markets } = useMarkets();

  const [governed, setGoverned] = useState<GovernedEditError | null>(null);
  const source = useMemo(
    () => createRestConceptSource(api, ws, { onGovernedEdit: setGoverned }),
    [api, ws],
  );

  const [own, setOwn] = useState<string | null>(defaultSelected ?? null);
  const selectedId = controlled !== undefined ? controlled : own;
  const select = useCallback(
    (id: string) => {
      setOwn(id);
      onSelect?.(id);
    },
    [onSelect],
  );

  const [text, setText] = useState("");
  const q = useDebounced(text.trim());
  const [status, setStatus] = useState<string>(ALL);
  const [market, setMarket] = useState<string>(ALL);
  const params = useMemo<ListConceptsParams>(() => {
    const p: ListConceptsParams = {};
    if (q) p.q = q;
    if (status !== ALL) p.status = status as TermStatus;
    if (market !== ALL) p.market = market;
    return p;
  }, [q, status, market]);
  const filtered = Object.keys(params).length > 0;

  const list = useInfiniteQuery({
    // Under the "concepts" key every concept write already refreshes.
    queryKey: ["concepts", ws, "browse", params],
    queryFn: ({ pageParam }) =>
      api.listConcepts(ws, { ...params, offset: pageParam, limit: CONCEPT_PAGE }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + p.concepts.length, 0);
      return last.concepts.length > 0 && loaded < last.total_count ? loaded : undefined;
    },
    enabled: !!ws,
    // The previous answer stays on screen while the next search is read, so
    // the list does not empty and refill on every keystroke.
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
  const concepts = useMemo(() => list.data?.pages.flatMap((p) => p.concepts) ?? [], [list.data]);
  const total = list.data?.pages[0]?.total_count ?? 0;
  const summary = useMemo(() => {
    const hit = concepts.find((c) => c.id === selectedId);
    return hit ? conceptSummaryFromInfo(hit) : undefined;
  }, [concepts, selectedId]);

  const workspaceTotal = counts.data?.total;
  const header = (
    <PageHeader
      className="mb-0 shrink-0"
      title="Concepts"
      subtitle={
        workspaceTotal !== undefined
          ? t("{count, plural, one {# concept} other {# concepts}}", { count: workspaceTotal })
          : undefined
      }
    />
  );

  let body: React.ReactNode;
  if (list.error && !list.data) {
    body = <ErrorNotice error={list.error} title={t("The concepts could not be read")} />;
  } else if (list.isLoading || !ws) {
    body = <LoadingSpinner />;
  } else if (concepts.length === 0 && !filtered) {
    body = <ConceptsZeroState pendingCount={pending.length} />;
  } else {
    const listPane = (
      <>
        <div className="sticky top-0 z-10 space-y-1.5 bg-background px-2 pt-2 pb-1">
          <div className="relative">
            <Search
              aria-hidden
              className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              type="search"
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                // Down from the search goes into the list, onto its first row.
                if (e.key === "ArrowDown" && concepts.length > 0) {
                  e.preventDefault();
                  if (!selectedId) select(concepts[0].id);
                  e.currentTarget
                    .closest("[data-pane='list']")
                    ?.querySelector<HTMLElement>("[role='listbox']")
                    ?.focus();
                }
              }}
              placeholder={t("Search concepts")}
              aria-label={t("Search concepts")}
              data-testid="concept-search"
              className="h-8 pl-8 text-sm"
            />
          </div>
          <div className="flex gap-1.5">
            <Select value={status} onValueChange={setStatus}>
              <SelectTrigger
                size="sm"
                data-slot="concept-status-filter"
                aria-label={t("Term status")}
                className="h-8 min-w-0 flex-1 text-xs"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>Every status</SelectItem>
                {TERM_STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {termStatusLabel(s)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {markets && markets.length > 0 && (
              <Select value={market} onValueChange={setMarket}>
                <SelectTrigger
                  size="sm"
                  data-slot="concept-market-filter"
                  aria-label={t("Market")}
                  className="h-8 min-w-0 flex-1 text-xs"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>Every market</SelectItem>
                  {markets.map((m) => (
                    <SelectItem key={m.id} value={m.name}>
                      <span translate="no">{m.name}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
          {filtered && (
            <p className="px-1 text-[11px] text-muted-foreground tabular-nums">
              {t("{count, plural, one {# match} other {# matches}}", { count: total })}
            </p>
          )}
        </div>
        {concepts.length === 0 ? (
          <p className="px-4 py-6 text-center text-xs text-muted-foreground">
            No concept matches this search.
          </p>
        ) : (
          <ListDetailList
            selectedId={selectedId}
            onSelect={select}
            aria-label={t("Concepts")}
            data-testid="concept-list"
            className="pt-0"
          >
            {concepts.map((concept) => (
              <ConceptRow key={concept.id} concept={concept} naming={naming} />
            ))}
          </ListDetailList>
        )}
        {list.hasNextPage && (
          <div className="px-2 pb-3">
            <Button
              variant="ghost"
              size="sm"
              data-slot="concepts-more"
              className="w-full text-muted-foreground"
              disabled={list.isFetchingNextPage}
              onClick={() => void list.fetchNextPage()}
            >
              {t("Show {count} more", { count: Math.min(CONCEPT_PAGE, total - concepts.length) })}
            </Button>
          </div>
        )}
      </>
    );
    body = (
      <div className="min-h-[420px] flex-1 overflow-hidden rounded-lg border border-border">
        <ListDetail
          list={listPane}
          listWidth={320}
          detail={
            selectedId ? (
              <ConceptDetail
                key={selectedId}
                conceptId={selectedId}
                summary={summary}
                source={source}
                naming={naming}
                markets={markets}
                onNavigate={select}
                onGoverned={setGoverned}
              />
            ) : (
              <ListDetailEmpty
                title={t("Choose a concept to see it here")}
                description={t(
                  "Its terms in every language, what it replaces, and where it came from.",
                )}
              />
            )
          }
        />
      </div>
    );
  }

  return (
    <div data-slot="concepts-page" className="flex min-h-0 flex-1 flex-col gap-3">
      {header}
      <PendingConceptChanges onOpenChangeSet={onOpenChangeSet} />
      {body}
      <GovernedEditDialog
        error={governed}
        onClose={() => setGoverned(null)}
        onOpenExperiments={
          onOpenExperiments
            ? () => {
                setGoverned(null);
                onOpenExperiments();
              }
            : undefined
        }
      />
    </div>
  );
}

/** One concept in the list: its term, that term's status, and its languages. */
function ConceptRow({ concept: info, naming }: { concept: ConceptInfo; naming: ConceptNaming }) {
  const concept = conceptSummaryFromInfo(info);
  const headline = headlineTerm(concept, naming);
  const languages = languageCount(concept);
  return (
    <ListDetailItem
      id={concept.id}
      data-testid="concept-row"
      data-slot="concept-row"
      // Rows off screen skip layout and paint, so a workspace of thousands of
      // concepts scrolls as lightly as a short one.
      className="[contain-intrinsic-size:auto_36px] [content-visibility:auto]"
      trailing={
        <span
          className="text-[11px] text-muted-foreground tabular-nums"
          title={t("{count, plural, one {# language} other {# languages}}", { count: languages })}
        >
          {languages}
        </span>
      }
    >
      <span className="font-medium" translate="no">
        {primaryName(concept, naming)}
      </span>
      {headline && <TermStatusChip status={headline.status} className="ml-2" />}
    </ListDetailItem>
  );
}

/**
 * The two empty Concepts pages, which mean opposite things. With nothing
 * waiting, the workspace has no terms yet and the next move is to push some.
 * With a change-set in review, the terms exist and are one approval away.
 */
function ConceptsZeroState({ pendingCount }: { pendingCount: number }) {
  const waiting = pendingCount > 0;
  return (
    <EmptyState
      className={cn("mx-auto w-full max-w-md")}
      icon={waiting ? <Shield className="mx-auto size-5" /> : <Layers className="mx-auto size-5" />}
      title={waiting ? t("Every concept here is waiting on a review") : t("No concepts yet")}
      description={
        waiting
          ? t("Proposed concepts join this list once their change is approved.")
          : t("Concepts arrive when you push terms from a project with kapi push.")
      }
    />
  );
}
