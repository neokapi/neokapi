// How much terminology the workspace holds and how far it spans, on the
// context dashboard (AD-021): concepts by the status of their terms, as
// labelled counts; each language's share of concepts with a term in it; and
// the markets the workspace defines. Each reads a server aggregate rather than
// folding a page of concepts.
import { t } from "@neokapi/i18n-react/runtime";
import {
  LocaleLabel,
  SectionHeading,
  Skeleton,
  StatRow,
  cn,
  type StatItem,
} from "@neokapi/ui-primitives";
import type { TermStatus } from "../../types/brand-graph";
import { useConceptLocaleCoverage, useConceptStatusCounts } from "../../hooks/useConceptsApi";
import { useMarkets } from "../../hooks/useMarketsApi";

// ── Vocabulary by status ──────────────────────────────────────────────────────

const SHOWN_STATUSES: TermStatus[] = ["preferred", "approved", "admitted", "proposed", "forbidden"];

function statusLabel(status: TermStatus): string {
  switch (status) {
    case "preferred":
      return t("preferred", "a term status, as a count's label");
    case "approved":
      return t("approved", "a term status, as a count's label");
    case "admitted":
      return t("admitted", "a term status, as a count's label");
    case "proposed":
      return t("proposed", "a term status, as a count's label");
    case "forbidden":
      return t("forbidden", "a term status, as a count's label");
    default:
      return status;
  }
}

/** Concepts carrying a term at each status, as labelled counts. */
export function VocabularyByStatus() {
  // One count over the whole vocabulary. The buckets overlap (a concept counts
  // under every status one of its terms carries), so they do not sum to total.
  const { data, isLoading } = useConceptStatusCounts();
  const byStatus = data?.by_status ?? {};
  const items: StatItem[] = SHOWN_STATUSES.map((status) => ({
    id: status,
    value: byStatus[status] ?? 0,
    label: statusLabel(status),
    tone: status === "proposed" || status === "forbidden" ? "muted" : "default",
    hint: t(
      "Concepts with a term at this status. A concept counts under each status its terms carry.",
    ),
  }));

  return (
    <section data-slot="dashboard-terms" aria-labelledby="dashboard-terms-heading">
      <div id="dashboard-terms-heading">
        <SectionHeading>Terms by status</SectionHeading>
      </div>
      {isLoading ? (
        <Skeleton className="mt-2 h-10 w-full" />
      ) : (data?.total ?? 0) === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">No concepts yet</p>
      ) : (
        <StatRow className="mt-2 gap-x-6" items={items} />
      )}
    </section>
  );
}

// ── Locale coverage ───────────────────────────────────────────────────────────

/** Each language's share of the workspace's concepts with a term in it, most complete first. */
export function LocaleCoveragePanel({ limit = 8 }: { limit?: number }) {
  const { data, isLoading } = useConceptLocaleCoverage();
  const coverage = (data?.locales ?? []).slice(0, limit);

  return (
    <section data-slot="dashboard-languages" aria-labelledby="dashboard-languages-heading">
      <div id="dashboard-languages-heading">
        <SectionHeading>Languages</SectionHeading>
      </div>
      {isLoading ? (
        <Skeleton className="mt-2 h-24 w-full" />
      ) : coverage.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">No terms in target languages yet</p>
      ) : (
        <ul role="list" className="mt-1.5 divide-y divide-border/50">
          {coverage.map((row) => (
            <li
              key={row.locale}
              data-slot="dashboard-language"
              className="grid h-9 grid-cols-[minmax(0,1fr)_auto_4rem] items-center gap-x-3 px-2"
            >
              <LocaleLabel locale={row.locale} className="truncate text-sm" />
              <span className="text-[11px] text-muted-foreground tabular-nums">
                {row.present}/{row.total} · {row.pct}%
              </span>
              <span aria-hidden className="h-1.5 overflow-hidden rounded-full bg-muted">
                <span
                  className={cn(
                    "block h-full rounded-full",
                    row.pct >= 80 ? "bg-success" : row.pct >= 40 ? "bg-primary" : "bg-warning",
                  )}
                  style={{ width: `${row.pct}%` }}
                />
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

// ── Markets ───────────────────────────────────────────────────────────────────

/** The markets the workspace defines: a name and the languages it covers. */
export function MarketsPanel() {
  const { data: markets, isLoading } = useMarkets();
  const list = markets ?? [];

  return (
    <section data-slot="dashboard-markets" aria-labelledby="dashboard-markets-heading">
      <div id="dashboard-markets-heading">
        <SectionHeading>Markets</SectionHeading>
      </div>
      {isLoading ? (
        <Skeleton className="mt-2 h-24 w-full" />
      ) : list.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">
          No markets defined. A market names a region and its languages, so a term can be valid in
          one and not another.
        </p>
      ) : (
        <ul role="list" className="mt-1.5 divide-y divide-border/50">
          {list.map((market) => (
            <li
              key={market.id}
              data-slot="dashboard-market"
              className="grid h-9 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 px-2"
            >
              <span className="truncate text-sm capitalize" title={market.description || undefined}>
                {market.name}
              </span>
              <span className="text-[11px] text-muted-foreground tabular-nums">
                {t("{count, plural, one {# language} other {# languages}}", {
                  count: market.locales.length,
                })}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
