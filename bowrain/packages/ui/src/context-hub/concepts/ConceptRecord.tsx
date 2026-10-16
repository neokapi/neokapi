// The platform's record of a concept, below its facts in the Concepts detail
// pane: its history, the discussion on it and the evidence recorded for it,
// one tab each. Kapi Desktop's Terms page has no such record, because these are
// what other people in the workspace did with the concept.
//
// A tab reads its own record when it opens, so stepping through the list with
// the keyboard reads one record per concept rather than three.
import { useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ActorMark,
  Badge,
  LoadingSpinner,
  LocaleLabel,
  SimpleTooltip,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  When,
  cn,
} from "@neokapi/ui-primitives";
import { Check, FlaskConical, MessageSquare, Pencil, Quote, type LucideIcon } from "lucide-react";
import { ErrorNotice } from "../../errors";
import { useConceptComments, useConceptStory, useObservations } from "../../hooks/useConceptsApi";
import { useUserDisplayNames, type NameResolver } from "../../hooks/useMembersApi";
import type {
  Comment,
  ConceptStoryEntry,
  ConceptStoryKind,
  Observation,
  ObservationKind,
} from "../../types/brand-graph";

export interface ConceptRecordProps {
  conceptId: string;
  /** The tab to open on. */
  defaultTab?: "history" | "discussion" | "evidence";
}

export function ConceptRecord({ conceptId, defaultTab = "history" }: ConceptRecordProps) {
  const { nameOf } = useUserDisplayNames();
  return (
    <section data-slot="concept-record" aria-label={t("Record")}>
      <Tabs defaultValue={defaultTab} className="gap-3">
        <TabsList variant="line" className="h-7 gap-3 px-0">
          <TabsTrigger value="history" data-slot="concept-record-history" className="px-0 text-xs">
            {t("History", "concept record tab")}
          </TabsTrigger>
          <TabsTrigger
            value="discussion"
            data-slot="concept-record-discussion"
            className="px-0 text-xs"
          >
            {t("Discussion", "concept record tab")}
          </TabsTrigger>
          <TabsTrigger
            value="evidence"
            data-slot="concept-record-evidence"
            className="px-0 text-xs"
          >
            {t("Evidence", "concept record tab")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="history">
          <HistoryTab conceptId={conceptId} nameOf={nameOf} />
        </TabsContent>
        <TabsContent value="discussion">
          <DiscussionTab conceptId={conceptId} nameOf={nameOf} />
        </TabsContent>
        <TabsContent value="evidence">
          <EvidenceTab conceptId={conceptId} nameOf={nameOf} />
        </TabsContent>
      </Tabs>
    </section>
  );
}

// ── History ──────────────────────────────────────────────────────────────────

const STORY_ICONS: Record<ConceptStoryKind, LucideIcon> = {
  revision: Pencil,
  observation: Quote,
  comment: MessageSquare,
  changeset: FlaskConical,
};

function storyKindLabel(kind: ConceptStoryKind): string {
  switch (kind) {
    case "revision":
      return t("Revision", "concept story kind");
    case "observation":
      return t("Evidence", "concept story kind");
    case "comment":
      return t("Comment", "concept story kind");
    case "changeset":
      return t("Change-set", "concept story kind");
    default:
      return kind;
  }
}

function HistoryTab({ conceptId, nameOf }: { conceptId: string; nameOf: NameResolver }) {
  const story = useConceptStory(conceptId);
  // Newest first, the way the rest of the platform reads a record.
  const entries = useMemo(
    () => [...(story.data?.entries ?? [])].sort((a, b) => b.at.localeCompare(a.at)),
    [story.data],
  );
  if (story.error) {
    return (
      <ErrorNotice
        variant="inline"
        error={story.error}
        title={t("The history could not be read")}
      />
    );
  }
  if (story.isLoading) return <LoadingSpinner />;
  if (entries.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        Nothing has been recorded about this concept yet.
      </p>
    );
  }
  return (
    <ul data-slot="concept-history" className="space-y-1.5">
      {entries.map((entry, i) => (
        <HistoryLine key={`${entry.kind}:${entry.at}:${i}`} entry={entry} nameOf={nameOf} />
      ))}
    </ul>
  );
}

function HistoryLine({ entry, nameOf }: { entry: ConceptStoryEntry; nameOf: NameResolver }) {
  const Icon = STORY_ICONS[entry.kind] ?? Pencil;
  const who = entry.actor ? nameOf(entry.actor) : "";
  return (
    <li
      data-slot="concept-history-entry"
      data-kind={entry.kind}
      className="flex min-w-0 items-start gap-2 text-sm"
    >
      <SimpleTooltip content={storyKindLabel(entry.kind)}>
        <span
          role="img"
          aria-label={storyKindLabel(entry.kind)}
          className="mt-0.5 inline-flex text-muted-foreground"
        >
          <Icon aria-hidden className="size-3.5" />
        </span>
      </SimpleTooltip>
      <div className="min-w-0 flex-1">
        <p className="min-w-0 break-words" translate="no">
          {entry.summary || storyKindLabel(entry.kind)}
        </p>
        <p className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted-foreground">
          {who && (
            <>
              <ActorMark actor="person" name={who} size="sm" decorative />
              <span translate="no">{who}</span>
              <span aria-hidden>·</span>
            </>
          )}
          <When iso={entry.at} relative />
        </p>
      </div>
    </li>
  );
}

// ── Discussion ───────────────────────────────────────────────────────────────

interface Thread {
  comment: Comment;
  replies: Thread[];
}

/** Comments as threads, parents before their replies, each level in the order posted. */
export function threadsOf(comments: Comment[]): Thread[] {
  const byId = new Map<string, Thread>();
  for (const comment of comments) byId.set(comment.id, { comment, replies: [] });
  const roots: Thread[] = [];
  for (const comment of comments) {
    const node = byId.get(comment.id)!;
    const parent = comment.parent_id ? byId.get(comment.parent_id) : undefined;
    if (parent) parent.replies.push(node);
    else roots.push(node);
  }
  return roots;
}

function DiscussionTab({ conceptId, nameOf }: { conceptId: string; nameOf: NameResolver }) {
  const comments = useConceptComments(conceptId);
  const threads = useMemo(() => threadsOf(comments.data ?? []), [comments.data]);
  if (comments.error) {
    return (
      <ErrorNotice
        variant="inline"
        error={comments.error}
        title={t("The discussion could not be read")}
      />
    );
  }
  if (comments.isLoading) return <LoadingSpinner />;
  if (threads.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">Nobody has commented on this concept yet.</p>
    );
  }
  return (
    <ul data-slot="concept-discussion" className="space-y-3">
      {threads.map((thread) => (
        <CommentLine key={thread.comment.id} thread={thread} depth={0} nameOf={nameOf} />
      ))}
    </ul>
  );
}

function CommentLine({
  thread,
  depth,
  nameOf,
}: {
  thread: Thread;
  depth: number;
  nameOf: NameResolver;
}) {
  const { comment, replies } = thread;
  const who = nameOf(comment.author);
  return (
    <li data-slot="concept-comment" className={cn(depth > 0 && "ml-5 border-l border-border pl-3")}>
      <p className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted-foreground">
        <ActorMark actor="person" name={who} size="sm" decorative />
        <span className="text-foreground" translate="no">
          {who}
        </span>
        <span aria-hidden>·</span>
        <When iso={comment.created_at} relative />
        {comment.resolved && (
          <span className="inline-flex items-center gap-0.5 text-success">
            <Check aria-hidden className="size-3" />
            Resolved
          </span>
        )}
      </p>
      <p className="mt-0.5 max-w-prose text-sm break-words whitespace-pre-wrap" translate="no">
        {comment.body}
      </p>
      {replies.length > 0 && (
        <ul className="mt-2 space-y-2">
          {replies.map((reply) => (
            <CommentLine key={reply.comment.id} thread={reply} depth={depth + 1} nameOf={nameOf} />
          ))}
        </ul>
      )}
    </li>
  );
}

// ── Evidence ─────────────────────────────────────────────────────────────────

function observationKindLabel(kind: ObservationKind): string {
  switch (kind) {
    case "competitor":
      return t("Competitor", "observation kind");
    case "customer":
      return t("Customer", "observation kind");
    case "style_guide":
      return t("Style guide", "observation kind");
    case "regulatory":
      return t("Regulatory", "observation kind");
    case "web":
      return t("Web", "observation kind");
    case "internal":
      return t("Internal", "observation kind");
    default:
      return kind;
  }
}

function EvidenceTab({ conceptId, nameOf }: { conceptId: string; nameOf: NameResolver }) {
  const observations = useObservations(conceptId);
  const items = observations.data ?? [];
  if (observations.error) {
    return (
      <ErrorNotice
        variant="inline"
        error={observations.error}
        title={t("The evidence could not be read")}
      />
    );
  }
  if (observations.isLoading) return <LoadingSpinner />;
  if (items.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No evidence is recorded for this concept.</p>
    );
  }
  return (
    <ul data-slot="concept-evidence" className="space-y-3">
      {items.map((observation) => (
        <EvidenceLine key={observation.id} observation={observation} nameOf={nameOf} />
      ))}
    </ul>
  );
}

function EvidenceLine({ observation, nameOf }: { observation: Observation; nameOf: NameResolver }) {
  const who = observation.created_by ? nameOf(observation.created_by) : "";
  return (
    <li data-slot="concept-observation" data-kind={observation.kind} className="space-y-1">
      <blockquote className="max-w-prose border-l-2 border-border pl-2 text-sm" translate="no">
        {observation.quote}
      </blockquote>
      <p className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 pl-2.5 text-[11px] text-muted-foreground">
        <Badge variant="outline" className="h-4 px-1.5 text-[11px]">
          {observationKindLabel(observation.kind)}
        </Badge>
        {observation.url ? (
          <a
            href={observation.url}
            target="_blank"
            rel="noreferrer"
            className="min-w-0 truncate underline-offset-2 hover:text-foreground hover:underline"
            translate="no"
          >
            {observation.source}
          </a>
        ) : (
          <span className="min-w-0 truncate" translate="no">
            {observation.source}
          </span>
        )}
        {observation.locale && <LocaleLabel locale={observation.locale} compact />}
        {observation.market && <span translate="no">{observation.market}</span>}
        {who && (
          <>
            <span aria-hidden>·</span>
            <ActorMark actor="person" name={who} size="sm" decorative />
            <span translate="no">{who}</span>
          </>
        )}
        <span aria-hidden>·</span>
        <When iso={observation.created_at} relative />
      </p>
      {observation.note && (
        <p className="pl-2.5 text-xs text-muted-foreground" translate="no">
          {observation.note}
        </p>
      )}
    </li>
  );
}
