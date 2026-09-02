// Activity — the brand-scoped timeline (AD-021). It reads the workspace
// activity feed, filtered server-side to the knowledge type families the
// category toggles select, and groups the page by day. The mapping and grouping
// live in ./feed (pure, tested); this file is the data wiring and the timeline
// presentation. Two list reads name the change-sets and concepts a page refers
// to, so a row says what it is about rather than showing an id.
import { useMemo, useState } from "react";
import {
  Button,
  Card,
  CardContent,
  Skeleton,
  ToggleGroup,
  ToggleGroupItem,
  cn,
} from "@neokapi/ui-primitives";
import { useInfiniteQuery } from "@tanstack/react-query";
import { ErrorNotice } from "../../errors";
import {
  Activity,
  FlaskConical,
  GitMerge,
  GitPullRequest,
  GitFork,
  Pencil,
  Quote,
  MessageSquare,
  Check,
  Archive,
  Loader2,
  Network,
  Lock,
} from "../../components/icons";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { useChangesets } from "../../hooks/useChangesetsApi";
import { useConcepts } from "../../hooks/useConceptsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { ContextHub } from "../shell/ContextHub";
import { EmptyState, formatRelative } from "../shell/atoms";
import {
  ACTIVITY_CATEGORIES,
  conceptDisplayName,
  groupByDay,
  toFeedItem,
  typePrefixesFor,
  type ActivityCategory,
  type EntityNames,
  type FeedItem,
} from "./feed";

/** Rows per page. The feed is cursor-paged; "Load more" fetches the next one. */
const PAGE_SIZE = 40;

/** Concepts named for the feed, most recently updated first — what a feed cites. */
const NAMED_CONCEPTS = 100;

const CATEGORY_LABEL: Record<ActivityCategory, string> = {
  experiment: "Experiments",
  concept: "Concepts",
  observation: "Observations",
  comment: "Comments",
};

export interface ActivityViewProps {
  onOpenConcept?: (conceptId: string) => void;
  onOpenExperiment?: (changesetId: string) => void;
}

export function ActivityView({ onOpenConcept, onOpenExperiment }: ActivityViewProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

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

  const feed = useMemo(
    () =>
      (feedQuery.data?.pages ?? []).flatMap((page) =>
        (page.activities ?? []).map((activity) => toFeedItem(activity, names)),
      ),
    [feedQuery.data, names],
  );

  const groups = groupByDay(feed);
  const nothingSelected = types.length === 0;

  const toolbar = (
    <ToggleGroup
      type="multiple"
      variant="outline"
      size="sm"
      value={enabled}
      onValueChange={(v: string[]) => setEnabled(v as ActivityCategory[])}
      className="flex-wrap justify-start"
    >
      {ACTIVITY_CATEGORIES.map((cat) => (
        <ToggleGroupItem key={cat} value={cat} className="gap-1.5 text-xs">
          {CATEGORY_LABEL[cat]}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );

  return (
    <ContextHub
      title="Activity"
      description="What's changing across your brand language: experiments moving through review, and edits to concepts, observations, and discussions."
      toolbar={toolbar}
    >
      {feedQuery.isError ? (
        <ErrorNotice
          error={feedQuery.error}
          title="Couldn't load the activity feed"
          hint="The workspace feed is a single read; retry it."
        />
      ) : nothingSelected ? (
        <EmptyState
          icon={<Activity />}
          title="No categories selected"
          description="Re-enable a filter to load activity."
        />
      ) : feedQuery.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-14 w-full" />
          ))}
        </div>
      ) : feed.length === 0 ? (
        <EmptyState
          icon={<Activity />}
          title="No activity yet"
          description="Once you open experiments or edit concepts, their changes appear here in order."
        />
      ) : (
        <div className="space-y-6">
          {groups.map((group) => (
            <section key={group.key}>
              <div className="mb-2 flex items-center gap-3">
                <h2 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {group.label}
                </h2>
                <div className="h-px flex-1 bg-border" />
                <span className="text-xs tabular-nums text-muted-foreground">
                  {group.items.length}
                </span>
              </div>
              <Card>
                <CardContent className="p-0">
                  <ol className="divide-y">
                    {group.items.map((item) => (
                      <TimelineRow
                        key={item.id}
                        item={item}
                        onOpenConcept={onOpenConcept}
                        onOpenExperiment={onOpenExperiment}
                      />
                    ))}
                  </ol>
                </CardContent>
              </Card>
            </section>
          ))}
          {feedQuery.hasNextPage && (
            <div className="flex justify-center">
              <Button
                variant="outline"
                size="sm"
                disabled={feedQuery.isFetchingNextPage}
                onClick={() => void feedQuery.fetchNextPage()}
              >
                {feedQuery.isFetchingNextPage && <Loader2 className="animate-spin" />}
                {feedQuery.isFetchingNextPage ? "Loading" : "Load older activity"}
              </Button>
            </div>
          )}
        </div>
      )}
    </ContextHub>
  );
}

// ── Row ───────────────────────────────────────────────────────────────────────

const CATEGORY_ACCENT: Record<ActivityCategory, string> = {
  experiment: "border-primary/30 bg-primary/10 text-primary",
  concept: "border-border bg-muted text-muted-foreground",
  observation: "border-warning/30 bg-warning/10 text-warning",
  comment: "border-accent/40 bg-accent/40 text-accent-foreground",
};

const NEUTRAL_ACCENT = "border-border bg-muted text-muted-foreground";

/** The icon for a recorded activity type, matched most-specific first. */
function typeIcon(type: string): React.ReactNode {
  switch (type) {
    case "changeset.created":
      return <FlaskConical />;
    case "changeset.abandoned":
    case "changeset.superseded":
      return <Archive />;
    case "review.assigned":
      return <GitPullRequest />;
    case "review.decided":
      return <Check />;
    case "concept.term.status_changed":
      return <Lock />;
    case "concept.relation.added":
    case "concept.relation.removed":
      return <Network />;
    default:
      break;
  }
  if (type.startsWith("pilot.")) return <GitFork />;
  if (type.startsWith("concept.")) return <Pencil />;
  if (type.startsWith("observation.")) return <Quote />;
  if (type.startsWith("comment.")) return <MessageSquare />;
  if (type.startsWith("changeset.")) return <GitMerge />;
  return <Activity />;
}

function TimelineRow({
  item,
  onOpenConcept,
  onOpenExperiment,
}: {
  item: FeedItem;
  onOpenConcept?: (conceptId: string) => void;
  onOpenExperiment?: (changesetId: string) => void;
}) {
  const { nameOf } = useUserDisplayNames();
  const { changesetId, conceptId } = item;
  const open =
    changesetId && onOpenExperiment
      ? () => onOpenExperiment(changesetId)
      : conceptId && onOpenConcept
        ? () => onOpenConcept(conceptId)
        : undefined;
  const Tag = open ? "button" : "div";

  return (
    <li>
      <Tag
        type={open ? "button" : undefined}
        onClick={open}
        className={cn(
          "flex w-full items-center gap-3 px-4 py-3 text-left",
          open && "transition-colors hover:bg-muted/40",
        )}
      >
        <span
          className={cn(
            "flex size-8 shrink-0 items-center justify-center rounded-full border [&_svg]:size-4",
            item.category ? CATEGORY_ACCENT[item.category] : NEUTRAL_ACCENT,
          )}
        >
          {typeIcon(item.type)}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-foreground">{item.title}</div>
          {item.detail && <p className="truncate text-xs text-muted-foreground">{item.detail}</p>}
        </div>
        <div className="shrink-0 text-right text-xs text-muted-foreground">
          {item.actor && <div className="truncate">{nameOf(item.actor)}</div>}
          <div>{formatRelative(item.at)}</div>
        </div>
      </Tag>
    </li>
  );
}
