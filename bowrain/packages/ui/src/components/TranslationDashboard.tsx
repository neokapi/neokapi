import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  cn,
  LocaleLabel,
} from "@neokapi/ui-primitives";
import type { LocaleTranslationStats, TranslationDashboardStats } from "../types/api";
import { Globe, FileText, Languages, BarChart3 } from "./icons";

import { LocaleCompletionChart } from "./LocaleCompletionChart";
import { ComplianceRateChip } from "./ComplianceRateChip";
import { ShipStateBadge } from "./ShipStateBadge";
import { WordCountChart } from "./WordCountChart";
import { CollectionOverview, type CollectionScope } from "./collections/CollectionOverview";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function compactNumber(n: number): string {
  if (n < 1000) return String(n);
  if (n < 10_000) return `${(n / 1000).toFixed(1)}k`;
  if (n < 1_000_000) return `${Math.round(n / 1000)}k`;
  return `${(n / 1_000_000).toFixed(1)}M`;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface TranslationDashboardProps {
  stats: TranslationDashboardStats | null;
  projectName?: string;
  className?: string;
  /**
   * Optional delivery panel (DeliveryPanel), rendered next to the per-locale
   * ship-readiness list. The route composes it so this component stays free of
   * data fetching.
   */
  delivery?: React.ReactNode;
  /** Open one collection's items — the overview's second level. */
  onOpenCollection?: (scope: CollectionScope) => void;
  /** Open the project's item list unscoped. */
  onOpenAllItems?: () => void;
  /** Open review with one collection pre-applied. */
  onReviewCollection?: (scope: CollectionScope) => void;
  /** The axis collections are grouped by; undefined takes the derived default. */
  groupingAxis?: string;
  onGroupingAxisChange?: (axis: string) => void;
  /**
   * Open the project's source view, where files are added and collections are
   * created. A project with nothing in it has no overview to show, so this is
   * the one action it offers.
   */
  onAddContent?: () => void;
}

// ---------------------------------------------------------------------------
// Ship readiness (per-locale ship states)
// ---------------------------------------------------------------------------

function ShipReadinessCard({ localeStats }: { localeStats: LocaleTranslationStats[] }) {
  return (
    <Card data-testid="ship-readiness" className="gap-3">
      <CardHeader className="pb-0">
        <CardTitle className="text-sm">Ship readiness</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-1.5">
          {localeStats.map((l) => (
            <li key={l.locale} className="flex items-center justify-between gap-2 text-sm">
              <span className="flex min-w-0 items-center gap-2">
                <LocaleLabel locale={l.locale} displayName={l.display_name} hideCode />
                <span className="text-muted-foreground text-xs tabular-nums">
                  {l.translated_blocks}/{l.total_blocks} blocks
                </span>
              </span>
              <span className="flex shrink-0 items-center gap-1.5">
                {l.compliance_rate !== undefined && l.compliance_basis && (
                  <ComplianceRateChip
                    rate={l.compliance_rate}
                    basis={l.compliance_basis}
                    compliantBlocks={l.compliant_blocks}
                    translatedBlocks={l.translated_blocks}
                  />
                )}
                {l.ship_state && (
                  <ShipStateBadge
                    state={l.ship_state}
                    approvedBlocks={l.approved_blocks}
                    totalBlocks={l.total_blocks}
                    failingChecks={l.failing_checks}
                  />
                )}
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Summary Cards
// ---------------------------------------------------------------------------

interface StatCardProps {
  label: string;
  value: string;
  icon: React.ComponentType<{ className?: string }>;
}

function StatCard({ label, value, icon: Icon }: StatCardProps) {
  return (
    <Card size="sm">
      <CardContent className="flex items-center gap-3 pt-0">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10">
          <Icon className="h-4 w-4 text-primary" />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">{label}</p>
          <p className="text-lg font-semibold tabular-nums">{value}</p>
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Main Component
// ---------------------------------------------------------------------------

/**
 * TranslationDashboard is a project's overview: what the project's content is,
 * grouped by where each collection sits in context space, and how far each
 * language has come in it.
 *
 * It is the first of two levels. The per-item list is the second — reached from
 * a collection card or from the all-items entry, and composed by the route
 * (CollectionItemsView), so a project holding a thousand files never opens on
 * a thousand file paths.
 */
export function TranslationDashboard({
  stats,
  projectName,
  className,
  delivery,
  onOpenCollection,
  onOpenAllItems,
  onReviewCollection,
  groupingAxis,
  onGroupingAxisChange,
  onAddContent,
}: TranslationDashboardProps) {
  // Nothing to report on: no stats at all, or a project whose content has yet
  // to arrive. Both render the same invitation rather than a grid of zeroes —
  // this is the surface a project opens on, so it has to say what to do next.
  if (!stats || (stats.translatable_blocks === 0 && stats.collection_stats.length === 0)) {
    return (
      <div data-testid="translation-dashboard" className={cn("space-y-6", className)}>
        <h1 className="text-lg font-semibold">
          {projectName ? `${projectName} · Overview` : "Overview"}
        </h1>
        <Card className="flex flex-col items-center gap-4 p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No content yet. Add files or connect a collection to see it here.
          </p>
          {onAddContent && (
            <Button onClick={onAddContent} data-testid="dashboard-add-content">
              Add content
            </Button>
          )}
        </Card>
      </div>
    );
  }

  // Compute overall completion weighted by words
  const totalWordsByLocale = stats.locale_stats.reduce((acc, l) => acc + l.total_words, 0);
  const translatedWordsByLocale = stats.locale_stats.reduce(
    (acc, l) => acc + l.translated_words,
    0,
  );
  const overallPct =
    totalWordsByLocale > 0 ? Math.round((translatedWordsByLocale / totalWordsByLocale) * 100) : 0;

  // The ship-readiness band renders once the server derives ship states
  // (older servers omit the field, keeping the legacy layout unchanged).
  const hasShipStates = stats.locale_stats.some((l) => l.ship_state);

  return (
    <div data-testid="translation-dashboard" className={cn("space-y-6", className)}>
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">
          {projectName ? `${projectName} · Overview` : "Overview"}
        </h1>
        <span className="text-sm text-muted-foreground">{overallPct}% complete</span>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <StatCard
          label="Source Words"
          value={compactNumber(stats.total_source_words)}
          icon={FileText}
        />
        <StatCard
          label="Translatable Blocks"
          value={compactNumber(stats.translatable_blocks)}
          icon={BarChart3}
        />
        <StatCard label="Translations" value={String(stats.locale_stats.length)} icon={Globe} />
        <StatCard label="Overall Completion" value={`${overallPct}%`} icon={Languages} />
      </div>

      {/* The content itself, before any report about it: which collections the
          project holds, where each sits, and how each language stands in it. */}
      {stats.collection_stats.length > 0 && onOpenCollection && (
        <CollectionOverview
          collections={stats.collection_stats}
          groupingAxis={groupingAxis}
          onGroupingAxisChange={onGroupingAxisChange}
          onOpenCollection={onOpenCollection}
          onOpenAllItems={onOpenAllItems}
          onReviewCollection={onReviewCollection}
          itemTotal={stats.item_total ?? stats.item_stats.length}
        />
      )}

      {/* Ship readiness + delivery */}
      {(hasShipStates || delivery) && (
        <div
          className={cn("grid grid-cols-1 gap-4", delivery && hasShipStates && "lg:grid-cols-2")}
        >
          {hasShipStates && <ShipReadinessCard localeStats={stats.locale_stats} />}
          {delivery}
        </div>
      )}

      {/* Charts Row */}
      {stats.locale_stats.length > 0 && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <LocaleCompletionChart localeStats={stats.locale_stats} />
          <WordCountChart localeStats={stats.locale_stats} />
        </div>
      )}
    </div>
  );
}
