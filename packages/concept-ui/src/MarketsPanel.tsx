// MarketsPanel — the geography axis (Apache-2.0). One panel per market/region
// showing, for THIS concept, the term and status used there — with banned and
// preferred wording made obvious and colour-coded. When the source supplies
// named markets (DACH, France, US…) each becomes a panel; in framework-only mode
// the panels are derived from the terms' `market` validity tags, with any
// untagged locales gathered into a trailing "Other locales" panel. The goal is
// that "where is this banned or preferred" reads at a glance.
import { useMemo } from "react";
import { Badge, Skeleton, cn } from "@neokapi/ui-primitives";
import { Ban, Globe, MapPin, Star } from "lucide-react";
import type { ConceptSectionProps } from "./ConceptView";
import type { Market, Term } from "./types";
import { isBannedStatus } from "./concept-meta";
import { deriveMarketsFromTerms } from "./grouping";
import { buildMarketView } from "./markets-view";
import type { MarketLocaleView, MarketView } from "./markets-view";
import { ConceptSection, EmptyHint, ErrorHint, LocalePill, StatusChip } from "./atoms";
import { useResource } from "./useResource";

export function MarketsPanel({ concept, source, capabilities }: ConceptSectionProps) {
  const markets = useResource<Market[]>(
    () => (capabilities.markets && source.getMarkets ? source.getMarkets() : []),
    [source, concept.id, capabilities.markets],
  );

  const named = (markets.data?.length ?? 0) > 0;
  const effective = useMemo(
    () => (named ? markets.data! : deriveMarketsFromTerms(concept.terms)),
    [named, markets.data, concept.terms],
  );
  const views = useMemo(
    () => buildMarketView(concept.terms, effective),
    [concept.terms, effective],
  );

  const anyBanned = views.some((v) => v.hasBanned);
  const anyPreferred = views.some((v) => v.hasPreferred);
  const loading = capabilities.markets && markets.loading && !markets.data;

  return (
    <ConceptSection
      title="Geography"
      icon={<Globe />}
      description="Markets and the term and status used in each."
      actions={
        named ? undefined : (
          <Badge variant="outline" className="gap-1 font-normal text-[10px] text-muted-foreground">
            <MapPin className="size-3" />
            from tags
          </Badge>
        )
      }
    >
      {markets.error ? (
        <ErrorHint title="Could not load markets" description={markets.error.message} />
      ) : loading ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <Skeleton className="h-28 w-full rounded-lg" />
          <Skeleton className="h-28 w-full rounded-lg" />
        </div>
      ) : views.length === 0 ? (
        <EmptyHint
          icon={<Globe />}
          title="No regional wording"
          description="This concept uses the same term everywhere."
        />
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2">
            {views.map((view) => (
              <MarketCard key={view.name} view={view} />
            ))}
          </div>
          {(anyBanned || anyPreferred) && (
            <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
              {anyPreferred && (
                <LegendItem icon={<Star className="size-3 fill-success text-success" />}>
                  preferred wording
                </LegendItem>
              )}
              {anyBanned && (
                <LegendItem icon={<Ban className="size-3 text-destructive" />}>
                  banned — do not use
                </LegendItem>
              )}
            </div>
          )}
        </>
      )}
    </ConceptSection>
  );
}

// ── Market card ──────────────────────────────────────────────────────────────

function MarketCard({ view }: { view: MarketView }) {
  // A left accent makes a panel's state legible before reading any row:
  // destructive where a term is banned, success where one is preferred.
  const accent = view.hasBanned
    ? "border-l-destructive/60"
    : view.hasPreferred
      ? "border-l-success/60"
      : "border-l-border";

  return (
    <div className={cn("rounded-lg border border-l-2 bg-muted/20 p-3", accent)}>
      <div className="mb-2 flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">{view.name}</p>
          {view.description && (
            <p className="truncate text-[11px] text-muted-foreground">{view.description}</p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1.5 pt-0.5">
          {view.hasPreferred && <Star className="size-3.5 fill-success text-success" />}
          {view.hasBanned && <Ban className="size-3.5 text-destructive" />}
          <span className="text-[11px] tabular-nums text-muted-foreground">{view.localeCount}</span>
        </div>
      </div>
      <ul className="space-y-1.5">
        {view.locales.map((locale) => (
          <LocaleBlock key={locale.locale} locale={locale} />
        ))}
      </ul>
    </div>
  );
}

function LocaleBlock({ locale }: { locale: MarketLocaleView }) {
  return (
    <li className="space-y-1">
      {locale.terms.map((term, i) => (
        <TermRow
          key={`${term.text}-${i}`}
          term={term}
          locale={i === 0 ? locale.locale : undefined}
        />
      ))}
    </li>
  );
}

function TermRow({ term, locale }: { term: Term; locale?: string }) {
  const banned = isBannedStatus(term.status);
  return (
    <div className="flex items-center gap-2 text-sm">
      {locale ? (
        <LocalePill locale={locale} />
      ) : (
        <span className="inline-block w-[2.75rem] shrink-0" aria-hidden />
      )}
      <span
        className={cn(
          "min-w-0 flex-1 truncate",
          banned ? "text-muted-foreground line-through" : "text-foreground",
          term.status === "preferred" && "font-medium",
        )}
        title={term.note}
      >
        {term.text}
      </span>
      <StatusChip status={term.status} className="text-[10px]" />
    </div>
  );
}

function LegendItem({ icon, children }: { icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1">
      {icon}
      {children}
    </span>
  );
}
