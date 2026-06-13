// ConceptTimeline — the concept-view evolution panel (Apache-2.0). A genuinely
// vertical product timeline: one spine, day-grouped, each event a node iconed by
// kind (a created/revised/status/relation/observation/comment/change-set) with
// its actor and a relative time. It reads the platform revision log when the
// source supplies one and otherwise degrades to a richer framework-only core
// synthesised from term and relation validity windows — so the same panel works
// against a local SQLite termbase and against bowrain's REST API.
//
// Bind it into a ConceptView timeline slot:  slots={{ timeline: (p) => <ConceptTimeline {...p} /> }}
import { useMemo, useState } from "react";
import type { ComponentType } from "react";
import { ToggleGroup, ToggleGroupItem, cn } from "@neokapi/ui-primitives";
import {
  GitCommitHorizontal,
  History,
  MessageSquare,
  PencilLine,
  Quote,
  Share2,
  Sparkles,
  Tag,
} from "lucide-react";
import type { LucideProps } from "lucide-react";
import type { ConceptSectionProps } from "./ConceptView";
import { ConceptSection, EmptyHint, ErrorHint, formatRelative } from "./atoms";
import { primaryName } from "./concept-meta";
import { buildDisplayTimeline, resolveTimelineEvents } from "./timeline-build";
import type { TimelineDisplayEvent, TimelineTone } from "./timeline-build";
import { useResource } from "./useResource";

type IconType = ComponentType<LucideProps>;

/** The icon + chip accent each tone renders with (icon is the primary signal). */
const TONE: Record<TimelineTone, { icon: IconType; chip: string }> = {
  genesis: { icon: Sparkles, chip: "bg-success/15 text-success" },
  edit: { icon: PencilLine, chip: "bg-muted text-muted-foreground" },
  status: { icon: Tag, chip: "bg-warning/15 text-warning" },
  relation: { icon: Share2, chip: "bg-primary/15 text-primary" },
  evidence: { icon: Quote, chip: "bg-primary/10 text-primary" },
  discussion: { icon: MessageSquare, chip: "bg-muted text-muted-foreground" },
  governed: { icon: GitCommitHorizontal, chip: "bg-primary/15 text-primary" },
};

export function ConceptTimeline({ concept, source, capabilities }: ConceptSectionProps) {
  const [order, setOrder] = useState<"asc" | "desc">("desc");

  // The concept's direct relations: needed for the core relation events and to
  // label a neighbour. Cheap, and always available.
  const rel = useResource(() => source.getRelations(concept.id), [source, concept.id]);
  const relations = useMemo(() => rel.data ?? [], [rel.data]);

  // Neighbour labels for relation events (core path).
  const neighbourIds = useMemo(() => {
    const ids = new Set<string>();
    for (const r of relations) {
      const other = r.sourceId === concept.id ? r.targetId : r.sourceId;
      if (other !== concept.id) ids.add(other);
    }
    return [...ids];
  }, [relations, concept.id]);

  const names = useResource(async () => {
    const entries = await Promise.all(
      neighbourIds.map(async (id) => {
        const s = source.getConceptSummary
          ? await source.getConceptSummary(id)
          : await source.getConcept(id);
        return [id, s ? primaryName(s) : id] as const;
      }),
    );
    return Object.fromEntries(entries) as Record<string, string>;
  }, [source, neighbourIds.join(",")]);

  // The platform revision log, when the source has one.
  const remote = useResource(
    () => (capabilities.timeline && source.getTimeline ? source.getTimeline(concept.id) : []),
    [source, concept.id, capabilities.timeline],
  );

  const days = useMemo(() => {
    const labelFor = (id: string) => names.data?.[id] ?? id;
    const events = resolveTimelineEvents(concept, {
      remote: remote.data ?? [],
      relations,
      labelFor,
    });
    return buildDisplayTimeline(events, order);
  }, [concept, remote.data, relations, names.data, order]);

  const total = useMemo(() => days.reduce((n, d) => n + d.events.length, 0), [days]);

  // Surface a failed fetch rather than letting it read as an empty history. The
  // core relations read and the optional revision log are the loads that matter.
  const error = rel.error ?? remote.error;

  return (
    <ConceptSection
      title="Timeline"
      icon={<History />}
      description="How this concept evolved."
      actions={
        total > 1 ? (
          <ToggleGroup
            type="single"
            size="sm"
            value={order}
            onValueChange={(v) => v && setOrder(v as "asc" | "desc")}
            aria-label="Timeline order"
          >
            <ToggleGroupItem value="desc" className="px-2 text-xs">
              Newest
            </ToggleGroupItem>
            <ToggleGroupItem value="asc" className="px-2 text-xs">
              Oldest
            </ToggleGroupItem>
          </ToggleGroup>
        ) : undefined
      }
    >
      {error ? (
        <ErrorHint title="Could not load timeline" description={error.message} />
      ) : total === 0 ? (
        <EmptyHint
          icon={<History />}
          title="No history yet"
          description="This concept has no recorded changes."
        />
      ) : (
        <div className="relative">
          {/* The continuous spine, fading at both ends. */}
          <span
            aria-hidden
            className="absolute bottom-3 left-3.5 top-3 w-px bg-gradient-to-b from-transparent via-border to-transparent"
          />
          <div className="space-y-5">
            {days.map((day) => (
              <section key={day.key}>
                <p className="pl-10 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                  {day.label}
                </p>
                <ol className="mt-2 space-y-2.5">
                  {day.events.map((event, i) => (
                    <EventRow key={event.id} event={event} index={i} />
                  ))}
                </ol>
              </section>
            ))}
          </div>
        </div>
      )}

      {!error && !capabilities.timeline && total > 0 && (
        <p className="mt-4 border-t pt-3 text-[11px] text-muted-foreground">
          Derived from the local termbase — status and relation changes inferred from validity
          windows.
        </p>
      )}
    </ConceptSection>
  );
}

function EventRow({ event, index }: { event: TimelineDisplayEvent; index: number }) {
  const tone = TONE[event.tone];
  const Icon = tone.icon;
  const isQuote = event.kind === "observation" || event.kind === "comment";
  return (
    <li
      className="grid animate-in grid-cols-[1.75rem_1fr] items-start gap-3 fade-in slide-in-from-left-1"
      style={{ animationDelay: `${Math.min(index, 8) * 40}ms` }}
    >
      <div className="relative z-10 flex justify-center">
        <span
          className={cn(
            "flex size-7 items-center justify-center rounded-full ring-4 ring-card [&_svg]:size-3.5",
            tone.chip,
          )}
        >
          <Icon aria-hidden />
        </span>
      </div>
      <div className="min-w-0 pt-1">
        <p className="text-sm leading-snug text-foreground">{event.summary}</p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {event.actor ? `${event.actor} · ` : ""}
          <time dateTime={event.at}>{formatRelative(event.at)}</time>
        </p>
        {event.detail && (
          <p
            className={cn(
              "mt-1.5 text-xs leading-relaxed text-muted-foreground",
              isQuote && "border-l-2 border-border pl-2.5 italic",
            )}
          >
            {isQuote ? `“${event.detail}”` : event.detail}
          </p>
        )}
      </div>
    </li>
  );
}
