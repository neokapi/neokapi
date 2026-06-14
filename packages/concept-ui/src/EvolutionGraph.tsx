// EvolutionGraph — the VERTICAL timeline (Apache-2.0). This is the DEFAULT face
// of a concept's evolution: a continuous spine with an icon node per moment and
// the moment's detail in a card beside it — the canonical timeline pattern, and
// the one that reads instantly as a timeline at any width (the horizontal lane
// "Compare languages" roadmap is the opt-in alternative). It flattens the whole
// derived model — global milestones, each lane's own milestones (carrying their
// locale), the structural sibling/rename branches, and the density clusters —
// into one time-sorted, day-grouped spine. Sibling/rename cards carry a branch
// accent so a fork still reads as a fork; cluster cards collapse a run of routine
// edits into one expandable "N changes" cloud.

import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Avatar, AvatarFallback, cn } from "@neokapi/ui-primitives";
import { History } from "lucide-react";
import { EmptyHint, LocalePill, formatDate, formatRelative } from "./atoms";
import { ClusterPill, MilestoneDot, toneMeta } from "./evolution-atoms";
import type {
  EvolutionBranch,
  EvolutionCluster,
  EvolutionMilestone,
  EvolutionTone,
} from "./evolution-types";
import type { EvolutionViewProps } from "./evolution-view";

// ── Row model ────────────────────────────────────────────────────────────────

/**
 * One entry on the vertical spine. A `point` is a single dated event (global or
 * lane-scoped); a `cluster` is a collapsible cloud of folded routine edits. Both
 * carry the structural flags the renderer needs — `branch` makes the row grow an
 * offshoot rail, `locale` shows a small chip, `navigateId` makes it clickable.
 */
type GraphRow =
  | {
      type: "point";
      id: string;
      at: string;
      tone: EvolutionTone;
      summary: string;
      detail?: string;
      actor?: string;
      locale?: string;
      navigateId?: string;
      /** Set when this row reads as a structural fork off the spine. */
      branch?: "sibling" | "rename";
    }
  | {
      type: "cluster";
      id: string;
      at: string;
      cluster: EvolutionCluster;
      locale?: string;
    };

/** A milestone with its lane locale resolved (global rows carry none). */
function pointFromMilestone(m: EvolutionMilestone, locale?: string): GraphRow {
  return {
    type: "point",
    id: m.id,
    at: m.at,
    tone: m.tone,
    summary: m.summary,
    detail: m.detail,
    actor: m.actor,
    locale: m.laneKey ? locale : undefined,
    navigateId: m.navigateId,
    branch: m.kind === "sibling" ? "sibling" : m.kind === "rename" ? "rename" : undefined,
  };
}

/**
 * A branch the spine should draw as its own row. Sibling births read as
 * "<locale> branched from <origin>"; a `REPLACED_BY` rename reads as the branch's
 * own label and navigates. Relation branches are skipped here — their milestone
 * already carries them on the spine.
 */
function pointFromBranch(b: EvolutionBranch): GraphRow | undefined {
  if (b.kind === "sibling") {
    const from = b.fromLaneKey ? ` from ${b.fromLaneKey}` : "";
    return {
      type: "point",
      id: b.id,
      at: b.at,
      tone: "sibling",
      summary: `${b.toLaneKey ?? "A sibling"} branched${from}`,
      locale: b.toLaneKey,
      branch: "sibling",
    };
  }
  if (b.kind === "rename") {
    return {
      type: "point",
      id: b.id,
      at: b.at,
      tone: "rename",
      summary: b.label,
      navigateId: b.navigateId,
      branch: "rename",
    };
  }
  return undefined;
}

/**
 * A stable dedupe key for a row's *meaning* (not its id), so a rename surfaced
 * BOTH as a relation milestone (`ms:rename:<r.id>`) and as its branch
 * (`br:rename:<r.id>`) — distinct ids — collapses to one spine row, and a sibling
 * birth doesn't double between the lane milestone and its branch.
 */
function rowKind(row: GraphRow): string {
  if (row.type === "cluster") return `cluster:${row.id}`;
  if (row.branch === "rename") return `rename:${row.navigateId ?? row.summary}:${row.at}`;
  if (row.branch === "sibling") return `sibling:${row.locale ?? ""}:${row.at}`;
  return `id:${row.id}`;
}

/** Merge every model signal into one time-sorted spine of rows. */
function buildRows(model: EvolutionViewProps["model"], order: "asc" | "desc"): GraphRow[] {
  const rows: GraphRow[] = [];

  // Branches first, so a sibling/rename surfaced BOTH as a branch and as a lane
  // milestone resolves to the branch's richer "<locale> branched from <origin>" /
  // navigating-rename framing (dedupe keeps the first row of each meaning).
  for (const b of model.branches) {
    const row = pointFromBranch(b);
    if (row) rows.push(row);
  }

  for (const m of model.milestones) rows.push(pointFromMilestone(m));
  for (const c of model.globalClusters)
    rows.push({ type: "cluster", id: c.id, at: c.start, cluster: c });

  for (const lane of model.lanes) {
    for (const m of lane.milestones) rows.push(pointFromMilestone(m, lane.locale));
    for (const c of lane.clusters) {
      rows.push({ type: "cluster", id: c.id, at: c.start, cluster: c, locale: lane.locale });
    }
  }

  // Dedupe by meaning (keep first seen), then time-sort honouring `order`.
  const seen = new Set<string>();
  const unique = rows.filter((row) => {
    const key = rowKind(row);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  const dir = order === "asc" ? 1 : -1;
  return unique.sort((a, b) => dir * (Date.parse(a.at) - Date.parse(b.at)));
}

// ── Day grouping ─────────────────────────────────────────────────────────────

interface GraphDay {
  key: string;
  label: string;
  rows: GraphRow[];
}

/** Group the already-sorted rows into day sections (the order is preserved). */
function groupByDay(rows: GraphRow[]): GraphDay[] {
  const days: GraphDay[] = [];
  for (const row of rows) {
    const key = row.at.slice(0, 10);
    const last = days[days.length - 1];
    if (last && last.key === key) last.rows.push(row);
    else days.push({ key, label: formatDate(row.at), rows: [row] });
  }
  return days;
}

// ── Component ────────────────────────────────────────────────────────────────

export function EvolutionGraph({
  model,
  order = "desc",
  onNavigate,
  className,
}: EvolutionViewProps) {
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const days = useMemo(() => groupByDay(buildRows(model, order)), [model, order]);
  const total = useMemo(() => days.reduce((n, d) => n + d.rows.length, 0), [days]);

  function toggle(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  if (total === 0) {
    return (
      <div className={cn("text-sm", className)}>
        <EmptyHint
          icon={<History />}
          title="No history yet"
          description="This concept has no recorded changes."
        />
      </div>
    );
  }

  return (
    <div className={cn("relative text-sm", className)}>
      {/* The continuous spine, faded at both ends, behind every node. */}
      <span
        aria-hidden
        className="absolute bottom-2 left-3.5 top-2 w-px -translate-x-1/2 bg-gradient-to-b from-transparent via-border to-transparent"
      />
      <div className="space-y-6">
        {days.map((day, di) => (
          <section key={day.key}>
            <DayHeader label={day.label} />
            <ol className="mt-3 space-y-3">
              {day.rows.map((row, i) =>
                row.type === "cluster" ? (
                  <ClusterRow
                    key={row.id}
                    row={row}
                    index={di + i}
                    expanded={expanded.has(row.id)}
                    onToggle={() => toggle(row.id)}
                    onNavigate={onNavigate}
                  />
                ) : (
                  <PointRow key={row.id} row={row} index={di + i} onNavigate={onNavigate} />
                ),
              )}
            </ol>
          </section>
        ))}
      </div>
    </div>
  );
}

// ── Rows ─────────────────────────────────────────────────────────────────────

/** The rail + card row layout (flex, so it needs no arbitrary grid template). */
const ROW_GRID = "flex items-stretch gap-2.5";
/** Stagger the entrance so a long history reads as it lands, not all at once. */
function delay(index: number): string {
  return `${Math.min(index, 10) * 35}ms`;
}

/** A subtle month/day marker, aligned with the node column, that sits on the spine. */
function DayHeader({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2.5">
      <span className="flex w-7 justify-center">
        <span aria-hidden className="size-1.5 rounded-full bg-border ring-4 ring-card" />
      </span>
      <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
    </div>
  );
}

/** First letters of an actor name, for the avatar fallback (max 2). */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? "").join("") || "·";
}

/** The card chrome shared by point + cluster rows, with the spine connector. */
function RowFrame({
  tone,
  index,
  branch,
  onNodeClick,
  nodeTitle,
  children,
}: {
  tone: EvolutionTone;
  index: number;
  branch?: "sibling" | "rename";
  onNodeClick?: () => void;
  nodeTitle?: string;
  children: ReactNode;
}) {
  return (
    <li
      className={cn(ROW_GRID, "animate-in fade-in slide-in-from-bottom-1")}
      style={{ animationDelay: delay(index) }}
    >
      <div className="relative flex w-7 shrink-0 justify-center">
        <MilestoneDot tone={tone} title={nodeTitle} onClick={onNodeClick} className="mt-1" />
      </div>
      <div className="relative min-w-0 flex-1">
        {/* A short connector tying the card to its node on the spine. */}
        <span aria-hidden className="absolute -left-2.5 top-4 h-px w-2.5 bg-border" />
        <div
          className={cn(
            "rounded-lg border bg-card px-3 py-2 shadow-sm transition-shadow hover:shadow-md",
            branch && "border-l-2 border-l-primary",
          )}
        >
          {children}
        </div>
      </div>
    </li>
  );
}

function PointRow({
  row,
  index,
  onNavigate,
}: {
  row: Extract<GraphRow, { type: "point" }>;
  index: number;
  onNavigate?: (conceptId: string) => void;
}) {
  const navigable = Boolean(row.navigateId && onNavigate);
  const go = navigable ? () => onNavigate!(row.navigateId!) : undefined;
  const accent = toneMeta(row.tone).accent;
  return (
    <RowFrame
      tone={row.tone}
      index={index}
      branch={row.branch}
      onNodeClick={go}
      nodeTitle={row.summary}
    >
      <div className="flex items-start justify-between gap-2">
        <button
          type="button"
          disabled={!navigable}
          onClick={go}
          className={cn(
            "min-w-0 text-left font-medium leading-snug text-foreground",
            navigable &&
              "rounded-sm hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          )}
        >
          {row.summary}
        </button>
        {row.locale && <LocalePill locale={row.locale} className="mt-0.5 shrink-0" />}
      </div>
      {row.detail && (
        <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{row.detail}</p>
      )}
      <div className="mt-1.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
        {row.actor ? (
          <>
            <Avatar className="size-4">
              <AvatarFallback className="text-[8px] font-medium">
                {initials(row.actor)}
              </AvatarFallback>
            </Avatar>
            <span className="font-medium text-foreground/80">{row.actor}</span>
            <span aria-hidden>·</span>
          </>
        ) : null}
        <span className={cn(toneMeta(row.tone).label && accent, "capitalize")}>
          {toneMeta(row.tone).label}
        </span>
        <span aria-hidden>·</span>
        <time dateTime={row.at}>{formatRelative(row.at)}</time>
      </div>
    </RowFrame>
  );
}

function ClusterRow({
  row,
  index,
  expanded,
  onToggle,
  onNavigate,
}: {
  row: Extract<GraphRow, { type: "cluster" }>;
  index: number;
  expanded: boolean;
  onToggle: () => void;
  onNavigate?: (conceptId: string) => void;
}) {
  const { cluster } = row;
  return (
    <RowFrame tone="edit" index={index} nodeTitle={`${cluster.count} routine changes`}>
      <div className="flex items-center gap-2">
        <ClusterPill count={cluster.count} expanded={expanded} onToggle={onToggle} />
        {row.locale ? <LocalePill locale={row.locale} /> : null}
      </div>
      {expanded && (
        <ol className="mt-2.5 space-y-2 border-l border-border/60 pl-3">
          {cluster.items.map((item) => {
            const navigable = Boolean(item.navigateId && onNavigate);
            const go = navigable ? () => onNavigate!(item.navigateId!) : undefined;
            return (
              <li key={item.id} className="flex items-start gap-2">
                <MilestoneDot tone={item.tone} size="sm" title={item.summary} onClick={go} />
                <div className="min-w-0 pt-0.5">
                  <p className="truncate text-xs leading-snug text-foreground">{item.summary}</p>
                  <p className="text-[11px] text-muted-foreground">
                    {item.actor ? `${item.actor} · ` : ""}
                    <time dateTime={item.at}>{formatRelative(item.at)}</time>
                  </p>
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </RowFrame>
  );
}
