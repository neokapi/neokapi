import type { TranslationDashboardStats } from "../types/api";
import { Card, CardContent } from "./ui/card";
import { cn } from "../lib/utils";
import { Globe, FileText, Languages, BarChart3 } from "./icons";
import { LocaleCompletionChart } from "./LocaleCompletionChart";
import { WordCountChart } from "./WordCountChart";
import { CollectionHeatmap } from "./CollectionHeatmap";
import { FileProgressTable } from "./FileProgressTable";

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

export function TranslationDashboard({ stats, projectName, className }: TranslationDashboardProps) {
  if (!stats) {
    return (
      <div data-testid="translation-dashboard" className={cn("space-y-6", className)}>
        <h1 className="text-lg font-semibold">Translation Dashboard</h1>
        <Card className="p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No translation data yet. Upload files and add translations to see progress here.
          </p>
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

  return (
    <div data-testid="translation-dashboard" className={cn("space-y-6", className)}>
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">
          {projectName ? `${projectName} — Translation Dashboard` : "Translation Dashboard"}
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
        <StatCard label="Target Languages" value={String(stats.locale_stats.length)} icon={Globe} />
        <StatCard label="Overall Completion" value={`${overallPct}%`} icon={Languages} />
      </div>

      {/* Charts Row */}
      {stats.locale_stats.length > 0 && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <LocaleCompletionChart localeStats={stats.locale_stats} />
          <WordCountChart localeStats={stats.locale_stats} />
        </div>
      )}

      {/* Collection Heatmap */}
      {stats.collection_stats.length > 0 && (
        <CollectionHeatmap
          collectionStats={stats.collection_stats}
          locales={stats.locale_stats.map((l) => l.locale)}
        />
      )}

      {/* File Progress Table */}
      {stats.item_stats.length > 0 && (
        <FileProgressTable
          itemStats={stats.item_stats}
          locales={stats.locale_stats.map((l) => l.locale)}
          localeDisplayNames={Object.fromEntries(
            stats.locale_stats
              .filter((l) => l.display_name)
              .map((l) => [l.locale, l.display_name!]),
          )}
        />
      )}
    </div>
  );
}
