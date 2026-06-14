// ConstraintsPanel — the concept-view constraints panel (Apache-2.0). It makes a
// concept's temporal + per-market truth visual in two reads:
//
//   • a small Gantt-style time chart: every dated term/relation is one bar on a
//     shared scale, with a "today" marker, open-start / open-end caps for
//     "since forever" and "still in force", and dimmed bars for windows that
//     have closed;
//   • a "banned where / preferred where" summary derived from per-market term
//     status — the part a flat termbase with no dates can still answer.
//
// Bind it into a ConceptView constraints slot:  slots={{ constraints: (p) => <ConstraintsPanel {...p} /> }}
import { useMemo } from "react";
import type { ReactNode } from "react";
import { Badge, cn } from "@neokapi/ui-primitives";
import { Ban, CalendarClock, Infinity as InfinityIcon, ShieldCheck } from "lucide-react";
import type { ConceptSectionProps } from "./ConceptView";
import { ConceptSection, EmptyHint, ErrorHint, LocalePill, StatusChip, formatDate } from "./atoms";
import { TERM_STATUS_LABEL, primaryName } from "./concept-meta";
import { buildConstraintModel, constraintSummary, windowPhrase } from "./constraints";
import type { ConstraintLane, ConstraintPlacement } from "./constraints";
import { deriveMarketsFromTerms } from "./grouping";
import type { Market, TermStatus } from "./types";
import { useResource } from "./useResource";

/** Bar tint per term status (relations fall back to a neutral primary). */
const STATUS_BAR: Record<TermStatus, string> = {
  preferred: "bg-success/70",
  approved: "bg-primary/55",
  admitted: "bg-warning/70",
  proposed: "bg-muted-foreground/40",
  deprecated: "bg-destructive/55",
  forbidden: "bg-destructive/75",
};

function laneBarClass(lane: ConstraintLane): string {
  return lane.status ? STATUS_BAR[lane.status] : "bg-primary/45";
}

export function ConstraintsPanel({ concept, source, capabilities }: ConceptSectionProps) {
  // Markets: explicit when the platform supplies them, else derived from the
  // term validity tags so the framework-only path still labels by market.
  const marketRes = useResource<Market[]>(
    () => (capabilities.markets && source.getMarkets ? source.getMarkets() : []),
    [source, capabilities.markets],
  );
  const markets = useMemo(
    () =>
      marketRes.data && marketRes.data.length > 0
        ? marketRes.data
        : deriveMarketsFromTerms(concept.terms),
    [marketRes.data, concept.terms],
  );

  // Direct relations — dated ones become lanes; neighbours need a label.
  const rel = useResource(() => source.getRelations(concept.id), [source, concept.id]);
  const relations = useMemo(() => rel.data ?? [], [rel.data]);
  const neighbourIds = useMemo(() => {
    const ids = new Set<string>();
    for (const r of relations) {
      if (!r.validity?.validFrom && !r.validity?.validTo) continue;
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

  const model = useMemo(
    () =>
      buildConstraintModel(concept, {
        markets,
        relations,
        labelFor: (id) => names.data?.[id] ?? id,
      }),
    [concept, markets, relations, names.data],
  );
  const summary = useMemo(() => constraintSummary(concept, { markets }), [concept, markets]);

  const nothing =
    !model.hasWindows && summary.banned.length === 0 && summary.preferred.length === 0;

  // A failed relations/markets read must not read as "no constraints".
  const error = rel.error ?? marketRes.error;

  return (
    <ConceptSection
      title="Constraints"
      icon={<CalendarClock />}
      description="Validity windows and where a term is banned or preferred."
    >
      {error ? (
        <ErrorHint title="Could not load constraints" description={error.message} />
      ) : nothing ? (
        <EmptyHint
          icon={<CalendarClock />}
          title="No constraints"
          description="Every term applies everywhere, always."
        />
      ) : (
        <div className="space-y-5">
          {model.hasWindows && <ValidityChart model={model} />}
          <Summary summary={summary} asOf={model.asOf} />
        </div>
      )}
    </ConceptSection>
  );
}

// ── Validity chart (the time bars) ───────────────────────────────────────────

function ValidityChart({ model }: { model: ReturnType<typeof buildConstraintModel> }) {
  const { scale, lanes } = model;
  const nowLeft = `${(scale.nowPos * 100).toFixed(2)}%`;
  return (
    <div>
      <div className="flex gap-3">
        {/* Label column. */}
        <ul className="w-36 shrink-0 sm:w-44">
          <li className="h-5" aria-hidden />
          {lanes.map((lane) => (
            <li key={lane.id} className="flex h-9 flex-col justify-center gap-0.5 pr-1">
              <div className="flex items-center gap-1.5">
                {lane.locale && <LocalePill locale={lane.locale} />}
                <span
                  className={cn(
                    "truncate text-xs font-medium",
                    lane.active ? "text-foreground" : "text-muted-foreground",
                  )}
                  title={lane.label}
                >
                  {lane.label}
                </span>
              </div>
              {lane.market && (
                <span className="truncate text-[10px] text-muted-foreground">{lane.market}</span>
              )}
            </li>
          ))}
        </ul>

        {/* Track column with shared gridlines + now marker overlay. */}
        <div className="relative min-w-0 flex-1">
          {scale.ticks.map((tick, i) => (
            <span
              key={`${tick.label}-${i}`}
              aria-hidden
              className="absolute inset-y-0 w-px bg-border/50"
              style={{ left: `${(tick.pos * 100).toFixed(2)}%` }}
            />
          ))}
          {/* Now marker. */}
          <span
            aria-hidden
            className="absolute inset-y-0 z-10 w-px bg-primary/70"
            style={{ left: nowLeft }}
          >
            <span className="absolute -top-px left-1/2 size-1.5 -translate-x-1/2 rounded-full bg-primary" />
          </span>

          {/* Axis header. */}
          <div className="relative h-5">
            {scale.ticks.map((tick, i) => (
              <span
                key={`${tick.label}-${i}`}
                className="absolute top-0 -translate-x-1/2 text-[10px] tabular-nums text-muted-foreground"
                style={{ left: `${(tick.pos * 100).toFixed(2)}%` }}
              >
                {tick.label}
              </span>
            ))}
          </div>

          {/* Bars. */}
          <ol>
            {lanes.map((lane) => (
              <li key={lane.id} className="relative flex h-9 items-center">
                <LaneBar lane={lane} />
              </li>
            ))}
          </ol>
        </div>
      </div>

      <p className="mt-2 flex items-center gap-1.5 pl-[9.75rem] text-[10px] text-muted-foreground">
        <span className="inline-block h-2 w-px bg-primary/70" />
        Today, {formatDate(model.asOf)}
      </p>
    </div>
  );
}

function LaneBar({ lane }: { lane: ConstraintLane }) {
  const leftPct = lane.start * 100;
  // Guarantee a visible bar even for an instant window.
  const widthPct = Math.max((lane.end - lane.start) * 100, 1.5);
  return (
    <div
      className={cn(
        "absolute flex h-3 items-center rounded-sm shadow-sm transition-[opacity] duration-200",
        laneBarClass(lane),
        !lane.active && "opacity-45",
        lane.openStart && "rounded-l-none",
        lane.openEnd && "rounded-r-none",
      )}
      style={{ left: `${leftPct.toFixed(2)}%`, width: `${widthPct.toFixed(2)}%` }}
      title={`${lane.status ? TERM_STATUS_LABEL[lane.status] : "Relation"} · ${windowPhrase(lane.validFrom, lane.validTo)}${lane.active ? "" : " · not in force"}`}
    >
      {lane.openStart && (
        <span className="absolute inset-y-0 left-0 w-3 rounded-l-sm bg-gradient-to-r from-card to-transparent" />
      )}
      {lane.openEnd && (
        <InfinityIcon
          aria-hidden
          className="absolute -right-4 top-1/2 size-3 -translate-y-1/2 text-muted-foreground"
        />
      )}
    </div>
  );
}

// ── Banned-where / preferred-where summary ───────────────────────────────────

function Summary({
  summary,
  asOf,
}: {
  summary: ReturnType<typeof constraintSummary>;
  asOf: string;
}) {
  if (summary.banned.length === 0 && summary.preferred.length === 0) return null;
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <PlacementColumn
        tone="banned"
        icon={<Ban aria-hidden />}
        title="Banned"
        emptyLabel="Nothing banned"
        placements={summary.banned}
      />
      <PlacementColumn
        tone="preferred"
        icon={<ShieldCheck aria-hidden />}
        title="Preferred"
        emptyLabel="No preferred term set"
        placements={summary.preferred}
      />
      <p className="text-[10px] text-muted-foreground sm:col-span-2">
        In force as of {formatDate(asOf)}.
      </p>
    </div>
  );
}

function PlacementColumn({
  tone,
  icon,
  title,
  emptyLabel,
  placements,
}: {
  tone: "banned" | "preferred";
  icon: ReactNode;
  title: string;
  emptyLabel: string;
  placements: ConstraintPlacement[];
}) {
  const accent = tone === "banned" ? "text-destructive" : "text-success";
  return (
    <div className="rounded-lg border bg-muted/20 p-3">
      <div className={cn("mb-2 flex items-center gap-1.5 text-xs font-semibold", accent)}>
        <span className="[&_svg]:size-3.5">{icon}</span>
        {title}
        <span className="font-normal text-muted-foreground">{placements.length}</span>
      </div>
      {placements.length === 0 ? (
        <p className="text-xs text-muted-foreground">{emptyLabel}</p>
      ) : (
        <ul className="space-y-1.5">
          {placements.map((p) => (
            <li key={`${p.market ?? ""}-${p.locale}-${p.text}`} className="text-sm">
              <div className="flex flex-wrap items-center gap-1.5">
                <LocalePill locale={p.locale} />
                <span
                  className={cn(
                    "font-medium",
                    tone === "banned" ? "text-muted-foreground line-through" : "text-foreground",
                  )}
                >
                  {p.text}
                </span>
                {p.market && (
                  <Badge variant="outline" className="font-normal text-[10px]">
                    {p.market}
                  </Badge>
                )}
                {!p.active && (
                  <span className="text-[10px] text-muted-foreground">not in force</span>
                )}
              </div>
              <p className="mt-0.5 flex items-center gap-1 text-[11px] text-muted-foreground">
                <StatusChip status={p.status} className="px-1 py-0 text-[9px] leading-tight" />
                {(p.validFrom || p.validTo) && <span>{windowPhrase(p.validFrom, p.validTo)}</span>}
              </p>
              {p.note && (
                <p className="mt-0.5 text-[11px] italic text-muted-foreground">{p.note}</p>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
