// The project feed: one timeline of how a project changes, as a column of
// cards under a heading per day in the reader's time zone. Each card names
// who did what in a sentence and shows a preview of the change; a card opens
// the place its entry links to, and a language chip opens that language.
//
// The server sends counts and names, never sentences; every sentence is
// composed here through the catalog (labels.ts). Quiet runs are left out and
// one actor's changes close together fold into one card (foldFeed). New
// entries arrive through the workspace event stream, which invalidates the
// ["projectFeed", ws, project] queries.
import { useMemo, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { t, useNeokapi } from "@neokapi/i18n-react/runtime";
import {
  foldFeed,
  groupFeedByDay,
  FEED_KINDS,
  type FeedActorKind,
  type FeedEntry,
  type FeedKind,
  type FeedLink,
} from "@neokapi/bowrain-client";
import {
  Button,
  EmptyState,
  LoadingSpinner,
  PageHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  ToggleGroup,
  ToggleGroupItem,
  formatWhen,
} from "@neokapi/ui-primitives";
import { Activity } from "../components/icons";
import { useApi } from "../context/ApiContext";
import { ErrorNotice } from "../errors";
import { FeedCard } from "./FeedCard";
import { actorKindLabel, kindLabel, languageName } from "./labels";

/** Entries per page. */
const PAGE_SIZE = 50;

const ACTOR_KINDS: FeedActorKind[] = ["person", "agent", "tool"];

/** The query key prefix of a project's feed; the event stream invalidates it. */
export function projectFeedKey(workspace: string, projectId: string) {
  return ["projectFeed", workspace, projectId] as const;
}

export interface ProjectFeedViewProps {
  workspace: string;
  projectId: string;
  /** The project's target languages, for the language filter. */
  languages?: string[];
  /** Opens the place an entry links to. */
  onOpenLink?: (entry: FeedEntry) => void;
  /** Opens a language's review deck, from a language chip. */
  onOpenLanguage?: (locale: string) => void;
  /** Whether the app has the screen a link names. Absent, every link opens. */
  canOpen?: (link: FeedLink) => boolean;
}

function DayHeading({ daysAgo, date }: { daysAgo: number; date: Date }) {
  const { locale } = useNeokapi();
  const label =
    daysAgo <= 0
      ? t("Today")
      : daysAgo === 1
        ? t("Yesterday")
        : formatWhen(date.toISOString(), { uiLocale: locale, timeStyle: "none" }).text;
  return <h2 className="px-1 pt-4 pb-1 text-base font-bold text-foreground">{label}</h2>;
}

export function ProjectFeedView({
  workspace,
  projectId,
  languages = [],
  onOpenLink,
  onOpenLanguage,
  canOpen,
}: ProjectFeedViewProps) {
  const api = useApi();
  const [kinds, setKinds] = useState<FeedKind[]>([]);
  const [actorKind, setActorKind] = useState<FeedActorKind | "">("");
  const [locale, setLocale] = useState("");

  const feed = useInfiniteQuery({
    queryKey: [...projectFeedKey(workspace, projectId), kinds, actorKind, locale],
    queryFn: ({ pageParam }) =>
      api.listProjectFeed(workspace, projectId, {
        kinds: kinds.length ? kinds : undefined,
        actorKind: actorKind || undefined,
        locale: locale || undefined,
        limit: PAGE_SIZE,
        cursor: pageParam || undefined,
      }),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled: !!workspace && !!projectId,
    staleTime: 10_000,
  });

  const days = useMemo(() => {
    const seen = new Set<string>();
    const entries = (feed.data?.pages ?? []).flatMap((p) =>
      p.entries.filter((e) => (seen.has(e.id) ? false : (seen.add(e.id), true))),
    );
    return groupFeedByDay(entries)
      .map((day) => ({ ...day, cards: foldFeed(day.entries) }))
      .filter((day) => day.cards.length > 0);
  }, [feed.data]);

  const open = (entry: FeedEntry) => {
    if (!onOpenLink) return;
    if (entry.link.screen === "recipe_diff") return;
    if (canOpen && !canOpen(entry.link)) return;
    onOpenLink(entry);
  };
  const filtered = kinds.length > 0 || actorKind !== "" || locale !== "";

  const filters = (
    <div className="flex flex-wrap items-center gap-2" data-slot="feed-filters">
      <ToggleGroup
        type="multiple"
        variant="outline"
        size="sm"
        value={kinds}
        onValueChange={(v: string[]) => setKinds(v as FeedKind[])}
        aria-label={t("Kinds to show")}
        className="flex-wrap justify-start"
      >
        {FEED_KINDS.map((k) => (
          <ToggleGroupItem key={k} value={k} data-slot={`feed-filter-${k}`} className="text-xs">
            {kindLabel(k)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        value={actorKind}
        onValueChange={(v: string) => setActorKind(v as FeedActorKind | "")}
        aria-label={t("Who to show")}
      >
        {ACTOR_KINDS.map((k) => (
          <ToggleGroupItem key={k} value={k} data-slot={`feed-actor-${k}`} className="text-xs">
            {actorKindLabel(k)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {languages.length > 0 && (
        <Select value={locale || "all"} onValueChange={(v) => setLocale(v === "all" ? "" : v)}>
          <SelectTrigger size="sm" className="w-44 text-xs" aria-label={t("Language")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("All languages")}</SelectItem>
            {languages.map((l) => (
              <SelectItem key={l} value={l}>
                {languageName(l, l)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );

  let body: React.ReactNode;
  if (feed.isError) {
    body = (
      <ErrorNotice
        error={feed.error}
        title={t("The activity could not be read")}
        hint={t("Retry, or reload the page.")}
      />
    );
  } else if (feed.isLoading) {
    body = <LoadingSpinner />;
  } else if (days.length === 0) {
    body = (
      <EmptyState
        className="mx-auto w-full max-w-md"
        icon={<Activity className="mx-auto size-5" />}
        title={filtered ? t("Nothing matches these filters") : t("No activity yet")}
        description={
          filtered
            ? t("Turn a filter off to see more of the project's activity.")
            : t("Pushes, runs, reviews, recipe changes and context changes appear here.")
        }
      />
    );
  } else {
    body = (
      <div data-testid="project-feed-entries" className="space-y-3 pb-6">
        {days.map((day) => (
          <section key={day.key} data-slot="feed-day" className="space-y-3">
            <DayHeading daysAgo={day.daysAgo} date={day.date} />
            {day.cards.map((card) => (
              <FeedCard key={card.key} card={card} onOpen={open} onOpenLanguage={onOpenLanguage} />
            ))}
          </section>
        ))}
        {feed.hasNextPage && (
          <Button
            variant="ghost"
            size="sm"
            data-slot="feed-older"
            className="w-full text-muted-foreground"
            disabled={feed.isFetchingNextPage}
            onClick={() => void feed.fetchNextPage()}
          >
            {t("Show older activity")}
          </Button>
        )}
      </div>
    );
  }

  return (
    <div data-slot="project-feed" className="mx-auto flex w-full max-w-2xl flex-col gap-2">
      <PageHeader className="mb-0" title={t("Activity")} />
      {filters}
      {body}
    </div>
  );
}
