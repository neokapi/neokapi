import { ActivityHeatmap } from "./ActivityHeatmap";
import { LanguageProgressGrid } from "./LanguageProgressGrid";
import { PulseProjectCard } from "./PulseProjectCard";

interface PulseOverviewProps {
  stats: {
    total_projects: number;
    total_languages: number;
    total_contributors: number;
    total_words: number;
    translated_words: number;
    overall_percent: number;
  };
  projects: {
    id: string;
    name: string;
    source_language: string;
    source_language_display_name?: string;
    target_languages: string[];
    target_language_names?: Record<string, string>;
    total_words: number;
    translated_words: number;
    percentage: number;
  }[];
  languages: {
    locale: string;
    display_name?: string;
    translated_words: number;
    total_words: number;
    percentage: number;
  }[];
  heatmap?: { date: string; count: number }[];
  onProjectClick?: (id: string) => void;
}

export function PulseOverview({
  stats,
  projects,
  languages,
  heatmap,
  onProjectClick,
}: PulseOverviewProps) {
  return (
    <div className="space-y-8">
      {/* Stats Cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label="Projects" value={stats.total_projects} />
        <StatCard label="Languages" value={stats.total_languages} />
        <StatCard label="Overall Progress" value={`${stats.overall_percent.toFixed(1)}%`} />
        <StatCard
          label="Words"
          value={`${stats.translated_words.toLocaleString()} / ${stats.total_words.toLocaleString()}`}
        />
      </div>

      {/* Activity Heatmap */}
      {heatmap && (
        <section>
          <h2 className="mb-4 text-lg font-semibold">Activity</h2>
          <div className="rounded-lg border bg-card p-4">
            <ActivityHeatmap days={heatmap} />
          </div>
        </section>
      )}

      {/* Projects Grid */}
      <section>
        <h2 className="mb-4 text-lg font-semibold">Projects</h2>
        {projects.length === 0 ? (
          <div className="rounded-lg border bg-card p-8 text-center text-muted-foreground">
            No public projects yet.
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {projects.map((p) => (
              <PulseProjectCard
                key={p.id}
                name={p.name}
                sourceLanguage={p.source_language_display_name ?? p.source_language}
                targetLanguages={p.target_languages.map((t) => p.target_language_names?.[t] ?? t)}
                totalWords={p.total_words}
                translatedWords={p.translated_words}
                percentage={p.percentage}
                onClick={() => onProjectClick?.(p.id)}
              />
            ))}
          </div>
        )}
      </section>

      {/* Language Progress */}
      <section>
        <h2 className="mb-4 text-lg font-semibold">Language Progress</h2>
        <LanguageProgressGrid languages={languages} />
      </section>
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="mt-1 text-2xl font-bold">{value}</div>
    </div>
  );
}
