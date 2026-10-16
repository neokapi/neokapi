// The concepts of a LOCAL terms store, as a list beside a detail pane.
//
// The list has one row per concept: the term it is headed by, that term's
// status, and how many languages the concept has terms in. The search at the
// top of the list filters as you type; the store answers it, a page at a time.
// The detail pane holds the selected concept: its definition, the forms it
// discourages and what to write instead, its terms language by language, where
// it came from, and its relations. Relate, in the pane's toolbar, relates it to
// another concept; selecting a related concept opens that one.
//
// Discussion and observations are interaction surfaces that presuppose other
// people and a shared record. They belong to the platform, whose concept source
// is the only one that supplies them, so this pane has no place for them: the
// local source (`localConceptSource.ts`) supplies terms and relations only.

import { useMemo, useState } from "react";
import { keepPreviousData, useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { BookOpen, Plus, Search } from "lucide-react";
import { t, useNeokapi } from "@neokapi/i18n-react/runtime";
import {
  ConceptNamingProvider,
  TERM_STATUS_CLASS,
  isBannedStatus,
  isPreferredStatus,
  primaryName,
  resolveCapabilities,
  termsByLocale,
  type Concept,
  type ConceptDataSource,
  type ConceptNaming,
  type ConceptSummary,
  type Term,
  type TermStatus,
} from "@neokapi/concept-ui";
import {
  Badge,
  Button,
  CoordinateChip,
  EmptyState,
  ErrorNotice,
  Input,
  ListDetail,
  ListDetailEmpty,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
  LoadingSpinner,
  LocaleLabel,
  When,
  cn,
  useDebounced,
} from "@neokapi/ui-primitives";
import { createLocalConceptSource } from "../lib/localConceptSource";
import { ConceptRelations, RelateDialog } from "./ConceptRelations";

export interface ConceptsViewProps {
  /** The open terms handle this view is bound to. */
  handle: string;
  /**
   * Pre-built source for Storybook/tests (skips the Wails backend). When omitted
   * a local source is created for `handle`.
   */
  source?: ConceptDataSource;
  /** Scope a concept's languages to these locales (the project's Active Filter). */
  localeScope?: string[];
  /**
   * The project's source language. A concept is headed by its term in this
   * locale, so an English-source project reads its own English terms even when
   * the store lists another locale first.
   */
  sourceLocale?: string;
  /** The concept to open on, by id. Absent opens with nothing selected. */
  defaultSelected?: string;
}

/** How many concepts one read of the store carries. */
export const CONCEPT_PAGE = 100;

export function ConceptsView({
  handle,
  source: injected,
  localeScope,
  sourceLocale,
  defaultSelected,
}: ConceptsViewProps) {
  const source = useMemo(() => injected ?? createLocalConceptSource(handle), [injected, handle]);
  const { locale } = useNeokapi();
  const naming = useMemo<ConceptNaming>(
    () => ({ sourceLocale: sourceLocale || undefined, uiLocale: locale }),
    [sourceLocale, locale],
  );

  const [text, setText] = useState("");
  const query = useDebounced(text.trim());
  const list = useInfiniteQuery({
    queryKey: ["terms", "concepts", handle, query],
    queryFn: ({ pageParam }) =>
      Promise.resolve(
        source.listConcepts({ text: query || undefined, offset: pageParam, limit: CONCEPT_PAGE }),
      ),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + p.concepts.length, 0);
      return last.concepts.length > 0 && loaded < last.total ? loaded : undefined;
    },
    // The previous answer stays on screen while the next search is read, so
    // the list does not empty and refill on every keystroke.
    placeholderData: keepPreviousData,
  });
  const concepts = useMemo(() => list.data?.pages.flatMap((p) => p.concepts) ?? [], [list.data]);
  const total = list.data?.pages[0]?.total ?? 0;

  const [selectedId, setSelectedId] = useState<string | null>(defaultSelected ?? null);

  if (list.error && !list.data) {
    return (
      <div className="p-6">
        <ErrorNotice error={list.error} title="The terms could not be read" />
      </div>
    );
  }
  if (list.isLoading) {
    return (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    );
  }
  if (concepts.length === 0 && !query) {
    return (
      <div className="p-6">
        <EmptyState
          icon={<BookOpen size={20} />}
          title="No terms yet"
          description="The words this project has agreed appear here: a term you keep in What's new, or one imported with kapi terms import."
        />
      </div>
    );
  }

  const listPane = (
    <>
      <div className="sticky top-0 z-10 bg-background px-2 pt-2 pb-1">
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
                setSelectedId((id) => id ?? concepts[0].id);
                const box = e.currentTarget
                  .closest("[data-pane='list']")
                  ?.querySelector<HTMLElement>("[role='listbox']");
                box?.focus();
              }
            }}
            placeholder={t("Search terms")}
            aria-label="Search concepts"
            data-slot="terms-search"
            className="h-8 pl-8 text-sm"
          />
        </div>
        {query && (
          <p className="px-1 pt-1.5 text-[11px] text-muted-foreground tabular-nums">
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
          onSelect={setSelectedId}
          aria-label={t("Concepts")}
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

  const summary = concepts.find((c) => c.id === selectedId);
  return (
    <ConceptNamingProvider naming={naming}>
      <div data-slot="terms-browser" className="h-full min-h-0">
        <ListDetail
          list={listPane}
          detail={
            selectedId ? (
              <ConceptDetail
                key={selectedId}
                handle={handle}
                id={selectedId}
                summary={summary}
                source={source}
                naming={naming}
                localeScope={localeScope}
                onNavigate={setSelectedId}
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
    </ConceptNamingProvider>
  );
}

/** The term a concept is headed by, among its terms: what the list shows. */
function headlineTerm(concept: ConceptSummary, naming: ConceptNaming): Term | undefined {
  const name = primaryName(concept, naming);
  return (
    concept.terms.find((term) => term.text === name && !isBannedStatus(term.status)) ??
    concept.terms.find((term) => term.text === name)
  );
}

/** How many languages a concept has terms in. */
function languageCount(concept: ConceptSummary): number {
  return new Set(concept.terms.map((term) => term.locale)).size;
}

/** One concept in the list: its term, that term's status, and its languages. */
function ConceptRow({ concept, naming }: { concept: ConceptSummary; naming: ConceptNaming }) {
  const headline = headlineTerm(concept, naming);
  const languages = languageCount(concept);
  return (
    <ListDetailItem
      id={concept.id}
      data-slot="terms-concept"
      // Rows off screen skip layout and paint, so a store of thousands of
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

/** A term's status, by its word, on the tint the concept UI gives it. */
function TermStatusChip({ status, className }: { status: TermStatus; className?: string }) {
  return (
    <Badge
      variant="outline"
      data-slot="term-status"
      data-status={status}
      className={cn("h-4 px-1.5 text-[11px]", TERM_STATUS_CLASS[status], className)}
    >
      {termStatusLabel(status)}
    </Badge>
  );
}

/** A term status in the active UI language. */
function termStatusLabel(status: TermStatus): string {
  switch (status) {
    case "preferred":
      return t("Preferred", "term status");
    case "approved":
      return t("Approved", "term status");
    case "admitted":
      return t("Admitted", "term status");
    case "proposed":
      return t("Proposed", "term status");
    case "deprecated":
      return t("Deprecated", "term status");
    case "forbidden":
      return t("Forbidden", "term status");
    default:
      return status;
  }
}

/** What to write instead of a discouraged term: the preferred term in its language. */
function replacementFor(term: Term, terms: Term[]): Term | undefined {
  const sameLocale = terms.filter((other) => other.locale === term.locale);
  return (
    sameLocale.find((other) => isPreferredStatus(other.status)) ??
    sameLocale.find((other) => !isBannedStatus(other.status))
  );
}

/** The axes a concept's `coordinates` property names, `axis=value` pairs joined by commas. */
function coordinatesOf(properties?: Record<string, string>): Array<[string, string]> {
  const text = properties?.coordinates ?? "";
  const out: Array<[string, string]> = [];
  for (const part of text.split(",")) {
    const cut = part.indexOf("=");
    if (cut <= 0) continue;
    const axis = part.slice(0, cut).trim();
    const value = part.slice(cut + 1).trim();
    if (axis && value) out.push([axis, value]);
  }
  return out;
}

interface ConceptDetailProps {
  handle: string;
  id: string;
  /** The concept as the list read it, shown while the full record is read. */
  summary?: ConceptSummary;
  source: ConceptDataSource;
  naming: ConceptNaming;
  localeScope?: string[];
  onNavigate: (id: string) => void;
}

/** The selected concept: its terms, what it replaces, where it came from, its relations. */
function ConceptDetail({
  handle,
  id,
  summary,
  source,
  naming,
  localeScope,
  onNavigate,
}: ConceptDetailProps) {
  const full = useQuery({
    queryKey: ["terms", "concept", handle, id],
    queryFn: () => Promise.resolve(source.getConcept(id)),
  });
  const caps = useMemo(() => resolveCapabilities(source), [source]);
  const [relating, setRelating] = useState(false);
  // The list's copy stands in while the full record is read; a read that
  // finds nothing means the concept left the store.
  const concept: Concept | null | undefined = full.data === null ? null : (full.data ?? summary);

  if (full.error && !concept) {
    return (
      <div className="p-6">
        <ErrorNotice error={full.error} title="This concept could not be read" />
      </div>
    );
  }
  if (!concept) {
    return full.isLoading ? (
      <div className="p-6">
        <LoadingSpinner />
      </div>
    ) : (
      <ListDetailEmpty
        title={t("This concept is no longer in the store")}
        description={t("It may have been deleted or merged.")}
      />
    );
  }

  const name = primaryName(concept, naming);
  const headline = headlineTerm(concept, naming);
  const discouraged = concept.terms.filter((term) => isBannedStatus(term.status));
  const scope = localeScope?.length ? new Set(localeScope) : null;
  const languages = termsByLocale(concept.terms.filter((term) => !isBannedStatus(term.status)));
  // The source language first, then the rest in the order the store keeps.
  const ordered = [
    ...languages.filter((g) => g.locale === naming.sourceLocale),
    ...languages.filter((g) => g.locale !== naming.sourceLocale),
  ];
  const shown = scope
    ? ordered.filter((g) => scope.has(g.locale) || g.locale === naming.sourceLocale)
    : ordered;
  const hidden = ordered.length - shown.length;
  const coordinates = coordinatesOf(concept.properties);
  const profile = concept.properties?.profile;

  return (
    <ListDetailPane
      data-slot="terms-concept-detail"
      data-concept={concept.id}
      toolbar={
        <ListDetailToolbar
          title={
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate" translate="no">
                {name}
              </span>
              {headline && <TermStatusChip status={headline.status} />}
            </span>
          }
          actions={
            caps.editRelations && (
              <Button
                variant="outline"
                size="sm"
                data-slot="terms-relate"
                onClick={() => setRelating(true)}
              >
                <Plus size={13} />
                Relate
              </Button>
            )
          }
        />
      }
    >
      {concept.definition && (
        <p className="max-w-prose text-sm" translate="no">
          {concept.definition}
        </p>
      )}

      {discouraged.length > 0 && (
        <section data-slot="terms-discouraged" aria-label={t("Discouraged forms")}>
          <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Write instead</h3>
          <ul className="space-y-1">
            {discouraged.map((term) => {
              const instead = replacementFor(term, concept.terms);
              return (
                <li
                  key={`${term.locale}:${term.text}`}
                  className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm"
                >
                  <span className="line-through decoration-muted-foreground" translate="no">
                    {term.text}
                  </span>
                  {instead && (
                    <span className="text-muted-foreground">
                      use{" "}
                      <span className="text-foreground" translate="no">
                        {instead.text}
                      </span>
                    </span>
                  )}
                  <TermStatusChip status={term.status} />
                  <LocaleLabel locale={term.locale} compact className="text-[11px]" />
                  {term.note && (
                    <span className="basis-full text-xs text-muted-foreground" translate="no">
                      {term.note}
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        </section>
      )}

      <section data-slot="terms-translations" aria-label={t("Terms by language")}>
        <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">By language</h3>
        <dl className="grid grid-cols-[minmax(7rem,max-content)_minmax(0,1fr)] gap-x-4 gap-y-1.5">
          {shown.map((group) => (
            <div key={group.locale} className="contents">
              <dt className="pt-0.5 text-xs">
                <LocaleLabel
                  locale={group.locale}
                  source={group.locale === naming.sourceLocale}
                  variant="short"
                />
              </dt>
              <dd className="min-w-0 space-y-1">
                {group.terms.map((term) => (
                  <TermLine key={`${term.locale}:${term.text}`} term={term} />
                ))}
              </dd>
            </div>
          ))}
        </dl>
        {hidden > 0 && (
          <p className="mt-1.5 text-[11px] text-muted-foreground">
            {t(
              "{count, plural, one {# more language is outside the filter} other {# more languages are outside the filter}}",
              { count: hidden },
            )}
          </p>
        )}
      </section>

      <section data-slot="terms-provenance" aria-label={t("Where it came from")}>
        <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Where it came from</h3>
        <dl className="grid grid-cols-[minmax(7rem,max-content)_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm">
          {concept.source && (
            <Fact label={<>Source</>}>
              {concept.source === "brand_vocabulary" ? <>Brand vocabulary</> : <>Terminology</>}
            </Fact>
          )}
          {concept.domain && (
            <Fact label={<>Domain</>}>
              <span translate="no">{concept.domain}</span>
            </Fact>
          )}
          {profile && (
            <Fact label={<>Voice profile</>}>
              <span className="font-mono text-xs" translate="no">
                {profile}
              </span>
            </Fact>
          )}
          {coordinates.length > 0 && (
            <Fact label={<>Applies at</>}>
              <span className="flex flex-wrap gap-1">
                {coordinates.map(([axis, value]) => (
                  <CoordinateChip key={axis} axis={axis} value={value} />
                ))}
              </span>
            </Fact>
          )}
          {concept.createdAt && (
            <Fact label={<>Added</>}>
              <When iso={concept.createdAt} />
            </Fact>
          )}
          {concept.updatedAt && concept.updatedAt !== concept.createdAt && (
            <Fact label={<>Changed</>}>
              <When iso={concept.updatedAt} relative />
            </Fact>
          )}
        </dl>
      </section>

      <ConceptRelations
        handle={handle}
        concept={concept}
        source={source}
        naming={naming}
        onNavigate={onNavigate}
        canEdit={caps.editRelations}
      />
      {caps.editRelations && (
        <RelateDialog
          handle={handle}
          concept={concept}
          source={source}
          naming={naming}
          open={relating}
          onOpenChange={setRelating}
        />
      )}
    </ListDetailPane>
  );
}

/** One term in its language: the text, its status, its other forms and when it holds. */
function TermLine({ term }: { term: Term }) {
  const validity = term.validity;
  const tags = Object.entries(validity?.tags ?? {});
  return (
    <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm">
      <span translate="no">{term.text}</span>
      <TermStatusChip status={term.status} />
      {term.forms && term.forms.length > 0 && (
        <span className="text-xs text-muted-foreground">
          also <span translate="no">{term.forms.join(", ")}</span>
        </span>
      )}
      {(term.partOfSpeech || term.gender) && (
        <span className="text-[11px] text-muted-foreground" translate="no">
          {[term.partOfSpeech, term.gender].filter(Boolean).join(", ")}
        </span>
      )}
      {validity?.validFrom && (
        <span className="text-[11px] text-muted-foreground">
          from <When iso={validity.validFrom} />
        </span>
      )}
      {validity?.validTo && (
        <span className="text-[11px] text-muted-foreground">
          until <When iso={validity.validTo} />
        </span>
      )}
      {tags.map(([key, value]) => (
        <Badge key={key} variant="outline" className="h-4 px-1.5 text-[11px]" translate="no">
          {key}: {value}
        </Badge>
      ))}
      {term.note && (
        <span className="basis-full text-xs text-muted-foreground" translate="no">
          {term.note}
        </span>
      )}
    </div>
  );
}

function Fact({ label, children }: { label: React.ReactNode; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-[11px] text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </>
  );
}
