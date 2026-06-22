// ConceptList — the entry surface of the concept UI (Apache-2.0). A searchable,
// filterable list of concepts; each row reads as "what we call this, per locale"
// with a status chip. Clicking a row opens that concept's view. There is no
// global graph here — the graph is a local widget inside the concept view.
import { useEffect, useMemo, useState } from "react";
import {
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
  Badge,
  cn,
} from "@neokapi/ui-primitives";
import { Search, Library } from "lucide-react";
import type { ConceptDataSource, ConceptQuery } from "./adapter";
import { resolveCapabilities } from "./adapter";
import type { ConceptSummary, Market, TermSource, TermStatus } from "./types";
import { TERM_STATUSES } from "./types";
import { primaryName, TERM_STATUS_LABEL } from "./concept-meta";
import { termsByLocale } from "./grouping";
import { StatusChip, LocalePill, EmptyHint } from "./atoms";
import { useResource } from "./useResource";
import { useDebounced } from "./useDebounced";

const ALL = "all";

const SOURCE_LABEL: Record<TermSource, string> = {
  terminology: "Terminology",
  brand_vocabulary: "Brand vocabulary",
};

export interface ConceptListProps {
  source: ConceptDataSource;
  /** Open a concept's view. */
  onOpen: (conceptId: string) => void;
  /** Optional starting filters. */
  initialQuery?: ConceptQuery;
  /** Max locale chips shown per row before a "+N more". */
  maxLocaleChips?: number;
  /** When set, scope each row's locale chips to these locales (Active Filter). */
  localeScope?: string[];
  className?: string;
}

export function ConceptList({
  source,
  onOpen,
  initialQuery,
  maxLocaleChips = 4,
  localeScope,
  className,
}: ConceptListProps) {
  const caps = useMemo(() => resolveCapabilities(source), [source]);

  const [text, setText] = useState(initialQuery?.text ?? "");
  const [status, setStatus] = useState<string>(initialQuery?.status ?? ALL);
  const [src, setSrc] = useState<string>(initialQuery?.source ?? ALL);
  const [domain, setDomain] = useState<string>(initialQuery?.domain ?? ALL);
  const [market, setMarket] = useState<string>(initialQuery?.market ?? ALL);
  const debouncedText = useDebounced(text.trim());

  const query = useMemo<ConceptQuery>(() => {
    const q: ConceptQuery = {};
    if (debouncedText) q.text = debouncedText;
    if (status !== ALL) q.status = status as TermStatus;
    if (src !== ALL) q.source = src as TermSource;
    if (domain !== ALL) q.domain = domain;
    if (market !== ALL) q.market = market;
    return q;
  }, [debouncedText, status, src, domain, market]);

  const result = useResource(() => source.listConcepts(query), [source, query]);
  const markets = useResource<Market[]>(
    () => (caps.markets && source.getMarkets ? source.getMarkets() : []),
    [source, caps.markets],
  );

  // Stable domain facet: accumulate every domain seen so the dropdown never
  // collapses to just the active filter.
  const [domains, setDomains] = useState<string[]>([]);
  const concepts = result.data?.concepts ?? [];
  useEffect(() => {
    if (!result.data) return;
    setDomains((prev) => {
      const next = new Set(prev);
      for (const c of result.data!.concepts) if (c.domain) next.add(c.domain);
      return next.size === prev.length ? prev : [...next].sort();
    });
  }, [result.data]);

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Search concepts and terms…"
            className="pl-8"
            aria-label="Search concepts"
          />
        </div>

        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger className="w-36" size="sm" aria-label="Filter by status">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All statuses</SelectItem>
            {TERM_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {TERM_STATUS_LABEL[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {domains.length > 0 && (
          <Select value={domain} onValueChange={setDomain}>
            <SelectTrigger className="w-40" size="sm" aria-label="Filter by domain">
              <SelectValue placeholder="Domain" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All domains</SelectItem>
              {domains.map((d) => (
                <SelectItem key={d} value={d}>
                  {d}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        <Select value={src} onValueChange={setSrc}>
          <SelectTrigger className="w-40" size="sm" aria-label="Filter by source">
            <SelectValue placeholder="Source" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All sources</SelectItem>
            <SelectItem value="terminology">{SOURCE_LABEL.terminology}</SelectItem>
            <SelectItem value="brand_vocabulary">{SOURCE_LABEL.brand_vocabulary}</SelectItem>
          </SelectContent>
        </Select>

        {caps.markets && (markets.data?.length ?? 0) > 0 && (
          <Select value={market} onValueChange={setMarket}>
            <SelectTrigger className="w-36" size="sm" aria-label="Filter by market">
              <SelectValue placeholder="Market" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All markets</SelectItem>
              {markets.data!.map((m) => (
                <SelectItem key={m.id ?? m.name} value={m.name}>
                  {m.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      {result.loading && !result.data ? (
        <ConceptListSkeleton />
      ) : concepts.length === 0 ? (
        <EmptyHint
          icon={<Library />}
          title="No concepts match"
          description="Adjust the search or filters to find a concept."
        />
      ) : (
        <ul className="divide-y overflow-hidden rounded-xl border bg-card">
          {concepts.map((c) => (
            <ConceptRow
              key={c.id}
              concept={c}
              maxLocaleChips={maxLocaleChips}
              localeScope={localeScope}
              onOpen={() => onOpen(c.id)}
            />
          ))}
        </ul>
      )}

      {result.data && (
        <p className="text-xs text-muted-foreground">
          {result.data.total} concept{result.data.total === 1 ? "" : "s"}
        </p>
      )}
    </div>
  );
}

function ConceptRow({
  concept,
  maxLocaleChips,
  localeScope,
  onOpen,
}: {
  concept: ConceptSummary;
  maxLocaleChips: number;
  localeScope?: string[];
  onOpen: () => void;
}) {
  const scopeKey = localeScope?.join(",") ?? "";
  const byLocale = useMemo(() => {
    const all = termsByLocale(concept.terms);
    if (!scopeKey) return all;
    const set = new Set(scopeKey.split(","));
    return all.filter((g) => set.has(g.locale));
  }, [concept.terms, scopeKey]);
  const name = primaryName(concept);

  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className="flex w-full items-start gap-4 px-4 py-3 text-left transition-colors hover:bg-muted/40 focus-visible:bg-muted/40 focus-visible:outline-none"
      >
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium text-foreground">{name}</span>
            {concept.domain && (
              <Badge variant="outline" className="shrink-0 text-[10px] font-normal">
                {concept.domain}
              </Badge>
            )}
          </div>
          {concept.definition && (
            <p className="line-clamp-1 text-sm text-muted-foreground">{concept.definition}</p>
          )}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pt-0.5">
            {byLocale.slice(0, maxLocaleChips).map(({ locale, terms }) => (
              <span key={locale} className="flex items-center gap-1.5 text-xs">
                <LocalePill locale={locale} />
                <span className="text-foreground">{terms[0].text}</span>
                <StatusChip status={terms[0].status} className="text-[10px]" />
              </span>
            ))}
            {byLocale.length > maxLocaleChips && (
              <span className="text-xs text-muted-foreground">
                +{byLocale.length - maxLocaleChips} more
              </span>
            )}
          </div>
        </div>
        <span className="shrink-0 pt-0.5 text-xs text-muted-foreground">
          {concept.terms.length} term{concept.terms.length === 1 ? "" : "s"}
        </span>
      </button>
    </li>
  );
}

function ConceptListSkeleton() {
  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      {Array.from({ length: 6 }).map((_, i) => (
        <div key={i} className="flex items-center gap-4 border-b px-4 py-3.5 last:border-b-0">
          <div className="flex-1 space-y-2">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-72" />
          </div>
          <Skeleton className="h-4 w-12" />
        </div>
      ))}
    </div>
  );
}
