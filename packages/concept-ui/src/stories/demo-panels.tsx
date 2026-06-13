// Illustrative section-slot renderers for the Storybook stories (Apache-2.0).
// These are DEMOS — they show how a panel agent fills a ConceptView slot using
// the package's exported pure logic + atoms, and give reviewers something to
// see. The production Geography / Relations / Timeline / Constraints panels land
// as their own files; treat these as a contract reference, not the final UI.
import { useMemo, useState } from "react";
import { Badge, Button, cn } from "@neokapi/ui-primitives";
import { ChevronRight, Quote } from "lucide-react";
import type { ConceptSectionProps } from "../ConceptView";
import {
  ConceptSection,
  EmptyHint,
  LocalePill,
  RelationChip,
  StatusChip,
  formatDate,
  formatRelative,
} from "../atoms";
import {
  RELATION_COLLAPSE_THRESHOLD,
  deriveMarketsFromTerms,
  groupRelations,
  primaryName,
  shouldCollapse,
  termsByMarket,
  isBannedStatus,
} from "../index";
import { buildTimeline, synthesizeTimeline } from "../timeline";
import type { Market } from "../types";
import { useResource } from "../useResource";

// ── Relations (local widget, with collapse) ──────────────────────────────────

export function DemoRelations({ concept, source, onNavigate }: ConceptSectionProps) {
  const rel = useResource(() => source.getRelations(concept.id), [source, concept.id]);
  const groups = useMemo(() => groupRelations(rel.data ?? [], concept.id), [rel.data, concept.id]);
  const ids = useMemo(
    () => [...new Set(groups.flatMap((g) => g.items.map((i) => i.otherId)))],
    [groups],
  );
  const names = useResource(async () => {
    const entries = await Promise.all(
      ids.map(async (id) => {
        const s = source.getConceptSummary
          ? await source.getConceptSummary(id)
          : await source.getConcept(id);
        return [id, s ? primaryName(s) : id] as const;
      }),
    );
    return Object.fromEntries(entries) as Record<string, string>;
  }, [source, ids.join(",")]);
  const label = (id: string) => names.data?.[id] ?? id;

  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const toggle = (type: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(type)) next.delete(type);
      else next.add(type);
      return next;
    });

  return (
    <ConceptSection title="Relations" description="This concept and its direct relations.">
      {groups.length === 0 ? (
        <EmptyHint title="No relations" description="This concept stands on its own." />
      ) : (
        <ul className="space-y-3">
          {groups.map((g) => {
            const collapsed = shouldCollapse(g) && !expanded.has(g.type);
            return (
              <li key={g.type} className="space-y-1.5">
                <div className="flex items-center gap-2">
                  <RelationChip type={g.type} />
                  <span className="text-xs text-muted-foreground">{g.items.length}</span>
                </div>
                {collapsed ? (
                  <button
                    type="button"
                    onClick={() => toggle(g.type)}
                    className="flex w-full items-center justify-between rounded-md border bg-muted/30 px-3 py-2 text-left text-sm transition-colors hover:bg-muted/60"
                  >
                    <span className="text-foreground">{g.items.length} related concepts</span>
                    <ChevronRight className="size-4 text-muted-foreground" />
                  </button>
                ) : (
                  <div className="flex flex-wrap gap-1.5">
                    {g.items.map((it) => (
                      <button
                        key={it.relation.id}
                        type="button"
                        onClick={() => onNavigate(it.otherId)}
                        className={cn(
                          "inline-flex items-center gap-1 rounded-full border bg-background px-2.5 py-1 text-xs transition-colors hover:border-primary/50 hover:bg-primary/5",
                          !it.outgoing && "border-dashed",
                        )}
                        title={it.outgoing ? "Outgoing" : "Incoming"}
                      >
                        {label(it.otherId)}
                      </button>
                    ))}
                    {shouldCollapse(g) && (
                      <button
                        type="button"
                        onClick={() => toggle(g.type)}
                        className="text-xs text-muted-foreground underline-offset-2 hover:underline"
                      >
                        collapse
                      </button>
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      <p className="mt-3 text-[11px] text-muted-foreground">
        Groups of more than {RELATION_COLLAPSE_THRESHOLD} collapse; a related concept re-centres the
        view.
      </p>
    </ConceptSection>
  );
}

// ── Geography (market panels) ────────────────────────────────────────────────

export function DemoGeography({ concept, source, capabilities }: ConceptSectionProps) {
  const markets = useResource<Market[]>(
    () => (capabilities.markets && source.getMarkets ? source.getMarkets() : []),
    [source, capabilities.markets],
  );
  const effectiveMarkets = useMemo(
    () =>
      markets.data && markets.data.length > 0
        ? markets.data
        : deriveMarketsFromTerms(concept.terms),
    [markets.data, concept.terms],
  );
  const groups = useMemo(
    () => termsByMarket(concept.terms, effectiveMarkets),
    [concept.terms, effectiveMarkets],
  );

  return (
    <ConceptSection title="Geography" description="Markets and the term and status used in each.">
      <div className="grid gap-3 sm:grid-cols-2">
        {groups.map((g) => (
          <div key={g.name} className="rounded-lg border bg-muted/20 p-3">
            <div className="mb-2 flex items-center justify-between">
              <span className="text-sm font-medium text-foreground">{g.name}</span>
              <span className="text-[11px] text-muted-foreground">
                {g.locales.length} locale{g.locales.length === 1 ? "" : "s"}
              </span>
            </div>
            <ul className="space-y-1.5">
              {g.locales.map(({ locale, terms }) =>
                terms.map((t) => (
                  <li key={`${locale}-${t.text}`} className="flex items-center gap-2 text-sm">
                    <LocalePill locale={locale} />
                    <span
                      className={cn(
                        "flex-1 truncate",
                        isBannedStatus(t.status)
                          ? "text-muted-foreground line-through"
                          : "text-foreground",
                      )}
                    >
                      {t.text}
                    </span>
                    <StatusChip status={t.status} className="text-[10px]" />
                  </li>
                )),
              )}
            </ul>
          </div>
        ))}
      </div>
    </ConceptSection>
  );
}

// ── Timeline (vertical) ──────────────────────────────────────────────────────

export function DemoTimeline({ concept, source, capabilities }: ConceptSectionProps) {
  const remote = useResource(
    () => (capabilities.timeline && source.getTimeline ? source.getTimeline(concept.id) : []),
    [source, concept.id, capabilities.timeline],
  );
  const events = useMemo(() => {
    const e = remote.data ?? [];
    return e.length > 0 ? e : synthesizeTimeline(concept);
  }, [remote.data, concept]);
  const days = useMemo(() => buildTimeline(events, "desc"), [events]);

  return (
    <ConceptSection title="Timeline" description="How this concept evolved.">
      {days.length === 0 ? (
        <EmptyHint title="No history yet" />
      ) : (
        <ol className="relative ml-1 space-y-4 border-l pl-4">
          {days.map((day) => (
            <li key={day.key} className="space-y-2">
              <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                {day.label}
              </p>
              {day.events.map((e, i) => (
                <div key={e.id ?? i} className="relative">
                  <span className="absolute -left-[1.32rem] top-1.5 size-2 rounded-full bg-primary ring-2 ring-background" />
                  <p className="text-sm text-foreground">{e.summary}</p>
                  <p className="text-xs text-muted-foreground">
                    {e.actor ? `${e.actor} · ` : ""}
                    {formatRelative(e.at)}
                  </p>
                  {e.detail && <p className="mt-0.5 text-xs text-muted-foreground">“{e.detail}”</p>}
                </div>
              ))}
            </li>
          ))}
        </ol>
      )}
    </ConceptSection>
  );
}

// ── Constraints (validity windows) ───────────────────────────────────────────

export function DemoConstraints({ concept }: ConceptSectionProps) {
  const rows = useMemo(
    () =>
      concept.terms
        .filter(
          (t) =>
            t.validity?.validFrom ||
            t.validity?.validTo ||
            isBannedStatus(t.status) ||
            t.status === "preferred",
        )
        .map((t) => ({ term: t })),
    [concept.terms],
  );

  return (
    <ConceptSection
      title="Constraints"
      description="Validity windows and where a term is banned or preferred."
    >
      {rows.length === 0 ? (
        <EmptyHint title="No constraints" description="Every term applies everywhere, always." />
      ) : (
        <ul className="space-y-2">
          {rows.map(({ term }, i) => (
            <li
              key={`${term.locale}-${term.text}-${i}`}
              className="flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm"
            >
              <LocalePill locale={term.locale} />
              <span
                className={cn(
                  "font-medium",
                  isBannedStatus(term.status)
                    ? "text-muted-foreground line-through"
                    : "text-foreground",
                )}
              >
                {term.text}
              </span>
              <StatusChip status={term.status} className="text-[10px]" />
              {(term.validity?.validFrom || term.validity?.validTo) && (
                <Badge variant="outline" className="ml-auto font-normal text-[11px]">
                  {formatDate(term.validity?.validFrom)} → {formatDate(term.validity?.validTo)}
                </Badge>
              )}
              {term.validity?.tags?.market && (
                <Badge variant="outline" className="font-normal text-[11px]">
                  {term.validity.tags.market}
                </Badge>
              )}
            </li>
          ))}
        </ul>
      )}
    </ConceptSection>
  );
}

// ── Observations (optional rich) ─────────────────────────────────────────────

export function DemoObservations({ concept, source, capabilities }: ConceptSectionProps) {
  const obs = useResource(
    () =>
      capabilities.observations && source.getObservations ? source.getObservations(concept.id) : [],
    [source, concept.id, capabilities.observations],
  );
  const items = obs.data ?? [];
  return (
    <ConceptSection title="Observations" icon={<Quote />} description="External evidence.">
      {items.length === 0 ? (
        <EmptyHint title="No observations" />
      ) : (
        <ul className="space-y-3">
          {items.map((o) => (
            <li key={o.id} className="rounded-md border-l-2 border-primary/40 pl-3">
              <p className="text-sm italic text-foreground">“{o.quote}”</p>
              <p className="mt-1 text-xs text-muted-foreground">
                <span className="capitalize">{o.kind.replace("_", " ")}</span> · {o.source}
                {o.actor ? ` · ${o.actor}` : ""} · {formatRelative(o.at)}
              </p>
            </li>
          ))}
        </ul>
      )}
    </ConceptSection>
  );
}

// ── Comments (optional rich) ─────────────────────────────────────────────────

export function DemoComments({ concept, source, capabilities }: ConceptSectionProps) {
  const com = useResource(
    () => (capabilities.comments && source.getComments ? source.getComments(concept.id) : []),
    [source, concept.id, capabilities.comments],
  );
  const items = com.data ?? [];
  return (
    <ConceptSection title="Comments" description="Discussion.">
      {items.length === 0 ? (
        <EmptyHint title="No comments" />
      ) : (
        <ul className="space-y-3">
          {items.map((c) => (
            <li key={c.id} className={cn("text-sm", c.parentId && "ml-5 border-l pl-3")}>
              <p className="text-foreground">{c.body}</p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {c.author} · {formatRelative(c.at)}
                {c.resolved ? " · resolved" : ""}
              </p>
            </li>
          ))}
        </ul>
      )}
    </ConceptSection>
  );
}

// ── Bundle ───────────────────────────────────────────────────────────────────

export const demoSlots = {
  geography: (p: ConceptSectionProps) => <DemoGeography {...p} />,
  relations: (p: ConceptSectionProps) => <DemoRelations {...p} />,
  timeline: (p: ConceptSectionProps) => <DemoTimeline {...p} />,
  constraints: (p: ConceptSectionProps) => <DemoConstraints {...p} />,
  observations: (p: ConceptSectionProps) => <DemoObservations {...p} />,
  comments: (p: ConceptSectionProps) => <DemoComments {...p} />,
};
