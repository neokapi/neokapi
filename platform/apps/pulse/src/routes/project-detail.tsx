import { useParams, Link } from "@tanstack/react-router";
import { LanguageProgressGrid, CompletionRing } from "@neokapi/ui/components/pulse";
import { usePulseProjectDetail } from "../hooks/use-pulse";
import { ArrowLeft } from "lucide-react";

export function ProjectDetailPage() {
  const { workspace, pid } = useParams({ strict: false }) as { workspace: string; pid: string };
  const { data, isLoading, error } = usePulseProjectDetail(workspace, pid);

  if (isLoading) {
    return <div className="h-64 animate-pulse rounded-lg border bg-muted" />;
  }

  if (error || !data) {
    return (
      <div className="flex min-h-[400px] items-center justify-center text-muted-foreground">
        Project not found or not public.
      </div>
    );
  }

  const { project, locales } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link to={`/${workspace}`} className="rounded p-1 hover:bg-muted">
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <div className="flex-1">
          <h1 className="text-xl font-bold">{project.name}</h1>
          <p className="text-sm text-muted-foreground">
            {project.source_language} → {project.target_languages.join(", ")}
          </p>
        </div>
        <CompletionRing percentage={project.percentage} size={64} />
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard label="Total Words" value={project.total_words.toLocaleString()} />
        <StatCard label="Translated" value={project.translated_words.toLocaleString()} />
        <StatCard label="Progress" value={`${project.percentage.toFixed(1)}%`} />
      </div>

      <section>
        <h2 className="mb-4 text-lg font-semibold">Languages</h2>
        <LanguageProgressGrid
          languages={locales.map((l) => ({
            locale: l.locale,
            translated_words: l.translated_words,
            total_words: l.total_words,
            percentage: l.percentage,
          }))}
        />
      </section>
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="mt-1 text-2xl font-bold">{value}</div>
    </div>
  );
}
