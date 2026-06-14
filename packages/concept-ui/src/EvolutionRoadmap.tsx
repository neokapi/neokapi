// EvolutionRoadmap — the HORIZONTAL renderer of a concept's evolution model
// (Apache-2.0). Time flows left (genesis) → right (now): every span, milestone,
// cluster, and branch is positioned by its fraction across one shared time
// scale, so the whole roadmap scales with the column width rather than fixed px
// offsets. It is the wide-viewport face of the shared `EvolutionViewProps`
// contract — `EvolutionGraph` is the narrow, vertical fallback the responsive
// orchestrator swaps to. The layout reads top→bottom: a context track, a global
// events rail, the focused language lanes (with sibling branches reading as a
// language joining in), a "now" line, an opt-in band of the remaining
// languages, then the axis labels and legend.
//
// We render only the inner content — the caller wraps it in a `ConceptSection`.
import { useState } from "react";
import { Button, cn } from "@neokapi/ui-primitives";
import { ChevronDown, ChevronRight } from "lucide-react";
import {
  AxisLabels,
  AxisTicks,
  ClusterPill,
  ContextMarkerTick,
  EvolutionLegend,
  LaneLabel,
  MilestoneDot,
  SpanBar,
  makeScale,
} from "./evolution-atoms";
import type { TimeScale } from "./evolution-atoms";
import { formatRelative } from "./atoms";
import type {
  EvolutionBranch,
  EvolutionCluster,
  EvolutionContextMarker,
  EvolutionLane,
  EvolutionTick,
} from "./evolution-types";
import type { EvolutionViewProps } from "./evolution-view";
import type { TermStatus } from "./types";

/** Percent-from-left CSS value for an instant, rounded for stable hydration. */
function leftPct(scale: TimeScale, at: string): string {
  return `${(scale.pct(at) * 100).toFixed(3)}%`;
}

/** Midpoint percent of a [start, end] cluster window — where its pill centres. */
function midPct(scale: TimeScale, start: string, end: string): string {
  return `${(((scale.pct(start) + scale.pct(end)) / 2) * 100).toFixed(3)}%`;
}

/** Distinct statuses on a lane's spans — drives the lane-label status dots. */
function laneStatuses(lane: EvolutionLane): TermStatus[] {
  return [...new Set(lane.spans.map((s) => s.status))];
}

/** Minimum fraction-of-extent gap two context labels need to both be drawn. */
const CONTEXT_LABEL_GAP = 0.16;

/**
 * Decide which context labels to draw. Every marker keeps its dot; a label is
 * shown only when it clears the previously-shown one by `CONTEXT_LABEL_GAP`, so a
 * dense context track stays readable instead of collapsing into one blur. The
 * suppressed labels remain reachable via the dot's tooltip.
 */
function decollideLabels(
  markers: EvolutionContextMarker[],
  scale: TimeScale,
): Array<{ marker: EvolutionContextMarker; showLabel: boolean }> {
  const sorted = [...markers].sort((a, b) => scale.pct(a.at) - scale.pct(b.at));
  let lastLabelPct = Number.NEGATIVE_INFINITY;
  return sorted.map((marker) => {
    const p = scale.pct(marker.at);
    const showLabel = p - lastLabelPct >= CONTEXT_LABEL_GAP;
    if (showLabel) lastLabelPct = p;
    return { marker, showLabel };
  });
}

export function EvolutionRoadmap({ model, onNavigate, className }: EvolutionViewProps) {
  const scale = makeScale(model.extent);
  const [showMore, setShowMore] = useState(false);
  // One cluster cloud open at a time — click to reveal the folded events.
  const [openCluster, setOpenCluster] = useState<string | null>(null);
  const toggleCluster = (id: string) => setOpenCluster((cur) => (cur === id ? null : id));

  // Sibling branches land ON a destination lane; index by it so a lane can draw
  // the "a language branched in here" marker at the fork instant.
  const siblingsByLane = new Map<string, EvolutionBranch[]>();
  for (const branch of model.branches) {
    if (branch.kind !== "sibling" || !branch.toLaneKey) continue;
    const list = siblingsByLane.get(branch.toLaneKey) ?? [];
    list.push(branch);
    siblingsByLane.set(branch.toLaneKey, list);
  }

  return (
    <div className={cn("text-card-foreground", className)}>
      {/* A horizontal scroll guard: a very tight extent overflows gracefully;
          a normal one fits the column. The min-width keeps tick labels legible. */}
      <div className="overflow-x-auto">
        <div className="min-w-[32rem] space-y-2">
          {/* 1 ── Context track (markets introduced, change-sets merged). */}
          {model.context.length > 0 && (
            <div className="flex items-start gap-2">
              <span className="w-16 shrink-0 pt-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                Context
              </span>
              <div className="relative h-9 flex-1">
                {decollideLabels(model.context, scale).map(({ marker, showLabel }) => (
                  <ContextMarkerTick
                    key={marker.id}
                    at={marker.at}
                    label={marker.label}
                    kind={marker.kind}
                    scale={scale}
                    showLabel={showLabel}
                  />
                ))}
              </div>
            </div>
          )}

          {/* 2 ── Global events rail (spine milestones + folded clouds). */}
          <div className="flex items-center gap-2">
            <span className="w-16 shrink-0 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
              Events
            </span>
            <div className="relative h-9 flex-1">
              <AxisTicks scale={scale} ticks={model.extent.ticks} />
              {model.milestones.map((milestone) => (
                <span
                  key={milestone.id}
                  className="absolute top-1/2 -translate-x-1/2 -translate-y-1/2"
                  style={{ left: leftPct(scale, milestone.at) }}
                >
                  <MilestoneDot
                    tone={milestone.tone}
                    title={milestone.summary}
                    size="sm"
                    onClick={
                      milestone.navigateId && onNavigate
                        ? () => onNavigate(milestone.navigateId!)
                        : undefined
                    }
                  />
                </span>
              ))}
              {model.globalClusters.map((cluster) => (
                <RoadmapCluster
                  key={cluster.id}
                  cluster={cluster}
                  scale={scale}
                  open={openCluster === cluster.id}
                  onToggle={() => toggleCluster(cluster.id)}
                  onNavigate={onNavigate}
                />
              ))}
            </div>
          </div>

          {/* 3 ── Focused language lanes. */}
          <div className="space-y-2.5 pt-1">
            {model.focusedLanes.map((lane) => (
              <LaneRow
                key={lane.key}
                lane={lane}
                scale={scale}
                ticks={model.extent.ticks}
                siblings={siblingsByLane.get(lane.key) ?? []}
                onNavigate={onNavigate}
                openCluster={openCluster}
                onToggleCluster={toggleCluster}
              />
            ))}
          </div>

          {/* 6 ── More-languages band: density preview, expandable to full lanes. */}
          {model.moreLanes.length > 0 && (
            <div className="space-y-1.5 border-t pt-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 gap-1 px-2 text-xs text-muted-foreground"
                aria-expanded={showMore}
                onClick={() => setShowMore((v) => !v)}
              >
                {showMore ? (
                  <ChevronDown aria-hidden className="size-3.5" />
                ) : (
                  <ChevronRight aria-hidden className="size-3.5" />
                )}
                {showMore ? "Fewer languages" : `+${model.moreLanes.length} more languages`}
              </Button>
              {showMore
                ? model.moreLanes.map((lane) => (
                    <LaneRow
                      key={lane.key}
                      lane={lane}
                      scale={scale}
                      ticks={model.extent.ticks}
                      siblings={siblingsByLane.get(lane.key) ?? []}
                      onNavigate={onNavigate}
                      openCluster={openCluster}
                      onToggleCluster={toggleCluster}
                    />
                  ))
                : model.moreLanes.map((lane) => (
                    <DensityRow key={lane.key} lane={lane} scale={scale} />
                  ))}
            </div>
          )}

          {/* 5 ── "now" line at the right edge + tag. */}
          <div className="relative h-3">
            <span
              aria-hidden
              className="absolute bottom-0 top-0 w-px bg-primary/60"
              style={{ left: leftPct(scale, model.extent.end) }}
            />
            <span
              className="absolute -translate-x-full whitespace-nowrap rounded-sm bg-primary/10 px-1 text-[9px] font-medium leading-none text-primary"
              style={{ left: leftPct(scale, model.extent.end), top: "0.125rem" }}
            >
              now
            </span>
          </div>

          {/* 7 ── Axis labels + legend. */}
          <div className="flex items-start gap-2">
            <span className="w-16 shrink-0" aria-hidden />
            <AxisLabels scale={scale} ticks={model.extent.ticks} className="flex-1" />
          </div>
        </div>
      </div>

      <EvolutionLegend className="mt-3" />
    </div>
  );
}

/**
 * One language lane: a leading label cell + a positioned track of validity bars,
 * lane-scoped signal milestones, folded clouds, and (subtly) the fork where a
 * sibling language branched in.
 */
function LaneRow({
  lane,
  scale,
  ticks,
  siblings,
  onNavigate,
  openCluster,
  onToggleCluster,
}: {
  lane: EvolutionLane;
  scale: TimeScale;
  ticks: EvolutionTick[];
  siblings: EvolutionBranch[];
  onNavigate?: (conceptId: string) => void;
  openCluster: string | null;
  onToggleCluster: (id: string) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <LaneLabel
        locale={lane.locale}
        market={lane.market}
        statuses={laneStatuses(lane)}
        className="w-16 shrink-0"
      />
      <div className="relative h-8 flex-1">
        <AxisTicks scale={scale} ticks={ticks} className="opacity-60" />

        {/* 4 ── Sibling branch-in: a dashed connector rising to the fork dot, so
            a new language reads as branching into this lane. Lightweight; it sits
            below the bars and never covers the label. */}
        {siblings.map((branch) => (
          <span
            key={branch.id}
            aria-hidden
            className="absolute bottom-0 top-0 -translate-x-1/2 border-l border-dashed border-primary/40"
            style={{ left: leftPct(scale, branch.at) }}
          />
        ))}

        {lane.spans.map((span) => (
          <SpanBar key={span.id} span={span} scale={scale} className="top-1/2 -translate-y-1/2" />
        ))}

        {lane.milestones.map((milestone) => (
          <span
            key={milestone.id}
            className="absolute top-0 z-10 -translate-x-1/2 -translate-y-1/2"
            style={{ left: leftPct(scale, milestone.at) }}
          >
            <MilestoneDot
              tone={milestone.tone}
              title={milestone.summary}
              size="sm"
              onClick={
                milestone.navigateId && onNavigate
                  ? () => onNavigate(milestone.navigateId!)
                  : undefined
              }
            />
          </span>
        ))}

        {lane.clusters.map((cluster) => (
          <RoadmapCluster
            key={cluster.id}
            cluster={cluster}
            scale={scale}
            open={openCluster === cluster.id}
            onToggle={() => onToggleCluster(cluster.id)}
            onNavigate={onNavigate}
          />
        ))}

        {siblings.map((branch) => (
          <span
            key={`${branch.id}:dot`}
            className="absolute top-0 z-10 -translate-x-1/2 -translate-y-1/2"
            style={{ left: leftPct(scale, branch.at) }}
          >
            <MilestoneDot tone="sibling" title={branch.label} size="sm" />
          </span>
        ))}
      </div>
    </div>
  );
}

/**
 * A folded "N changes" cloud on the roadmap. Clicking it reveals the folded
 * events in a small popover anchored at the cluster's time position, so the
 * routine edits a cluster hides are reachable in the wide renderer too (the
 * git-graph expands them inline).
 */
function RoadmapCluster({
  cluster,
  scale,
  open,
  onToggle,
  onNavigate,
}: {
  cluster: EvolutionCluster;
  scale: TimeScale;
  open: boolean;
  onToggle: () => void;
  onNavigate?: (conceptId: string) => void;
}) {
  return (
    <span
      className="absolute top-1/2 z-20 -translate-x-1/2 -translate-y-1/2"
      style={{ left: midPct(scale, cluster.start, cluster.end) }}
    >
      <ClusterPill count={cluster.count} expanded={open} onToggle={onToggle} />
      {open && (
        <div className="absolute left-1/2 top-full z-30 mt-1 max-h-56 w-60 -translate-x-1/2 overflow-auto rounded-lg border bg-popover p-1.5 text-popover-foreground shadow-md">
          <ul className="space-y-0.5">
            {cluster.items.map((item) => {
              const nav =
                item.navigateId && onNavigate ? () => onNavigate(item.navigateId!) : undefined;
              return (
                <li key={item.id}>
                  <button
                    type="button"
                    disabled={!nav}
                    onClick={nav}
                    className={cn(
                      "flex w-full items-start gap-2 rounded px-1.5 py-1 text-left",
                      nav &&
                        "cursor-pointer hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    )}
                  >
                    <MilestoneDot tone={item.tone} size="sm" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-xs text-foreground">{item.summary}</span>
                      <span className="block text-[10px] text-muted-foreground">
                        {item.actor ? `${item.actor} · ` : ""}
                        {formatRelative(item.at)}
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </span>
  );
}

/**
 * The collapsed one-line preview of a folded-away language: a thin 2px track of
 * its spans by percent, so the reader stays aware of many languages without the
 * focused 1–3 drowning. It carries the locale as a label for orientation.
 */
function DensityRow({ lane, scale }: { lane: EvolutionLane; scale: TimeScale }) {
  return (
    <div className="flex items-center gap-2">
      <span className="w-16 shrink-0 truncate font-mono text-[10px] uppercase text-muted-foreground">
        {lane.locale}
      </span>
      <div className="relative h-0.5 flex-1 rounded-full bg-muted/40">
        {lane.spans.map((span) => {
          const start = scale.pct(span.start);
          const end = span.end === null ? 1 : scale.pct(span.end);
          const width = Math.max(end - start, 0.012);
          return (
            <span
              key={span.id}
              className="absolute inset-y-0 rounded-full bg-muted-foreground/40"
              style={{
                left: `${(start * 100).toFixed(3)}%`,
                width: `${(width * 100).toFixed(3)}%`,
              }}
            />
          );
        })}
      </div>
    </div>
  );
}
