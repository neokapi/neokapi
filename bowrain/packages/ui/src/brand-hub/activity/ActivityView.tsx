// Activity — the brand-scoped timeline (AD-021). It weaves change-set lifecycle,
// reviews, pilots, and per-concept revisions/observations/comments into one
// chronological feed, grouped by day and filterable by event category. The
// weaving + grouping live in ./feed (pure, tested); this file is the data wiring
// and the timeline presentation.
import { useMemo, useState } from "react";
import {
  Card,
  CardContent,
  Skeleton,
  ToggleGroup,
  ToggleGroupItem,
  cn,
} from "@neokapi/ui-primitives";
import { useQueries } from "@tanstack/react-query";
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
} from "../../components/icons";
import type { ChangeSetDetail } from "../../types/brand-graph";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { useChangesets } from "../../hooks/useChangesetsApi";
import { useConcepts } from "../../hooks/useConceptsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { BrandHub } from "../shell/BrandHub";
import { EmptyState, formatRelative } from "../shell/atoms";
import {
  ACTIVITY_CATEGORIES,
  buildFeed,
  conceptDisplayName,
  groupByDay,
  type ActivityCategory,
  type FeedItem,
  type FeedKind,
} from "./feed";

const STORY_FANOUT = 12; // concepts whose stories we expand into the feed
const DETAIL_FANOUT = 15; // change-sets whose reviews + pilots we expand

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

  const { data: changesets, isLoading: csLoading } = useChangesets();
  const { data: conceptsData, isLoading: conceptsLoading } = useConcepts({ limit: 50 });

  const concepts = useMemo(() => conceptsData?.concepts ?? [], [conceptsData]);
  const conceptNames = useMemo(
    () => Object.fromEntries(concepts.map((c) => [c.id, conceptDisplayName(c)])),
    [concepts],
  );

  const storyIds = useMemo(
    () =>
      [...concepts]
        .sort(
          (a, b) =>
            new Date(b.updated_at || b.created_at).getTime() -
            new Date(a.updated_at || a.created_at).getTime(),
        )
        .slice(0, STORY_FANOUT)
        .map((c) => c.id),
    [concepts],
  );

  const changesetIds = useMemo(
    () =>
      [...(changesets ?? [])]
        .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
        .slice(0, DETAIL_FANOUT)
        .map((c) => c.id),
    [changesets],
  );

  const storyQueries = useQueries({
    queries: storyIds.map((id) => ({
      queryKey: ["concept-story", ws, id],
      queryFn: () => api.getConceptStory(ws, id),
      enabled: !!ws,
      staleTime: 15_000,
    })),
  });

  const detailQueries = useQueries({
    queries: changesetIds.map((id) => ({
      queryKey: ["changeset", ws, id],
      queryFn: () => api.getChangeset(ws, id),
      enabled: !!ws,
      staleTime: 5_000,
    })),
  });

  const stories = storyQueries.map((q, i) => ({
    conceptId: storyIds[i],
    entries: q.data?.entries ?? [],
  }));
  const details = detailQueries.map((q) => q.data).filter((d): d is ChangeSetDetail => Boolean(d));

  const feed = buildFeed({
    changesets: changesets ?? [],
    details,
    stories,
    conceptNames,
  });

  // ── Category filter ──────────────────────────────────────────────────────
  const [enabled, setEnabled] = useState<ActivityCategory[]>([...ACTIVITY_CATEGORIES]);
  const counts = useMemo(() => {
    const c: Record<ActivityCategory, number> = {
      experiment: 0,
      concept: 0,
      observation: 0,
      comment: 0,
    };
    for (const item of feed) c[item.category] += 1;
    return c;
  }, [feed]);

  const filtered = feed.filter((item) => enabled.includes(item.category));
  const groups = groupByDay(filtered);
  const isLoading = csLoading || conceptsLoading;

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
          <span className="tabular-nums text-muted-foreground">{counts[cat]}</span>
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );

  return (
    <BrandHub
      title="Activity"
      description="What's changing across your brand language — experiments moving through review, and edits to concepts, observations, and discussions."
      toolbar={feed.length > 0 ? toolbar : undefined}
    >
      {isLoading ? (
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
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={<Activity />}
          title="No matching activity"
          description="No events match the selected categories. Re-enable a filter to see more."
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
        </div>
      )}
    </BrandHub>
  );
}

// ── Row ───────────────────────────────────────────────────────────────────────

const CATEGORY_ACCENT: Record<ActivityCategory, string> = {
  experiment: "border-primary/30 bg-primary/10 text-primary",
  concept: "border-border bg-muted text-muted-foreground",
  observation: "border-warning/30 bg-warning/10 text-warning",
  comment: "border-accent/40 bg-accent/40 text-accent-foreground",
};

function kindIcon(kind: FeedKind): React.ReactNode {
  switch (kind) {
    case "changeset.opened":
      return <FlaskConical />;
    case "changeset.submitted":
      return <GitPullRequest />;
    case "changeset.merged":
      return <GitMerge />;
    case "changeset.abandoned":
      return <Archive />;
    case "changeset.reviewed":
      return <Check />;
    case "pilot.started":
      return <GitFork />;
    case "concept.revision":
      return <Pencil />;
    case "observation":
      return <Quote />;
    case "comment":
      return <MessageSquare />;
    default:
      return <Activity />;
  }
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
  const open = item.changesetId
    ? onOpenExperiment
      ? () => onOpenExperiment(item.changesetId as string)
      : undefined
    : item.conceptId
      ? onOpenConcept
        ? () => onOpenConcept(item.conceptId as string)
        : undefined
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
            CATEGORY_ACCENT[item.category],
          )}
        >
          {kindIcon(item.kind)}
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
