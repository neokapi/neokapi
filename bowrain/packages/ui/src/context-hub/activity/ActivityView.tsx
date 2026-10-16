// Context activity: the workspace's recorded work on its context, as a list of
// sessions beside a detail pane, the platform's twin of Kapi Desktop's History.
//
// The feed is one server read, filtered by the type families the category
// toggles select and paged by cursor. A session is one person's run of work
// (feed.ts sessionsOf). The list has one row per session under its day: who
// worked, when, and how many operations the session holds. The detail pane
// lists the session's operations, one row each with the icon of its kind, and
// its facts: who, when, what kinds of thing changed, and the reviews decided in
// it, which is the platform's record of who decided what stays. A row about a
// concept or a change-set opens it.
//
// Two list reads name the change-sets and concepts a page refers to, so a row
// says what it is about rather than showing an id.
import { useCallback, useMemo, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { t, useNeokapi } from "@neokapi/i18n-react/runtime";
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
  SimpleTooltip,
  ToggleGroup,
  ToggleGroupItem,
  When,
  cn,
  formatWhen,
} from "@neokapi/ui-primitives";
import { ErrorNotice } from "../../errors";
import {
  Activity,
  Archive,
  Check,
  FlaskConical,
  GitFork,
  GitMerge,
  GitPullRequest,
  Lock,
  MessageSquare,
  Network,
  Pencil,
  Quote,
} from "../../components/icons";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { useChangesets } from "../../hooks/useChangesetsApi";
import { useConcepts } from "../../hooks/useConceptsApi";
import { useUserDisplayNames, type NameResolver } from "../../hooks/useMembersApi";
import {
  ACTIVITY_CATEGORIES,
  categoryCounts,
  conceptDisplayName,
  decisionsOf,
  groupSessionsByDay,
  sessionsOf,
  toFeedItem,
  typePrefixesFor,
  type ActivityCategory,
  type ActivitySession,
  type EntityNames,
  type FeedItem,
} from "./feed";

/** Operations per page. The feed is cursor-paged; "Show older activity" reads the next one. */
const PAGE_SIZE = 100;

/** Concepts named for the feed, most recently updated first: what a feed cites. */
const NAMED_CONCEPTS = 100;

function categoryLabel(category: ActivityCategory): string {
  switch (category) {
    case "experiment":
      return t("Changes", "activity category");
    case "concept":
      return t("Concepts", "activity category");
    case "observation":
      return t("Evidence", "activity category");
    case "comment":
      return t("Comments", "activity category");
  }
}

/** A category's count, worded: "2 concepts", "1 change". */
function categoryCount(category: ActivityCategory, count: number): string {
  switch (category) {
    case "experiment":
      return t("{count, plural, one {# change} other {# changes}}", { count });
    case "concept":
      return t("{count, plural, one {# concept} other {# concepts}}", { count });
    case "observation":
      return t("{count, plural, one {# piece of evidence} other {# pieces of evidence}}", {
        count,
      });
    case "comment":
      return t("{count, plural, one {# comment} other {# comments}}", { count });
  }
}

export interface ActivityViewProps {
  onOpenConcept?: (conceptId: string) => void;
  onOpenExperiment?: (changesetId: string) => void;
  /** The session to open on, by its id. Absent opens on the newest. */
  defaultSelected?: string;
}

export function ActivityView({
  onOpenConcept,
  onOpenExperiment,
  defaultSelected,
}: ActivityViewProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const { nameOf } = useUserDisplayNames();

  const [enabled, setEnabled] = useState<ActivityCategory[]>([...ACTIVITY_CATEGORIES]);
  const types = useMemo(() => typePrefixesFor(enabled), [enabled]);

  const feedQuery = useInfiniteQuery({
    queryKey: ["voice-activities", ws, types],
    queryFn: ({ pageParam }) =>
      api.listActivities(ws, {
        types,
        limit: PAGE_SIZE,
        cursor: pageParam || undefined,
      }),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    // Nothing selected asks the server for nothing, rather than for everything.
    enabled: !!ws && types.length > 0,
    staleTime: 10_000,
  });

  // The names a page cites. Both are single reads the hub already makes
  // elsewhere; a row whose subject neither names falls back to the server's own
  // sentence, so nothing renders as a raw id.
  const { data: changesets } = useChangesets();
  const { data: conceptsData } = useConcepts({ sort: "updated_at", limit: NAMED_CONCEPTS });
  const names: EntityNames = useMemo(() => {
    const out: EntityNames = {};
    for (const cs of changesets ?? []) out[cs.id] = cs.name;
    for (const concept of conceptsData?.concepts ?? []) {
      out[concept.id] = conceptDisplayName(concept);
    }
    return out;
  }, [changesets, conceptsData]);

  const sessions = useMemo(
    () =>
      sessionsOf(
        (feedQuery.data?.pages ?? []).flatMap((page) =>
          (page.activities ?? []).map((activity) => toFeedItem(activity, names)),
        ),
      ),
    [feedQuery.data, names],
  );
  const days = useMemo(() => groupSessionsByDay(sessions), [sessions]);

  // The selected session, by id, so a refetch that brings new operations
  // leaves the same session selected. A session that is gone falls back to
  // the newest.
  const [chosen, setChosen] = useState<string | null>(defaultSelected ?? null);
  const selected = sessions.find((s) => s.id === chosen) ?? sessions[0] ?? null;
  const select = useCallback((id: string) => setChosen(id), []);

  const toolbar = (
    <ToggleGroup
      type="multiple"
      variant="outline"
      size="sm"
      value={enabled}
      onValueChange={(v: string[]) => setEnabled(v as ActivityCategory[])}
      aria-label={t("What to show")}
      className="flex-wrap justify-start"
    >
      {ACTIVITY_CATEGORIES.map((cat) => (
        <ToggleGroupItem
          key={cat}
          value={cat}
          data-slot={`activity-filter-${cat}`}
          className="gap-1.5 text-xs"
        >
          {categoryLabel(cat)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );

  let body: React.ReactNode;
  if (feedQuery.isError) {
    body = (
      <ErrorNotice
        error={feedQuery.error}
        title={t("The activity could not be read")}
        hint={t("The workspace feed is a single read; retry it.")}
      />
    );
  } else if (types.length === 0) {
    body = (
      <EmptyState
        className="mx-auto w-full max-w-md"
        icon={<Activity className="mx-auto size-5" />}
        title={t("Nothing is selected to show")}
        description={t("Turn a category back on to read its activity.")}
      />
    );
  } else if (feedQuery.isLoading) {
    body = <LoadingSpinner />;
  } else if (sessions.length === 0) {
    body = (
      <EmptyState
        className="mx-auto w-full max-w-md"
        icon={<Activity className="mx-auto size-5" />}
        title={t("No activity yet")}
        description={t(
          "When someone changes a concept, records evidence or reviews a change, it appears here.",
        )}
      />
    );
  } else {
    const list = (
      <>
        <ListDetailList
          selectedId={selected?.id}
          onSelect={select}
          aria-label={t("Sessions")}
          data-testid="activity-sessions"
        >
          {days.map((day) => (
            <ListDetailGroup
              key={day.key}
              data-slot="activity-day"
              label={<DayLabel iso={day.sessions[0].last} />}
              count={day.sessions.length}
            >
              {day.sessions.map((session) => (
                <SessionRow key={session.id} session={session} nameOf={nameOf} />
              ))}
            </ListDetailGroup>
          ))}
        </ListDetailList>
        {feedQuery.hasNextPage && (
          <div className="px-2 pb-3">
            <Button
              variant="ghost"
              size="sm"
              data-slot="activity-older"
              className="w-full text-muted-foreground"
              disabled={feedQuery.isFetchingNextPage}
              onClick={() => void feedQuery.fetchNextPage()}
            >
              Show older activity
            </Button>
          </div>
        )}
      </>
    );
    body = (
      <div className="min-h-[420px] flex-1 overflow-hidden rounded-lg border border-border">
        <ListDetail
          list={list}
          listWidth={320}
          detail={
            selected ? (
              <SessionDetail
                key={selected.id}
                session={selected}
                nameOf={nameOf}
                names={names}
                onOpenConcept={onOpenConcept}
                onOpenExperiment={onOpenExperiment}
              />
            ) : (
              <ListDetailEmpty title={t("Choose a session to see what it changed")} />
            )
          }
        />
      </div>
    );
  }

  return (
    <div data-slot="context-activity" className="flex min-h-0 flex-1 flex-col gap-3">
      <PageHeader className="mb-0 shrink-0" title="Activity" actions={toolbar} />
      {body}
    </div>
  );
}

// ── Labels ───────────────────────────────────────────────────────────────────

function startOfDay(ms: number): number {
  const d = new Date(ms);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** A day heading in the reader's language: Today, Yesterday, or the date. */
function DayLabel({ iso }: { iso: string }) {
  const { locale } = useNeokapi();
  const at = Date.parse(iso);
  const days = Number.isNaN(at)
    ? Number.NaN
    : Math.round((startOfDay(Date.now()) - startOfDay(at)) / 86_400_000);
  if (days <= 0) return <>{t("Today")}</>;
  if (days === 1) return <>{t("Yesterday")}</>;
  return <>{formatWhen(iso, { uiLocale: locale, timeStyle: "none" }).text}</>;
}

/** The name a session's actor goes by: the member's name, else the name the server recorded. */
function actorName(session: ActivitySession, nameOf: NameResolver): string {
  const resolved = nameOf(session.actor);
  if (resolved && resolved !== session.actor) return resolved;
  return session.actorName || session.actor || t("Someone");
}

/** The mark for who worked: an assistant by its host when it is one, else a person. */
function SessionMark({
  session,
  name,
  size = "sm",
}: {
  session: ActivitySession;
  name: string;
  size?: "sm" | "md";
}) {
  const key = (session.actorName ?? "").trim().toLowerCase();
  const assistant = (KNOWN_ACTORS as readonly string[]).includes(key) && key !== "person";
  return assistant ? (
    <ActorMark actor={key} size={size} decorative />
  ) : (
    <ActorMark actor="person" name={name} size={size} decorative />
  );
}

// ── The list ─────────────────────────────────────────────────────────────────

/** One session in the list: who worked, when, and how many operations it holds. */
function SessionRow({ session, nameOf }: { session: ActivitySession; nameOf: NameResolver }) {
  const name = actorName(session, nameOf);
  const decided = decisionsOf(session).length;
  return (
    <ListDetailItem
      id={session.id}
      data-slot="activity-session"
      data-session={session.id}
      // Rows off screen skip layout and paint, so a long feed scrolls as
      // lightly as a short one. The intrinsic size keeps the scrollbar honest.
      className="[contain-intrinsic-size:auto_36px] [content-visibility:auto]"
      leading={<SessionMark session={session} name={name} />}
      trailing={
        <>
          {decided > 0 && (
            <span
              role="img"
              data-slot="activity-session-decided"
              aria-label={t("{count, plural, one {# review decided} other {# reviews decided}}", {
                count: decided,
              })}
              className="size-1.5 rounded-full bg-success"
            />
          )}
          <span className="text-[11px] text-muted-foreground tabular-nums">
            {session.items.length}
          </span>
        </>
      }
    >
      <span translate="no">{name}</span>
      <When iso={session.last} relative className="ml-1.5 text-[11px] text-muted-foreground" />
    </ListDetailItem>
  );
}

// ── The detail ───────────────────────────────────────────────────────────────

interface SessionDetailProps {
  session: ActivitySession;
  nameOf: NameResolver;
  names: EntityNames;
  onOpenConcept?: (conceptId: string) => void;
  onOpenExperiment?: (changesetId: string) => void;
}

const FACTS_CLASS =
  "grid grid-cols-[minmax(6rem,auto)_minmax(0,1fr)] items-baseline gap-x-4 gap-y-1.5 text-sm";

/** The selected session: who worked in the toolbar, the facts, and the operations below. */
function SessionDetail({
  session,
  nameOf,
  names,
  onOpenConcept,
  onOpenExperiment,
}: SessionDetailProps) {
  const name = actorName(session, nameOf);
  const decisions = decisionsOf(session);
  const counts = categoryCounts(session);
  const span = session.first !== session.last;

  return (
    <ListDetailPane
      data-slot="activity-session-detail"
      data-session={session.id}
      toolbar={
        <ListDetailToolbar
          title={
            <span className="flex min-w-0 items-center gap-2">
              <SessionMark session={session} name={name} size="md" />
              <span className="min-w-0 truncate" translate="no">
                {name}
              </span>
            </span>
          }
        />
      }
    >
      <dl data-slot="activity-session-facts" className={FACTS_CLASS}>
        <dt className="text-[11px] text-muted-foreground">When</dt>
        <dd className="min-w-0">
          {span ? (
            <>
              <When iso={session.first} /> to{" "}
              <When iso={session.last} timeStyle="short" dateStyle="none" />
            </>
          ) : (
            <When iso={session.last} />
          )}
        </dd>
        {counts.length > 0 && (
          <>
            <dt className="text-[11px] text-muted-foreground">Changed</dt>
            <dd className="min-w-0 tabular-nums">
              {counts.map(([category, count]) => categoryCount(category, count)).join(" · ")}
            </dd>
          </>
        )}
        {decisions.length > 0 && (
          <>
            <dt className="text-[11px] text-muted-foreground">Decided</dt>
            <dd className="min-w-0">
              <ul data-slot="activity-session-decisions" className="space-y-0.5">
                {decisions.map((decision) => (
                  <li key={decision.id} className="flex min-w-0 flex-wrap items-baseline gap-x-1.5">
                    <Check aria-hidden className="size-3 shrink-0 self-center text-success" />
                    <span translate="no">{decision.detail ?? decision.title}</span>
                    {decision.changesetId && names[decision.changesetId] && (
                      <EntityLink
                        label={names[decision.changesetId]}
                        onOpen={
                          onOpenExperiment
                            ? () => onOpenExperiment(decision.changesetId!)
                            : undefined
                        }
                      />
                    )}
                  </li>
                ))}
              </ul>
            </dd>
          </>
        )}
      </dl>

      <section aria-label={t("Operations")}>
        <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Operations</h3>
        <ul data-slot="activity-operations" className="-mx-2 flex flex-col gap-px">
          {session.items.map((item) => (
            <li key={item.id}>
              <OperationRow
                item={item}
                onOpenConcept={onOpenConcept}
                onOpenExperiment={onOpenExperiment}
              />
            </li>
          ))}
        </ul>
      </section>
    </ListDetailPane>
  );
}

function EntityLink({ label, onOpen }: { label: string; onOpen?: () => void }) {
  if (!onOpen) {
    return (
      <span className="text-muted-foreground" translate="no">
        {label}
      </span>
    );
  }
  return (
    <button
      type="button"
      onClick={onOpen}
      className="min-w-0 truncate rounded text-left text-muted-foreground underline-offset-2 hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
      translate="no"
    >
      {label}
    </button>
  );
}

/** What a recorded activity type is, in the active UI language. */
function typeLabel(type: string): string {
  switch (type) {
    case "changeset.created":
      return t("Experiment opened", "activity type");
    case "changeset.abandoned":
      return t("Experiment abandoned", "activity type");
    case "changeset.superseded":
      return t("Experiment superseded", "activity type");
    case "review.assigned":
      return t("Submitted for review", "activity type");
    case "review.decided":
      return t("Review decided", "activity type");
    case "concept.term.status_changed":
      return t("Term status changed", "activity type");
    case "concept.relation.added":
      return t("Relation added", "activity type");
    case "concept.relation.removed":
      return t("Relation removed", "activity type");
    case "concept.created":
      return t("Concept created", "activity type");
    case "concept.deleted":
      return t("Concept deleted", "activity type");
    case "observation.added":
      return t("Evidence recorded", "activity type");
    case "comment.added":
      return t("Comment posted", "activity type");
    default:
      break;
  }
  if (type.startsWith("pilot.")) return t("Pilot", "activity type");
  if (type.startsWith("concept.")) return t("Concept changed", "activity type");
  return type;
}

/** The icon for a recorded activity type, matched most-specific first. */
function TypeIcon({ type }: { type: string }) {
  let icon: React.ReactNode;
  switch (type) {
    case "changeset.created":
      icon = <FlaskConical />;
      break;
    case "changeset.abandoned":
    case "changeset.superseded":
      icon = <Archive />;
      break;
    case "review.assigned":
      icon = <GitPullRequest />;
      break;
    case "review.decided":
      icon = <Check />;
      break;
    case "concept.term.status_changed":
      icon = <Lock />;
      break;
    case "concept.relation.added":
    case "concept.relation.removed":
      icon = <Network />;
      break;
    default:
      if (type.startsWith("pilot.")) icon = <GitFork />;
      else if (type.startsWith("concept.")) icon = <Pencil />;
      else if (type.startsWith("observation.")) icon = <Quote />;
      else if (type.startsWith("comment.")) icon = <MessageSquare />;
      else if (type.startsWith("changeset.")) icon = <GitMerge />;
      else icon = <Activity />;
  }
  const label = typeLabel(type);
  return (
    <SimpleTooltip content={label}>
      <span
        role="img"
        aria-label={label}
        data-slot="activity-kind"
        data-type={type}
        className={cn(
          "inline-flex [&_svg]:size-3.5",
          type === "review.decided" ? "text-success" : "text-muted-foreground",
        )}
      >
        {icon}
      </span>
    </SimpleTooltip>
  );
}

/** One operation: its kind, what it was about, and when. A row about something opens it. */
function OperationRow({
  item,
  onOpenConcept,
  onOpenExperiment,
}: {
  item: FeedItem;
  onOpenConcept?: (conceptId: string) => void;
  onOpenExperiment?: (changesetId: string) => void;
}) {
  const { changesetId, conceptId } = item;
  const open =
    changesetId && onOpenExperiment
      ? () => onOpenExperiment(changesetId)
      : conceptId && onOpenConcept
        ? () => onOpenConcept(conceptId)
        : undefined;
  const Tag = open ? "button" : "div";
  return (
    <Tag
      type={open ? "button" : undefined}
      onClick={open}
      data-slot="activity-operation"
      data-type={item.type}
      className={cn(
        "flex w-full min-w-0 items-start gap-2 rounded-md px-2 py-1.5 text-left",
        open &&
          "hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none",
      )}
    >
      <span className="mt-0.5 flex w-4 shrink-0 justify-center">
        <TypeIcon type={item.type} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm" translate="no">
          {item.title}
        </span>
        <span className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted-foreground">
          {item.detail && (
            <>
              <span translate="no">{item.detail}</span>
              <span aria-hidden>·</span>
            </>
          )}
          <When iso={item.at} relative />
        </span>
      </span>
    </Tag>
  );
}
