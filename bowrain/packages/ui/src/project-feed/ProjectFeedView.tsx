// The project feed: one timeline of how a project changes. List and detail,
// at full width. The list holds one row per entry under a sticky header per
// day in the reader's time zone; the detail pane shows the selected entry's
// counts, who did it and for whom, where it sits, a recipe change's values
// before and after, and one button to the place the entry links to.
//
// The server sends counts and names, never sentences, so every label and unit
// is worded here (labels.ts). New entries arrive through the workspace event
// stream, which invalidates the ["projectFeed", ws, project] queries.
import { useCallback, useMemo, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { t, useNeokapi } from "@neokapi/i18n-react/runtime";
import {
  countsOf,
  groupFeedByDay,
  FEED_KINDS,
  type FeedActorKind,
  type FeedEntry,
  type FeedKind,
  type FeedLink,
} from "@neokapi/bowrain-client";
import {
  ActorMark,
  Button,
  EmptyState,
  KNOWN_ACTORS,
  ListDetail,
  ListDetailEmpty,
  ListDetailGroup,
  ListDetailItem,
  ListDetailList,
  ListDetailPane,
  ListDetailToolbar,
  LoadingSpinner,
  PageHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  StatRow,
  ToggleGroup,
  ToggleGroupItem,
  When,
  formatWhen,
} from "@neokapi/ui-primitives";
import {
  Activity,
  BookOpen,
  CheckCheck,
  FileText,
  Flag,
  GitPullRequest,
  RefreshCw,
  Upload,
} from "../components/icons";
import { useApi } from "../context/ApiContext";
import { ErrorNotice } from "../errors";
import {
  actorKindLabel,
  actorName,
  entryTitle,
  kindLabel,
  languageName,
  mainCountText,
  unitLabel,
} from "./labels";

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
  /** Opens the place an entry links to. Absent, the detail offers no button. */
  onOpenLink?: (entry: FeedEntry) => void;
  /** Whether the app has the screen a link names. Absent, every link opens. */
  canOpen?: (link: FeedLink) => boolean;
  /** The entry to open on, by id. Absent opens on the newest. */
  defaultSelected?: string;
}

function KindIcon({ kind }: { kind: string }) {
  const cls = "size-4 text-muted-foreground";
  switch (kind) {
    case "push":
      return <Upload aria-hidden className={cls} />;
    case "convergence_run":
      return <RefreshCw aria-hidden className={cls} />;
    case "review_summary":
      return <CheckCheck aria-hidden className={cls} />;
    case "recipe_change":
      return <FileText aria-hidden className={cls} />;
    case "context_change":
      return <BookOpen aria-hidden className={cls} />;
    case "change_set":
      return <GitPullRequest aria-hidden className={cls} />;
    case "coverage_milestone":
      return <Flag aria-hidden className={cls} />;
    default:
      return <Activity aria-hidden className={cls} />;
  }
}

/** The actor's mark: the assistant's host for an agent, a gear for a tool, else the person. */
function FeedActorMark({ entry, size = "sm" }: { entry: FeedEntry; size?: "sm" | "md" }) {
  const name = actorName(entry);
  if (entry.actor.kind === "tool") return <ActorMark actor="sync" size={size} decorative />;
  if (entry.actor.kind === "agent") {
    const host = (entry.actor.name ?? "").trim().toLowerCase();
    if ((KNOWN_ACTORS as readonly string[]).includes(host)) {
      return <ActorMark actor={host} size={size} decorative />;
    }
    return <ActorMark actor={host || "agent"} name={name} size={size} decorative />;
  }
  return <ActorMark actor="person" name={name} size={size} decorative />;
}

function DayLabel({ daysAgo, date }: { daysAgo: number; date: Date }) {
  const { locale } = useNeokapi();
  if (daysAgo <= 0) return <>{t("Today")}</>;
  if (daysAgo === 1) return <>{t("Yesterday")}</>;
  return <>{formatWhen(date.toISOString(), { uiLocale: locale, timeStyle: "none" }).text}</>;
}

/** One entry: the kind's icon, the primary line, then who, the main count and the time. */
function FeedRow({ entry }: { entry: FeedEntry }) {
  const { locale } = useNeokapi();
  const count = mainCountText(entry, locale);
  return (
    <ListDetailItem
      id={entry.id}
      data-slot="feed-entry"
      data-kind={entry.kind}
      className="[contain-intrinsic-size:auto_36px] [content-visibility:auto]"
      leading={<KindIcon kind={entry.kind} />}
      trailing={<FeedActorMark entry={entry} />}
    >
      <span translate="no">{entryTitle(entry, locale)}</span>
      <span className="ml-1.5 text-[11px] text-muted-foreground tabular-nums">
        <span translate="no">{actorName(entry)}</span>
        {count && <> · {count}</>}
        {" · "}
        <When iso={entry.at} dateStyle="none" timeStyle="short" />
      </span>
    </ListDetailItem>
  );
}

const FACTS_CLASS =
  "grid grid-cols-[minmax(6rem,auto)_minmax(0,1fr)] items-baseline gap-x-4 gap-y-1.5 text-sm";

function coordinatesText(c: Record<string, string> | undefined): string {
  const parts = Object.entries(c ?? {}).map(([axis, value]) => `${axis}: ${value}`);
  return parts.length ? parts.join(", ") : "–";
}

/** The selected entry. */
export function FeedEntryDetail({
  entry,
  onOpenLink,
  canOpen,
}: {
  entry: FeedEntry;
  onOpenLink?: (entry: FeedEntry) => void;
  canOpen?: (link: FeedLink) => boolean;
}) {
  const { locale } = useNeokapi();
  const counts = countsOf(entry);
  const openable =
    onOpenLink && entry.link.screen !== "recipe_diff" && (canOpen?.(entry.link) ?? true);
  const diff = entry.diff;

  return (
    <ListDetailPane
      data-slot="feed-entry-detail"
      data-entry={entry.id}
      toolbar={
        <ListDetailToolbar
          title={
            <span className="flex min-w-0 items-center gap-2">
              <KindIcon kind={entry.kind} />
              <span className="min-w-0 truncate" translate="no">
                {entryTitle(entry, locale)}
              </span>
            </span>
          }
          actions={
            openable ? (
              <Button size="sm" data-slot="feed-entry-open" onClick={() => onOpenLink(entry)}>
                {t("Open")}
              </Button>
            ) : undefined
          }
        />
      }
    >
      {counts.length > 0 && (
        <StatRow
          data-slot="feed-entry-counts"
          items={counts.map((c) => ({
            id: c.key,
            value: c.n,
            label: unitLabel(c.key, c.n),
          }))}
        />
      )}
      <dl data-slot="feed-entry-facts" className={FACTS_CLASS}>
        <dt className="text-[11px] text-muted-foreground">{t("Who")}</dt>
        <dd className="flex min-w-0 items-center gap-1.5">
          <FeedActorMark entry={entry} />
          <span translate="no">{actorName(entry)}</span>
          <span className="text-muted-foreground">· {actorKindLabel(entry.actor.kind)}</span>
        </dd>
        {entry.on_behalf_of && (
          <>
            <dt className="text-[11px] text-muted-foreground">{t("For", "acting for a member")}</dt>
            <dd className="min-w-0" translate="no">
              {entry.on_behalf_of.name ?? entry.on_behalf_of.id}
            </dd>
          </>
        )}
        {entry.actor.session && (
          <>
            <dt className="text-[11px] text-muted-foreground">{t("Session")}</dt>
            <dd className="min-w-0 font-mono text-xs" translate="no">
              {[entry.actor.host, entry.actor.session].filter(Boolean).join(" · ")}
            </dd>
          </>
        )}
        <dt className="text-[11px] text-muted-foreground">{t("When")}</dt>
        <dd className="min-w-0">
          <When iso={entry.at} />
        </dd>
        {entry.stream && (
          <>
            <dt className="text-[11px] text-muted-foreground">{t("Stream")}</dt>
            <dd className="min-w-0" translate="no">
              {entry.stream}
            </dd>
          </>
        )}
        {entry.locales && entry.locales.length > 0 && (
          <>
            <dt className="text-[11px] text-muted-foreground">{t("Languages")}</dt>
            <dd className="min-w-0">
              {entry.locales.map((l) => languageName(l, locale)).join(", ")}
            </dd>
          </>
        )}
        {entry.coordinates && (
          <>
            <dt className="text-[11px] text-muted-foreground">{t("Where")}</dt>
            <dd className="min-w-0" translate="no">
              {coordinatesText(entry.coordinates)}
            </dd>
          </>
        )}
      </dl>

      {diff && (
        <section aria-label={t("Changes")} data-slot="feed-recipe-diff">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11px] text-muted-foreground">
                <th className="py-1 pr-4 font-medium">{t("What")}</th>
                <th className="py-1 pr-4 font-medium">{t("Before")}</th>
                <th className="py-1 font-medium">{t("After")}</th>
              </tr>
            </thead>
            <tbody>
              {diff.targets && (
                <tr className="border-t border-border align-baseline">
                  <td className="py-1.5 pr-4">{t("Target languages")}</td>
                  <td className="py-1.5 pr-4" translate="no">
                    {diff.targets.before.join(", ") || "–"}
                  </td>
                  <td className="py-1.5" translate="no">
                    {diff.targets.after.join(", ") || "–"}
                  </td>
                </tr>
              )}
              {(diff.collections ?? []).map((c) => (
                <tr key={`c-${c.name}`} className="border-t border-border align-baseline">
                  <td className="py-1.5 pr-4" translate="no">
                    {c.name}
                  </td>
                  <td className="py-1.5 pr-4" translate="no">
                    {coordinatesText(c.before)}
                  </td>
                  <td className="py-1.5" translate="no">
                    {coordinatesText(c.after)}
                  </td>
                </tr>
              ))}
              {(diff.settings ?? []).map((s) => (
                <tr key={`s-${s.name}`} className="border-t border-border align-baseline">
                  <td className="py-1.5 pr-4 font-mono text-xs" translate="no">
                    {s.name}
                  </td>
                  <td className="py-1.5 pr-4" translate="no">
                    {s.before || "–"}
                  </td>
                  <td className="py-1.5" translate="no">
                    {s.after || "–"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </ListDetailPane>
  );
}

export function ProjectFeedView({
  workspace,
  projectId,
  languages = [],
  onOpenLink,
  canOpen,
  defaultSelected,
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

  const entries = useMemo(() => {
    const seen = new Set<string>();
    return (feed.data?.pages ?? []).flatMap((p) =>
      p.entries.filter((e) => (seen.has(e.id) ? false : (seen.add(e.id), true))),
    );
  }, [feed.data]);
  const days = useMemo(() => groupFeedByDay(entries), [entries]);

  const [chosen, setChosen] = useState<string | null>(defaultSelected ?? null);
  const selected = entries.find((e) => e.id === chosen) ?? entries[0] ?? null;
  const select = useCallback((id: string) => setChosen(id), []);
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
          <SelectTrigger size="sm" className="w-40 text-xs" aria-label={t("Language")}>
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
  } else if (entries.length === 0) {
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
    const list = (
      <>
        <ListDetailList
          selectedId={selected?.id}
          onSelect={select}
          aria-label={t("Activity")}
          data-testid="project-feed-entries"
        >
          {days.map((day) => (
            <ListDetailGroup
              key={day.key}
              data-slot="feed-day"
              label={<DayLabel daysAgo={day.daysAgo} date={day.date} />}
              count={day.entries.length}
            >
              {day.entries.map((entry) => (
                <FeedRow key={entry.id} entry={entry} />
              ))}
            </ListDetailGroup>
          ))}
        </ListDetailList>
        {feed.hasNextPage && (
          <div className="px-2 pb-3">
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
          </div>
        )}
      </>
    );
    body = (
      <div className="min-h-[420px] flex-1 overflow-hidden rounded-lg border border-border">
        <ListDetail
          list={list}
          listWidth={380}
          detail={
            selected ? (
              <FeedEntryDetail
                key={selected.id}
                entry={selected}
                onOpenLink={onOpenLink}
                canOpen={canOpen}
              />
            ) : (
              <ListDetailEmpty title={t("Choose an entry to see what it changed")} />
            )
          }
        />
      </div>
    );
  }

  return (
    <div data-slot="project-feed" className="flex min-h-0 flex-1 flex-col gap-3">
      <PageHeader className="mb-0 shrink-0" title={t("Activity")} actions={filters} />
      {body}
    </div>
  );
}
