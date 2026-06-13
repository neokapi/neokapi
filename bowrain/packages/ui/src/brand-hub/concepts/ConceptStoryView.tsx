// The concept story — one page that tells the whole story of a concept (AD-021):
// its terms grouped by market, a typed relations explorer, external
// observations, a threaded discussion (with replies and @mentions), and a single
// day-grouped timeline merging revisions, status transitions, observations,
// comments, and change-sets.
import { useMemo, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Textarea,
  cn,
} from "@neokapi/ui-primitives";
import {
  ArrowLeft,
  ArrowRight,
  Plus,
  Trash2,
  Check,
  MessageSquare,
  Quote,
  GitFork,
  ScrollText,
  FlaskConical,
  Pencil,
  Sparkles,
  Globe,
} from "../../components/icons";
import { Reply } from "lucide-react";
import type { ConceptInfo, TermInfo } from "../../types/api";
import type {
  Observation,
  Comment as ConceptComment,
  ObservationKind,
  RelationType,
  TermStatus,
} from "../../types/brand-graph";
import { RELATION_TYPES, OBSERVATION_KINDS } from "../../types/brand-graph";
import {
  useConcept,
  useConcepts,
  useConceptStory,
  useConceptRelations,
  useAddConceptRelation,
  useDeleteConceptRelation,
  useObservations,
  useAddObservation,
  useDeleteObservation,
  useConceptComments,
  useAddConceptComment,
  useResolveConceptComment,
  useDeleteConceptComment,
  useConceptBlastRadius,
} from "../../hooks/useConceptsApi";
import { useMarkets } from "../../hooks/useMarketsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { BrandHub } from "../shell/BrandHub";
import {
  TermStatusBadge,
  RelationBadge,
  relationLabel,
  EmptyState,
  formatRelative,
  formatDate,
} from "../shell/atoms";
import {
  buildStoryTimeline,
  termsByMarket,
  groupRelations,
  type StoryTone,
  type RelationGroup,
} from "./story-timeline";

export interface ConceptStoryViewProps {
  conceptId: string;
  onBack: () => void;
  /** Navigate to another concept (e.g. a relation target). */
  onOpenConcept?: (conceptId: string) => void;
}

export function ConceptStoryView({ conceptId, onBack, onOpenConcept }: ConceptStoryViewProps) {
  const { data: concept, isLoading } = useConcept(conceptId);

  if (isLoading) {
    return (
      <BrandHub title="Concept" width="wide">
        <div className="space-y-3">
          <Skeleton className="h-6 w-56" />
          <Skeleton className="h-4 w-80" />
          <Skeleton className="h-40 w-full" />
        </div>
      </BrandHub>
    );
  }

  if (!concept) {
    return (
      <BrandHub title="Concept" width="wide">
        <EmptyState
          title="Concept not found"
          description="This concept may have been deleted or merged."
          action={
            <Button variant="outline" size="sm" onClick={onBack}>
              <ArrowLeft />
              Back to concepts
            </Button>
          }
        />
      </BrandHub>
    );
  }

  return (
    <BrandHub
      title={primaryName(concept)}
      description={concept.definition || undefined}
      width="wide"
      actions={
        <Button variant="outline" size="sm" onClick={onBack}>
          <ArrowLeft />
          Concepts
        </Button>
      }
      toolbar={<ConceptMeta concept={concept} />}
    >
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="relations">Relations</TabsTrigger>
          <TabsTrigger value="observations">Observations</TabsTrigger>
          <TabsTrigger value="story">Story</TabsTrigger>
          <TabsTrigger value="discussion">Discussion</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4">
          <div className="grid gap-4 lg:grid-cols-3">
            <Card className="lg:col-span-2">
              <CardContent className="p-4">
                <TermsByMarket terms={concept.terms} />
              </CardContent>
            </Card>
            <WhereUsedCard conceptId={conceptId} />
          </div>
        </TabsContent>

        <TabsContent value="relations" className="mt-4">
          <RelationsSection conceptId={conceptId} onOpenConcept={onOpenConcept} />
        </TabsContent>

        <TabsContent value="observations" className="mt-4">
          <ObservationsSection conceptId={conceptId} />
        </TabsContent>

        <TabsContent value="story" className="mt-4">
          <StorySection conceptId={conceptId} />
        </TabsContent>

        <TabsContent value="discussion" className="mt-4">
          <DiscussionSection conceptId={conceptId} />
        </TabsContent>
      </Tabs>
    </BrandHub>
  );
}

// ── Header meta ──────────────────────────────────────────────────────────────

function ConceptMeta({ concept }: { concept: ConceptInfo }) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      {concept.domain && (
        <Badge variant="outline" className="font-normal">
          {concept.domain}
        </Badge>
      )}
      <span>
        {concept.terms.length} term{concept.terms.length === 1 ? "" : "s"}
      </span>
      <span aria-hidden>·</span>
      <span className="font-mono">{concept.id}</span>
    </div>
  );
}

function WhereUsedCard({ conceptId }: { conceptId: string }) {
  const { data, isLoading } = useConceptBlastRadius(conceptId);
  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <h3 className="text-sm font-medium">Where used</h3>
        {isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : data ? (
          <div className="grid grid-cols-3 gap-2 text-center">
            <Stat label="Blocks" value={data.total_blocks} />
            <Stat label="Uses" value={data.occurrences} />
            <Stat label="Words" value={data.words} />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No usage data.</p>
        )}
        {data && data.projects.length > 0 && (
          <ul className="space-y-1 pt-1">
            {data.projects.slice(0, 5).map((p) => (
              <li key={p.project_id} className="flex justify-between text-xs">
                <span className="truncate text-muted-foreground">{p.project_name}</span>
                <span className="tabular-nums">{p.blocks}</span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border bg-muted/30 py-2">
      <div className="text-base font-semibold tabular-nums">{value.toLocaleString()}</div>
      <div className="text-[11px] text-muted-foreground">{label}</div>
    </div>
  );
}

// ── Terms by market / locale ─────────────────────────────────────────────────

function TermsByMarket({ terms }: { terms: TermInfo[] }) {
  const { data: markets } = useMarkets();
  const groups = useMemo(() => termsByMarket(terms, markets ?? []), [terms, markets]);

  if (terms.length === 0) {
    return <p className="text-sm text-muted-foreground">No terms on this concept yet.</p>;
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-medium">Terms by market</h3>
        <span className="text-xs text-muted-foreground">
          how this concept is said, and its status
        </span>
      </div>
      <div className="space-y-4 pt-1">
        {groups.map((group) => (
          <div key={group.market?.id ?? "other"} className="space-y-2">
            <div className="flex items-center gap-2">
              <Globe className="size-3.5 text-muted-foreground" />
              <span className="text-sm font-medium">{group.name}</span>
              {group.market && (
                <span className="text-xs text-muted-foreground">
                  {group.market.locales.join(", ")}
                </span>
              )}
            </div>
            <ul className="space-y-1.5 border-l pl-4">
              {group.locales.map(({ locale, terms: localeTerms }) => (
                <li key={locale} className="flex flex-wrap items-center gap-2">
                  <span className="w-16 shrink-0 font-mono text-xs text-muted-foreground">
                    {locale}
                  </span>
                  {localeTerms.map((t, i) => (
                    <span key={`${t.text}-${i}`} className="flex items-center gap-1.5">
                      <span className="font-medium text-foreground">{t.text}</span>
                      <TermStatusBadge
                        status={(t.status as TermStatus) ?? "proposed"}
                        className="text-[10px]"
                      />
                      {t.part_of_speech && (
                        <span className="text-xs italic text-muted-foreground">
                          {t.part_of_speech}
                        </span>
                      )}
                    </span>
                  ))}
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </div>
  );
}

// ── Relations ────────────────────────────────────────────────────────────────

function RelationsSection({
  conceptId,
  onOpenConcept,
}: {
  conceptId: string;
  onOpenConcept?: (id: string) => void;
}) {
  const { data: relations, isLoading } = useConceptRelations(conceptId);
  const { data: conceptList } = useConcepts({ limit: 200 });
  const del = useDeleteConceptRelation(conceptId);
  const [addOpen, setAddOpen] = useState(false);

  const labelOf = useMemo(() => {
    const map = new Map<string, string>();
    for (const c of conceptList?.concepts ?? []) map.set(c.id, primaryName(c));
    return map;
  }, [conceptList]);

  const groups = useMemo(() => groupRelations(relations ?? [], conceptId), [relations, conceptId]);

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
          <Plus />
          Add relation
        </Button>
      </div>
      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : groups.length === 0 ? (
        <EmptyState
          icon={<GitFork />}
          title="No relations"
          description="Connect this concept to others — what it replaces, what it's part of, or a competitor's term."
        />
      ) : (
        <div className="space-y-4">
          {groups.map((group) => (
            <RelationGroupCard
              key={group.type}
              group={group}
              labelOf={labelOf}
              onOpenConcept={onOpenConcept}
              onDelete={(id) => del.mutate(id)}
              deletingId={del.isPending ? (del.variables as string | undefined) : undefined}
            />
          ))}
        </div>
      )}
      <AddRelationDialog conceptId={conceptId} open={addOpen} onOpenChange={setAddOpen} />
    </div>
  );
}

function RelationGroupCard({
  group,
  labelOf,
  onOpenConcept,
  onDelete,
  deletingId,
}: {
  group: RelationGroup;
  labelOf: Map<string, string>;
  onOpenConcept?: (id: string) => void;
  onDelete: (relationId: string) => void;
  deletingId?: string;
}) {
  return (
    <section className="overflow-hidden rounded-lg border">
      <header className="flex items-center gap-2 border-b bg-muted/30 px-4 py-2">
        <RelationBadge type={group.type} />
        <span className="text-xs text-muted-foreground">
          {group.items.length} concept{group.items.length === 1 ? "" : "s"}
        </span>
      </header>
      <ul className="divide-y">
        {group.items.map(({ relation, otherId, outgoing }) => (
          <li key={relation.id} className="flex items-center gap-3 px-4 py-2.5">
            {outgoing ? (
              <ArrowRight className="size-3.5 shrink-0 text-muted-foreground" />
            ) : (
              <ArrowLeft className="size-3.5 shrink-0 text-muted-foreground" />
            )}
            <button
              type="button"
              onClick={() => onOpenConcept?.(otherId)}
              disabled={!onOpenConcept}
              className={cn(
                "min-w-0 flex-1 truncate text-left text-sm text-foreground",
                onOpenConcept && "hover:underline",
              )}
              title={otherId}
            >
              {labelOf.get(otherId) ?? otherId}
            </button>
            {relation.note && (
              <span className="hidden truncate text-xs text-muted-foreground sm:block">
                {relation.note}
              </span>
            )}
            <Button
              size="icon"
              variant="ghost"
              className="size-7 text-muted-foreground hover:text-destructive"
              onClick={() => onDelete(relation.id)}
              disabled={deletingId === relation.id}
              aria-label="Remove relation"
            >
              <Trash2 />
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}

function AddRelationDialog({
  conceptId,
  open,
  onOpenChange,
}: {
  conceptId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const add = useAddConceptRelation(conceptId);
  const { data } = useConcepts({ limit: 50 });
  const [relationType, setRelationType] = useState<RelationType>("RELATED");
  const [targetId, setTargetId] = useState("");
  const [note, setNote] = useState("");

  const targets = (data?.concepts ?? []).filter((c) => c.id !== conceptId);
  const canSubmit = targetId.length > 0 && !add.isPending;

  const submit = () => {
    if (!canSubmit) return;
    add.mutate(
      { target_id: targetId, relation_type: relationType, note: note.trim() || undefined },
      {
        onSuccess: () => {
          setTargetId("");
          setNote("");
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add relation</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label>Relation</Label>
            <Select value={relationType} onValueChange={(v) => setRelationType(v as RelationType)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RELATION_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {relationLabel(t)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label>Target concept</Label>
            <Select value={targetId} onValueChange={setTargetId}>
              <SelectTrigger>
                <SelectValue placeholder="Choose a concept…" />
              </SelectTrigger>
              <SelectContent>
                {targets.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {primaryName(c)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="rel-note">Note</Label>
            <Input
              id="rel-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="optional"
            />
          </div>
          {add.isError && (
            <p className="text-sm text-destructive">
              {add.error instanceof Error ? add.error.message : "Could not add relation."}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit}>
            {add.isPending ? "Adding…" : "Add relation"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ── Observations ─────────────────────────────────────────────────────────────

function ObservationsSection({ conceptId }: { conceptId: string }) {
  const { data: observations, isLoading } = useObservations(conceptId);
  const del = useDeleteObservation(conceptId);
  const [addOpen, setAddOpen] = useState(false);

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
          <Plus />
          Add observation
        </Button>
      </div>
      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : !observations || observations.length === 0 ? (
        <EmptyState
          icon={<Quote />}
          title="No observations"
          description="Record how others use this term — a competitor's phrasing, customer language, a style-guide citation. Evidence, not rules."
        />
      ) : (
        <ul className="space-y-2">
          {observations.map((o) => (
            <ObservationCard
              key={o.id}
              observation={o}
              onDelete={() => del.mutate(o.id)}
              deleting={del.isPending && del.variables === o.id}
            />
          ))}
        </ul>
      )}
      <AddObservationDialog conceptId={conceptId} open={addOpen} onOpenChange={setAddOpen} />
    </div>
  );
}

function ObservationCard({
  observation,
  onDelete,
  deleting,
}: {
  observation: Observation;
  onDelete: () => void;
  deleting: boolean;
}) {
  return (
    <li className="rounded-lg border p-3">
      <div className="flex items-start gap-3">
        <Badge variant="outline" className="shrink-0 capitalize">
          {observation.kind.replace(/_/g, " ")}
        </Badge>
        <blockquote className="min-w-0 flex-1 border-l-2 pl-3 text-sm text-foreground">
          {observation.quote}
        </blockquote>
        <Button
          size="icon"
          variant="ghost"
          className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
          onClick={onDelete}
          disabled={deleting}
          aria-label="Remove observation"
        >
          <Trash2 />
        </Button>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 pl-[3.25rem] text-xs text-muted-foreground">
        <span>{observation.source}</span>
        {observation.market && <span>· {observation.market}</span>}
        {observation.locale && <span className="font-mono">· {observation.locale}</span>}
        {observation.url && (
          <a
            href={observation.url}
            target="_blank"
            rel="noreferrer"
            className="text-primary hover:underline"
          >
            source link
          </a>
        )}
      </div>
    </li>
  );
}

function AddObservationDialog({
  conceptId,
  open,
  onOpenChange,
}: {
  conceptId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const add = useAddObservation(conceptId);
  const [kind, setKind] = useState<ObservationKind>("competitor");
  const [quote, setQuote] = useState("");
  const [source, setSource] = useState("");
  const [url, setUrl] = useState("");

  const canSubmit = quote.trim().length > 0 && source.trim().length > 0 && !add.isPending;

  const submit = () => {
    if (!canSubmit) return;
    add.mutate(
      { kind, quote: quote.trim(), source: source.trim(), url: url.trim() || undefined },
      {
        onSuccess: () => {
          setQuote("");
          setSource("");
          setUrl("");
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add observation</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label>Kind</Label>
            <Select value={kind} onValueChange={(v) => setKind(v as ObservationKind)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {OBSERVATION_KINDS.map((k) => (
                  <SelectItem key={k} value={k} className="capitalize">
                    {k.replace(/_/g, " ")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="obs-quote">Quote</Label>
            <Textarea
              id="obs-quote"
              value={quote}
              onChange={(e) => setQuote(e.target.value)}
              rows={2}
              placeholder="The exact phrasing observed."
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="obs-source">Source</Label>
              <Input
                id="obs-source"
                value={source}
                onChange={(e) => setSource(e.target.value)}
                placeholder="e.g. Acme website"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="obs-url">URL</Label>
              <Input
                id="obs-url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="optional"
              />
            </div>
          </div>
          {add.isError && (
            <p className="text-sm text-destructive">
              {add.error instanceof Error ? add.error.message : "Could not add observation."}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit}>
            {add.isPending ? "Adding…" : "Add observation"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ── Story timeline ───────────────────────────────────────────────────────────

const TONE_ICON: Record<StoryTone, React.ReactNode> = {
  create: <Sparkles />,
  revision: <Pencil />,
  observation: <Quote />,
  comment: <MessageSquare />,
  changeset: <FlaskConical />,
};

const TONE_RING: Record<StoryTone, string> = {
  create: "border-success/40 bg-success/10 text-success",
  revision: "border-border bg-background text-muted-foreground",
  observation: "border-accent-foreground/30 bg-accent text-accent-foreground",
  comment: "border-border bg-background text-muted-foreground",
  changeset: "border-primary/40 bg-primary/10 text-primary",
};

function StorySection({ conceptId }: { conceptId: string }) {
  const { data: story, isLoading } = useConceptStory(conceptId);
  const { nameOf } = useUserDisplayNames();
  const [order, setOrder] = useState<"asc" | "desc">("asc");

  const groups = useMemo(() => buildStoryTimeline(story?.entries ?? [], order), [story, order]);

  if (isLoading) return <Skeleton className="h-48 w-full" />;
  if (!story || story.entries.length === 0) {
    return (
      <EmptyState
        icon={<ScrollText />}
        title="No history yet"
        description="Edits, observations, comments, and change-sets that touch this concept appear here in order."
      />
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {story.entries.length} event{story.entries.length === 1 ? "" : "s"} on this concept
        </p>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setOrder((o) => (o === "asc" ? "desc" : "asc"))}
        >
          {order === "asc" ? "Oldest first" : "Newest first"}
        </Button>
      </div>

      <div className="space-y-6">
        {groups.map((group) => (
          <div key={group.key} className="space-y-3">
            <div className="flex items-center gap-3">
              <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                {group.label}
              </span>
              <span className="h-px flex-1 bg-border" />
            </div>
            <ol className="relative ml-2 space-y-4 border-l pl-6">
              {group.entries.map((entry) => (
                <li key={entry.id} className="relative">
                  <span
                    className={cn(
                      "absolute -left-[2.05rem] flex size-7 items-center justify-center rounded-full border [&_svg]:size-3.5",
                      TONE_RING[entry.tone],
                    )}
                  >
                    {TONE_ICON[entry.tone]}
                  </span>
                  <div className="flex flex-wrap items-center gap-2">
                    {entry.actor && <ActorAvatar name={nameOf(entry.actor)} />}
                    <span className="text-sm font-medium text-foreground">{entry.title}</span>
                    <span className="text-xs text-muted-foreground" title={formatDate(entry.at)}>
                      {formatRelative(entry.at)}
                    </span>
                  </div>
                  {entry.detail && (
                    <p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">
                      {entry.detail}
                    </p>
                  )}
                </li>
              ))}
            </ol>
          </div>
        ))}
      </div>
    </div>
  );
}

function ActorAvatar({ name }: { name: string }) {
  const initials = name
    .split(/[\s@._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase())
    .join("");
  return (
    <span className="flex items-center gap-1.5">
      <span className="flex size-5 items-center justify-center rounded-full bg-muted text-[10px] font-medium text-muted-foreground">
        {initials || "?"}
      </span>
      <span className="text-sm font-medium text-foreground">{name}</span>
    </span>
  );
}

// ── Discussion ───────────────────────────────────────────────────────────────

function DiscussionSection({ conceptId }: { conceptId: string }) {
  const { data: comments, isLoading } = useConceptComments(conceptId);
  const add = useAddConceptComment(conceptId);
  const [body, setBody] = useState("");
  const [replyTo, setReplyTo] = useState<string | null>(null);

  const threaded = useMemo(() => threadComments(comments ?? []), [comments]);

  const submit = () => {
    if (!body.trim() || add.isPending) return;
    add.mutate({ body: body.trim() }, { onSuccess: () => setBody("") });
  };

  return (
    <div className="space-y-4">
      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : threaded.length === 0 ? (
        <EmptyState
          icon={<MessageSquare />}
          title="No discussion yet"
          description="Discuss this concept where the decision will be made. Resolved threads stay part of the story."
        />
      ) : (
        <ul className="space-y-3">
          {threaded.map(({ comment, replies }) => (
            <li key={comment.id} className="space-y-3">
              <CommentCard
                conceptId={conceptId}
                comment={comment}
                onReply={() => setReplyTo((id) => (id === comment.id ? null : comment.id))}
              />
              {(replies.length > 0 || replyTo === comment.id) && (
                <ul className="ml-6 space-y-3 border-l pl-4">
                  {replies.map((r) => (
                    <li key={r.id}>
                      <CommentCard conceptId={conceptId} comment={r} />
                    </li>
                  ))}
                  {replyTo === comment.id && (
                    <li>
                      <ReplyComposer
                        conceptId={conceptId}
                        parentId={comment.id}
                        onDone={() => setReplyTo(null)}
                      />
                    </li>
                  )}
                </ul>
              )}
            </li>
          ))}
        </ul>
      )}

      <div className="space-y-2 rounded-lg border p-3">
        <Textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={2}
          placeholder="Add a comment… use @name to mention a teammate"
        />
        <div className="flex justify-end">
          <Button size="sm" onClick={submit} disabled={!body.trim() || add.isPending}>
            {add.isPending ? "Posting…" : "Comment"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function ReplyComposer({
  conceptId,
  parentId,
  onDone,
}: {
  conceptId: string;
  parentId: string;
  onDone: () => void;
}) {
  const add = useAddConceptComment(conceptId);
  const [body, setBody] = useState("");

  const submit = () => {
    if (!body.trim() || add.isPending) return;
    add.mutate(
      { body: body.trim(), parent_id: parentId },
      {
        onSuccess: () => {
          setBody("");
          onDone();
        },
      },
    );
  };

  return (
    <div className="space-y-2 rounded-lg border bg-muted/20 p-3">
      <Textarea
        value={body}
        onChange={(e) => setBody(e.target.value)}
        rows={2}
        autoFocus
        placeholder="Write a reply…"
      />
      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm" onClick={onDone}>
          Cancel
        </Button>
        <Button size="sm" onClick={submit} disabled={!body.trim() || add.isPending}>
          {add.isPending ? "Replying…" : "Reply"}
        </Button>
      </div>
    </div>
  );
}

function CommentCard({
  conceptId,
  comment,
  onReply,
}: {
  conceptId: string;
  comment: ConceptComment;
  onReply?: () => void;
}) {
  const resolve = useResolveConceptComment(conceptId);
  const del = useDeleteConceptComment(conceptId);
  const { nameOf } = useUserDisplayNames();
  return (
    <div className={cn("rounded-lg border p-3", comment.resolved && "bg-muted/40 opacity-80")}>
      <div className="flex items-center gap-2">
        <ActorAvatar name={nameOf(comment.author)} />
        <span className="text-xs text-muted-foreground" title={formatDate(comment.created_at)}>
          {formatRelative(comment.created_at)}
        </span>
        {comment.resolved && (
          <Badge variant="outline" className="text-[10px] text-success">
            Resolved
          </Badge>
        )}
        <div className="ml-auto flex items-center gap-1">
          {onReply && (
            <Button
              size="icon"
              variant="ghost"
              className="size-7 text-muted-foreground hover:text-foreground"
              onClick={onReply}
              aria-label="Reply"
              title="Reply"
            >
              <Reply className="size-4" />
            </Button>
          )}
          <Button
            size="icon"
            variant="ghost"
            className="size-7 text-muted-foreground hover:text-foreground"
            onClick={() => resolve.mutate({ commentId: comment.id, resolved: !comment.resolved })}
            disabled={resolve.isPending}
            aria-label={comment.resolved ? "Reopen" : "Resolve"}
            title={comment.resolved ? "Reopen" : "Resolve"}
          >
            <Check />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            className="size-7 text-muted-foreground hover:text-destructive"
            onClick={() => del.mutate(comment.id)}
            disabled={del.isPending}
            aria-label="Delete comment"
          >
            <Trash2 />
          </Button>
        </div>
      </div>
      <p className="mt-1.5 whitespace-pre-wrap text-sm text-foreground">
        {renderMentions(comment.body)}
      </p>
    </div>
  );
}

/** Render @mentions as subtly emphasised spans (display-only). */
function renderMentions(body: string): React.ReactNode {
  const parts = body.split(/(@[\w.-]+)/g);
  return parts.map((part, i) =>
    /^@[\w.-]+$/.test(part) ? (
      <span key={i} className="font-medium text-primary">
        {part}
      </span>
    ) : (
      <span key={i}>{part}</span>
    ),
  );
}

// ── helpers ──────────────────────────────────────────────────────────────────

function threadComments(
  comments: ConceptComment[],
): { comment: ConceptComment; replies: ConceptComment[] }[] {
  const tops = comments.filter((c) => !c.parent_id);
  const repliesByParent = new Map<string, ConceptComment[]>();
  for (const c of comments) {
    if (c.parent_id) {
      const arr = repliesByParent.get(c.parent_id) ?? [];
      arr.push(c);
      repliesByParent.set(c.parent_id, arr);
    }
  }
  return tops.map((comment) => ({ comment, replies: repliesByParent.get(comment.id) ?? [] }));
}

function primaryName(concept: ConceptInfo): string {
  if (concept.terms.length === 0) return concept.domain || concept.id;
  const preferred = concept.terms.find((t) => t.status === "preferred");
  const english = concept.terms.find((t) => t.locale.startsWith("en"));
  return (preferred ?? english ?? concept.terms[0]).text;
}
