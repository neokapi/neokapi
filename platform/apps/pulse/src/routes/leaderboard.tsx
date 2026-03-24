import { useParams, useSearch } from "@tanstack/react-router";
import { ContributorBoard, LanguageProgressGrid } from "@neokapi/ui/components/pulse";
import { usePulseLeaderboard } from "../hooks/use-pulse";

export function LeaderboardPage() {
  const { workspace } = useParams({ strict: false }) as { workspace: string };
  const search = useSearch({ strict: false }) as { tab?: string; time?: string };
  const tab = search.tab ?? "languages";

  const { data, isLoading } = usePulseLeaderboard(workspace);

  if (isLoading) {
    return <div className="h-64 animate-pulse rounded-lg border bg-muted" />;
  }

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-bold">Leaderboard</h1>

      <div className="flex gap-2 border-b">
        <TabButton active={tab === "languages"} href={`/${workspace}/leaderboard?tab=languages`}>
          Languages
        </TabButton>
        <TabButton
          active={tab === "contributors"}
          href={`/${workspace}/leaderboard?tab=contributors`}
        >
          Contributors
        </TabButton>
      </div>

      {tab === "languages" && data && (
        <LanguageProgressGrid
          languages={data.languages.map((l) => ({
            locale: l.locale,
            translated_words: l.translated_words,
            total_words: l.total_words,
            percentage: l.percentage,
          }))}
        />
      )}

      {tab === "contributors" && data && <ContributorBoard contributors={data.contributors} />}
    </div>
  );
}

function TabButton({
  active,
  href,
  children,
}: {
  active: boolean;
  href: string;
  children: React.ReactNode;
}) {
  return (
    <a
      href={href}
      className={`border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
        active
          ? "border-primary text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground"
      }`}
    >
      {children}
    </a>
  );
}
