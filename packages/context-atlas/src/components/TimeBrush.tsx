import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  Label,
  SimpleTooltip,
  Switch,
  ViewTab,
  ViewTabGroup,
} from "@neokapi/ui-primitives";
import { ChevronLeft, ChevronRight, Pause, Play } from "lucide-react";
import { eventGroupLabel, formatCount, formatDay } from "../labels";
import type { Atlas } from "../model/atlas";
import { EVENT_GROUPS, type EventGroup, eventHistogram } from "../model/histogram";
import {
  type AtlasQuery,
  WINDOW_PRESETS,
  presetWindow,
  shiftWindow,
  withFollow,
  withSnapshot,
  withWindow,
} from "../model/query";
import { addDays, clampDay, daysBetween } from "../model/time";
import { useElementWidth, usePrefersReducedMotion } from "./hooks";

const GROUP_FILL: Record<EventGroup, string> = {
  added: "var(--info)",
  decided: "var(--success)",
  closed: "var(--atlas-neutral)",
  window: "var(--atlas-cat-5)",
  other: "color-mix(in oklab, var(--atlas-neutral) 60%, var(--background))",
};

const BAR_H = 52;
const AXIS_H = 16;
const PAD = 8;

type Drag =
  | { kind: "new"; anchor: string }
  | { kind: "from" | "to" }
  | { kind: "move"; grab: string; from: string; to: string }
  | { kind: "snapshot" };

export interface TimeBrushProps {
  atlas: Atlas;
  query: AtlasQuery;
  /** A change to the query. `transient` changes (dragging, playback) skip the history. */
  onQuery: (next: AtlasQuery, opts?: { transient?: boolean }) => void;
}

/**
 * The time brush: events per day across the whole span, stacked by kind, on
 * the chosen clock. Drag to draw a window, drag its edges to resize it, drag
 * inside it (or shift-drag anywhere) to move it, double-click for the whole
 * span. The snapshot cursor marks the instant states are read at; it follows
 * the window's end until moved.
 */
export function TimeBrush({ atlas, query, onQuery }: TimeBrushProps) {
  const [wrapRef, width] = useElementWidth<HTMLDivElement>(720);
  const hist = useMemo(() => eventHistogram(atlas, query.clock), [atlas, query.clock]);
  const n = Math.max(1, hist.days.length);
  const step = (width - PAD * 2) / n;
  const xOf = (day: string) => PAD + daysBetween(atlas.firstDay, day) * step;
  const dayAt = (x: number) =>
    clampDay(addDays(atlas.firstDay, Math.floor((x - PAD) / step)), atlas.firstDay, atlas.lastDay);

  const [drag, setDrag] = useState<Drag | null>(null);
  const [draft, setDraft] = useState<AtlasQuery | null>(null);
  const q = draft ?? query;

  const svgRef = useRef<SVGSVGElement | null>(null);
  const localX = (e: PointerEvent) => {
    const rect = svgRef.current?.getBoundingClientRect();
    return rect ? e.clientX - rect.left : 0;
  };

  const begin = (e: PointerEvent<SVGElement>, d: Drag) => {
    e.stopPropagation();
    (e.target as Element).setPointerCapture?.(e.pointerId);
    setDrag(d);
    setDraft(query);
  };

  const onBackgroundDown = (e: PointerEvent<SVGSVGElement>) => {
    const day = dayAt(localX(e));
    const inside = day >= query.from && day <= query.to;
    if (e.shiftKey || inside) {
      begin(e, { kind: "move", grab: day, from: query.from, to: query.to });
    } else {
      begin(e, { kind: "new", anchor: day });
      setDraft(withWindow(query, day, day));
    }
  };

  const onMove = (e: PointerEvent<SVGSVGElement>) => {
    if (!drag) return;
    const day = dayAt(localX(e));
    const base = draft ?? query;
    let next = base;
    switch (drag.kind) {
      case "new":
        next = withWindow(base, drag.anchor, day);
        break;
      case "from":
        next = withWindow(base, day <= base.to ? day : base.to, base.to);
        break;
      case "to":
        next = withWindow(base, base.from, day >= base.from ? day : base.from);
        break;
      case "move": {
        const delta = daysBetween(drag.grab, day);
        next = shiftWindow(withWindow(base, drag.from, drag.to), atlas, delta);
        break;
      }
      case "snapshot":
        next = withSnapshot(base, day);
        break;
    }
    setDraft(next);
    onQuery(next, { transient: true });
  };

  const onUp = () => {
    if (drag && draft) onQuery(draft);
    setDrag(null);
    setDraft(null);
  };

  // Playback: one day per tick; slower and without animation under reduced motion.
  const reduced = usePrefersReducedMotion();
  const [playing, setPlaying] = useState(false);
  const live = useRef(query);
  live.current = query;
  useEffect(() => {
    if (!playing) return;
    const id = setInterval(
      () => {
        const cur = live.current;
        if (cur.to >= atlas.lastDay) {
          setPlaying(false);
          onQuery(cur);
          return;
        }
        onQuery(shiftWindow(cur, atlas, 1), { transient: true });
      },
      reduced ? 1500 : 600,
    );
    return () => clearInterval(id);
  }, [playing, reduced, atlas, onQuery]);

  const keyStep = (e: KeyboardEvent, apply: (days: number) => AtlasQuery) => {
    const d = e.key === "ArrowLeft" ? -1 : e.key === "ArrowRight" ? 1 : 0;
    if (!d) return;
    e.preventDefault();
    onQuery(apply(e.shiftKey ? d * 7 : d));
  };

  const span = daysBetween(q.from, q.to) + 1;
  const activePreset =
    q.from === atlas.firstDay && q.to === atlas.lastDay
      ? "all"
      : q.to === atlas.lastDay
        ? (WINDOW_PRESETS.find((p) => p === span) ?? null)
        : null;

  const brushX = xOf(q.from);
  const brushW = (daysBetween(q.from, q.to) + 1) * step;
  const snapX = xOf(q.asOf) + step;
  const height = BAR_H + AXIS_H;
  const ticks = hist.days.filter((d, i) => d.endsWith("-01") || i === 0);
  const groupsPresent = EVENT_GROUPS.filter((_, g) => hist.counts.some((row) => row[g] > 0));

  return (
    <div className="flex flex-col gap-2" data-testid="atlas-time-brush">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <ViewTabGroup aria-label={t("Window", "time brush: preset windows")}>
          {WINDOW_PRESETS.map((days) => (
            <ViewTab
              key={days}
              active={activePreset === days}
              onClick={() => {
                const w = presetWindow(atlas, days);
                onQuery(withWindow(query, w.from, w.to));
              }}
            >
              {t("{days}d", "a window of N days", { days })}
            </ViewTab>
          ))}
          <ViewTab
            active={activePreset === "all"}
            onClick={() => {
              const w = presetWindow(atlas, "all");
              onQuery(withWindow(query, w.from, w.to));
            }}
          >
            All
          </ViewTab>
        </ViewTabGroup>
        <span className="text-sm tabular-nums">
          {t("{from} to {to}", "the time window", {
            from: formatDay(q.from),
            to: formatDay(q.to, true),
          })}
        </span>
        <div className="ml-auto flex items-center gap-1.5">
          <span className="text-[11px] text-muted-foreground">Clock</span>
          <ViewTabGroup aria-label={t("Event clock", "time brush")}>
            <ViewTab
              active={query.clock === "recorded"}
              onClick={() => onQuery({ ...query, clock: "recorded" })}
            >
              Recorded
            </ViewTab>
            <ViewTab
              active={query.clock === "effective"}
              onClick={() => onQuery({ ...query, clock: "effective" })}
            >
              Effective
            </ViewTab>
          </ViewTabGroup>
        </div>
      </div>

      <div ref={wrapRef} className="relative w-full select-none">
        <svg
          ref={svgRef}
          width={width}
          height={height}
          className="block touch-none"
          role="group"
          aria-label={
            query.clock === "recorded"
              ? t("Events per day, recorded clock", "time brush chart")
              : t("Events per day, effective clock", "time brush chart")
          }
          onPointerDown={onBackgroundDown}
          onPointerMove={onMove}
          onPointerUp={onUp}
          onPointerCancel={onUp}
          onDoubleClick={() => {
            const w = presetWindow(atlas, "all");
            onQuery(withWindow(query, w.from, w.to));
          }}
          style={{ cursor: drag?.kind === "move" ? "grabbing" : "crosshair" }}
        >
          <rect x={0} y={0} width={width} height={BAR_H} fill="var(--muted)" rx={6} opacity={0.5} />
          {hist.days.map((day, i) => {
            if (hist.totals[i] === 0) return null;
            let y = BAR_H;
            const scale = (BAR_H - 6) / Math.max(1, hist.max);
            return (
              <g key={day}>
                <title>
                  {t("{day}: {count} events", "time brush bar tooltip", {
                    day: formatDay(day, true),
                    count: formatCount(hist.totals[i]),
                  })}
                </title>
                {EVENT_GROUPS.map((g, gi) => {
                  const c = hist.counts[i][gi];
                  if (c === 0) return null;
                  const h = Math.max(1, c * scale);
                  y -= h;
                  return (
                    <rect
                      key={g}
                      x={PAD + i * step + 0.5}
                      y={y}
                      width={Math.max(1, step - 1)}
                      height={h}
                      fill={GROUP_FILL[g]}
                    />
                  );
                })}
              </g>
            );
          })}
          {/* The window. */}
          <rect
            x={brushX}
            y={1}
            width={brushW}
            height={BAR_H - 2}
            fill="var(--primary)"
            fillOpacity={0.1}
            stroke="var(--primary)"
            strokeWidth={1.5}
            rx={4}
            pointerEvents="none"
          />
          {(["from", "to"] as const).map((edge) => (
            <rect
              key={edge}
              role="slider"
              tabIndex={0}
              aria-label={
                edge === "from"
                  ? t("Window start", "time brush handle")
                  : t("Window end", "time brush handle")
              }
              aria-valuetext={formatDay(edge === "from" ? q.from : q.to, true)}
              aria-valuemin={0}
              aria-valuemax={n - 1}
              aria-valuenow={daysBetween(atlas.firstDay, edge === "from" ? q.from : q.to)}
              x={(edge === "from" ? brushX : brushX + brushW) - 4}
              y={8}
              width={8}
              height={BAR_H - 16}
              rx={3}
              fill="var(--background)"
              stroke="var(--primary)"
              strokeWidth={1.5}
              className="cursor-ew-resize outline-none focus-visible:stroke-[3]"
              onPointerDown={(e) => begin(e, { kind: edge })}
              onKeyDown={(e) =>
                keyStep(e, (d) =>
                  edge === "from"
                    ? withWindow(
                        query,
                        clampDay(addDays(query.from, d), atlas.firstDay, query.to),
                        query.to,
                      )
                    : withWindow(
                        query,
                        query.from,
                        clampDay(addDays(query.to, d), query.from, atlas.lastDay),
                      ),
                )
              }
            />
          ))}
          {/* The snapshot. */}
          <g
            role="slider"
            tabIndex={0}
            aria-label={t("Snapshot", "time brush: the instant states are read at")}
            aria-valuetext={formatDay(q.asOf, true)}
            aria-valuemin={0}
            aria-valuemax={n - 1}
            aria-valuenow={daysBetween(atlas.firstDay, q.asOf)}
            className="cursor-ew-resize outline-none [&:focus-visible>path]:stroke-ring"
            onPointerDown={(e) => begin(e, { kind: "snapshot" })}
            onKeyDown={(e) =>
              keyStep(e, (d) =>
                withSnapshot(
                  query,
                  clampDay(addDays(query.asOf, d), atlas.firstDay, atlas.lastDay),
                ),
              )
            }
          >
            <line
              x1={snapX}
              x2={snapX}
              y1={0}
              y2={BAR_H}
              stroke="var(--foreground)"
              strokeWidth={1.5}
            />
            <path
              d={`M ${snapX - 5} 0 L ${snapX + 5} 0 L ${snapX} 7 Z`}
              fill="var(--foreground)"
              stroke="transparent"
              strokeWidth={2}
            />
            <rect x={snapX - 6} y={0} width={12} height={BAR_H} fill="transparent" />
          </g>
          {ticks.map((d) => (
            <text
              key={d}
              x={xOf(d)}
              y={BAR_H + 12}
              fontSize={11}
              fill="var(--muted-foreground)"
              className="tabular-nums"
            >
              {formatDay(d)}
            </text>
          ))}
        </svg>
      </div>

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex items-center gap-1">
          <SimpleTooltip content={t("Move the window back a day", "time brush")}>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("Back a day", "time brush step")}
              onClick={() => onQuery(shiftWindow(query, atlas, -1))}
            >
              <ChevronLeft aria-hidden />
            </Button>
          </SimpleTooltip>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPlaying((p) => !p)}
            aria-pressed={playing}
            disabled={!playing && query.to >= atlas.lastDay}
          >
            {playing ? <Pause aria-hidden /> : <Play aria-hidden />}
            {playing ? t("Pause", "time brush playback") : t("Play", "time brush playback")}
          </Button>
          <SimpleTooltip content={t("Move the window forward a day", "time brush")}>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("Forward a day", "time brush step")}
              onClick={() => onQuery(shiftWindow(query, atlas, 1))}
            >
              <ChevronRight aria-hidden />
            </Button>
          </SimpleTooltip>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-[11px] text-muted-foreground">Snapshot</span>
          <span className="text-sm tabular-nums">{formatDay(q.asOf, true)}</span>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="atlas-follow"
            checked={query.follow}
            onCheckedChange={(v) => onQuery(withFollow(query, v))}
          />
          <Label htmlFor="atlas-follow" className="text-[11px] font-normal text-muted-foreground">
            Follows the window end
          </Label>
        </div>
        {groupsPresent.length > 0 && (
          <ul
            className="ml-auto flex flex-wrap items-center gap-x-3 gap-y-1"
            aria-label={t("Event kinds", "time brush legend")}
          >
            {groupsPresent.map((g) => (
              <li key={g} className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <span
                  aria-hidden
                  className="size-2 rounded-[2px]"
                  style={{ background: GROUP_FILL[g] }}
                />
                {eventGroupLabel(g)}
              </li>
            ))}
            {hist.outside > 0 && (
              <li className="text-[11px] text-muted-foreground tabular-nums">
                {t("{count} outside the span", "events whose instant falls outside the timeline", {
                  count: formatCount(hist.outside),
                })}
              </li>
            )}
          </ul>
        )}
      </div>
    </div>
  );
}
