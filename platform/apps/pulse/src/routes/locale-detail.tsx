import { useParams, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { CompletionRing } from "@neokapi/ui/components/pulse";
import { fetchLocaleDetail } from "../api";
import { ArrowLeft } from "lucide-react";

export function LocaleDetailPage() {
  const { workspace, pid, locale } = useParams({ strict: false }) as {
    workspace: string;
    pid: string;
    locale: string;
  };

  const { data, isLoading, error } = useQuery({
    queryKey: ["pulse", workspace, "locale", pid, locale],
    queryFn: () => fetchLocaleDetail(workspace, pid, locale),
    staleTime: 2 * 60_000,
  });

  if (isLoading) {
    return <div className="h-64 animate-pulse rounded-lg border bg-muted" />;
  }

  if (error || !data) {
    return (
      <div className="flex min-h-[400px] items-center justify-center text-muted-foreground">
        Locale not found.
      </div>
    );
  }

  const detail = data as {
    locale: string;
    stats: { translated_words: number; total_words: number; percentage: number };
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link
          to="/$workspace/projects/$pid"
          params={{ workspace, pid }}
          className="rounded p-1 hover:bg-muted"
        >
          <ArrowLeft className="h-5 w-5" />
        </Link>
        <div className="flex-1">
          <h1 className="text-xl font-bold">{detail.locale}</h1>
        </div>
        <CompletionRing percentage={detail.stats.percentage} size={64} />
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <div className="rounded-lg border bg-card p-4">
          <div className="text-sm text-muted-foreground">Words Translated</div>
          <div className="mt-1 text-2xl font-bold">
            {detail.stats.translated_words.toLocaleString()}
          </div>
        </div>
        <div className="rounded-lg border bg-card p-4">
          <div className="text-sm text-muted-foreground">Total Words</div>
          <div className="mt-1 text-2xl font-bold">{detail.stats.total_words.toLocaleString()}</div>
        </div>
        <div className="rounded-lg border bg-card p-4">
          <div className="text-sm text-muted-foreground">Completion</div>
          <div className="mt-1 text-2xl font-bold">{detail.stats.percentage.toFixed(1)}%</div>
        </div>
      </div>
    </div>
  );
}
