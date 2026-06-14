// Shared presentational primitives for the concept evolution timeline
// (Apache-2.0). The two renderers — the horizontal `EvolutionRoadmap` and the
// vertical `EvolutionGraph` it folds into on narrow widths — compose these so a
// span bar, a milestone marker, a cluster cloud, and the tone vocabulary look
// identical in both. Pure presentational + one measurement hook; the temporal
// maths live in `./evolution-model`.

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ComponentType, ReactNode, RefObject } from "react";
import { Badge, cn } from "@neokapi/ui-primitives";
import {
  Ban,
  GitBranch,
  GitCommitHorizontal,
  GitFork,
  Layers,
  MessageSquare,
  PencilLine,
  Quote,
  Share2,
  Sparkles,
  Star,
} from "lucide-react";
import type { LucideProps } from "lucide-react";
import { TERM_STATUS_LABEL } from "./concept-meta";
import type { EvolutionExtent, EvolutionSpan, EvolutionTone, SpanCap } from "./evolution-types";
import type { TermStatus } from "./types";

type IconType = ComponentType<LucideProps>;

// ── Tone vocabulary (icon + accent) ──────────────────────────────────────────

interface ToneMeta {
  icon: IconType;
  /** Marker dot classes (background + ring). */
  dot: string;
  /** Text/icon accent classes. */
  accent: string;
  label: string;
}

/** The icon, accent, and label each tone renders with — one source of truth. */
export const TONE_META: Record<EvolutionTone, ToneMeta> = {
  genesis: {
    icon: Sparkles,
    dot: "bg-success text-success-foreground",
    accent: "text-success",
    label: "Created",
  },
  rename: {
    icon: GitFork,
    dot: "bg-primary text-primary-foreground",
    accent: "text-primary",
    label: "Renamed",
  },
  promote: {
    icon: Star,
    dot: "bg-success text-success-foreground",
    accent: "text-success",
    label: "Preferred",
  },
  ban: {
    icon: Ban,
    dot: "bg-destructive text-destructive-foreground",
    accent: "text-destructive",
    label: "Banned",
  },
  sibling: {
    icon: GitBranch,
    dot: "bg-primary text-primary-foreground",
    accent: "text-primary",
    label: "Sibling",
  },
  relation: {
    icon: Share2,
    dot: "bg-primary/80 text-primary-foreground",
    accent: "text-primary",
    label: "Relation",
  },
  governed: {
    icon: GitCommitHorizontal,
    dot: "bg-primary text-primary-foreground",
    accent: "text-primary",
    label: "Change-set",
  },
  edit: {
    icon: PencilLine,
    dot: "bg-muted text-muted-foreground",
    accent: "text-muted-foreground",
    label: "Edit",
  },
  evidence: {
    icon: Quote,
    dot: "bg-muted text-muted-foreground",
    accent: "text-muted-foreground",
    label: "Observation",
  },
  discussion: {
    icon: MessageSquare,
    dot: "bg-muted text-muted-foreground",
    accent: "text-muted-foreground",
    label: "Comment",
  },
};

export function toneMeta(tone: EvolutionTone): ToneMeta {
  return TONE_META[tone] ?? TONE_META.edit;
}

// ── Status → span fill ───────────────────────────────────────────────────────

/** The bar fill for a term status. Banned/deprecated read as muted + struck. */
export const SPAN_FILL: Record<TermStatus, string> = {
  preferred: "bg-success/70 border-success",
  approved: "bg-primary/55 border-primary",
  admitted: "bg-warning/60 border-warning",
  proposed: "bg-muted-foreground/30 border-muted-foreground/40",
  deprecated: "bg-muted-foreground/20 border-muted-foreground/30",
  forbidden: "bg-destructive/45 border-destructive",
};

/** A small dot colour for a status (lane headers, legends). */
export const STATUS_DOT: Record<TermStatus, string> = {
  preferred: "bg-success",
  approved: "bg-primary",
  admitted: "bg-warning",
  proposed: "bg-muted-foreground/50",
  deprecated: "bg-muted-foreground/40",
  forbidden: "bg-destructive",
};

// ── Time scale ───────────────────────────────────────────────────────────────

export interface TimeScale {
  startMs: number;
  endMs: number;
  /** Fraction 0..1 across the extent for an ISO instant (clamped). */
  pct(iso: string): number;
  /** Fraction 0..1, or `null` for the open end (no validTo). */
  pctOrNull(iso: string | null): number | null;
}

/** Build a clamped 0..1 time scale from the model's extent. */
export function makeScale(extent: EvolutionExtent): TimeScale {
  const startMs = Date.parse(extent.start);
  const endMs = Math.max(Date.parse(extent.end), startMs + 1);
  const span = endMs - startMs;
  const pct = (iso: string): number => {
    const v = Date.parse(iso);
    if (Number.isNaN(v)) return 0;
    return Math.min(1, Math.max(0, (v - startMs) / span));
  };
  return { startMs, endMs, pct, pctOrNull: (iso) => (iso === null ? null : pct(iso)) };
}

// ── Container-width measurement ──────────────────────────────────────────────

/**
 * Measure an element's own width (not the viewport) so the timeline picks its
 * layout from the room IT has — the panel sits in a narrow dashboard column on
 * large screens and full-width elsewhere. Returns [ref, width]; width is 0 until
 * the first measurement (and where ResizeObserver is unavailable, e.g. jsdom).
 */
export function useContainerWidth<T extends HTMLElement = HTMLDivElement>(): [
  RefObject<T | null>,
  number,
] {
  const ref = useRef<T | null>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.getBoundingClientRect().width);
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) setWidth(entry.contentRect.width);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, width];
}

/** Re-export so renderers don't each import React effects for tiny needs. */
export { useEffect };

// ── Axis ─────────────────────────────────────────────────────────────────────

export function AxisTicks({
  scale,
  ticks,
  className,
}: {
  scale: TimeScale;
  ticks: EvolutionExtent["ticks"];
  className?: string;
}) {
  return (
    <div className={cn("pointer-events-none absolute inset-0", className)} aria-hidden>
      {ticks.map((tick) => {
        const left = `${(scale.pct(tick.at) * 100).toFixed(3)}%`;
        return (
          <div key={tick.at} className="absolute bottom-0 top-0 flex flex-col" style={{ left }}>
            <span className={cn("w-px flex-1", tick.major ? "bg-border" : "bg-border/50")} />
          </div>
        );
      })}
    </div>
  );
}

export function AxisLabels({
  scale,
  ticks,
  className,
}: {
  scale: TimeScale;
  ticks: EvolutionExtent["ticks"];
  className?: string;
}) {
  return (
    <div className={cn("relative h-4", className)}>
      {ticks.map((tick) => (
        <span
          key={tick.at}
          className={cn(
            "absolute -translate-x-1/2 whitespace-nowrap text-[10px] leading-none",
            tick.major ? "font-medium text-muted-foreground" : "text-muted-foreground/60",
          )}
          style={{ left: `${(scale.pct(tick.at) * 100).toFixed(3)}%` }}
        >
          {tick.label}
        </span>
      ))}
    </div>
  );
}

// ── Span bar ─────────────────────────────────────────────────────────────────

const CAP_TITLE: Record<SpanCap, string> = {
  open: "current",
  bounded: "valid until",
  expired: "expired",
  banned: "banned — do not use after",
};

/**
 * A term's validity as a positioned bar across the time axis. The parent must be
 * `relative`; the bar is absolutely positioned by the scale. An open span runs
 * to the right edge with a soft fade; a banned span is hatched and struck.
 */
export function SpanBar({
  span,
  scale,
  onSelect,
  className,
}: {
  span: EvolutionSpan;
  scale: TimeScale;
  onSelect?: (span: EvolutionSpan) => void;
  className?: string;
}) {
  const left = scale.pct(span.start);
  const endPct = span.end === null ? 1 : scale.pct(span.end);
  const widthPct = Math.max(endPct - left, 0.012);
  const banned = span.cap === "banned";
  const expired = span.cap === "expired";
  const interactive = Boolean(onSelect);
  return (
    <button
      type={interactive ? "button" : undefined}
      disabled={!interactive}
      onClick={interactive ? () => onSelect!(span) : undefined}
      title={`${span.termText} · ${TERM_STATUS_LABEL[span.status]} · ${CAP_TITLE[span.cap]}`}
      className={cn(
        "group absolute flex h-6 items-center gap-1 overflow-hidden rounded border px-1.5 text-[11px] font-medium leading-none transition",
        SPAN_FILL[span.status],
        // Diagonal hatch marks a retired/expired span. Explicit light + dark
        // stops so it reads on either theme (a single black hatch vanished on
        // dark backgrounds).
        (banned || expired) &&
          "[background-image:repeating-linear-gradient(135deg,transparent,transparent_3px,rgb(0_0_0/0.10)_3px,rgb(0_0_0/0.10)_5px)] dark:[background-image:repeating-linear-gradient(135deg,transparent,transparent_3px,rgb(255_255_255/0.16)_3px,rgb(255_255_255/0.16)_5px)]",
        span.cap === "open" && "rounded-r-none",
        interactive &&
          "cursor-pointer hover:brightness-105 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
      style={{
        left: `${(left * 100).toFixed(3)}%`,
        width: `calc(${(widthPct * 100).toFixed(3)}% )`,
      }}
    >
      {banned && <Ban aria-hidden className="size-3 shrink-0" />}
      <span className={cn("truncate text-foreground/90", banned && "line-through")}>
        {span.termText}
      </span>
      {span.cap === "open" && (
        <span
          aria-hidden
          className="ml-auto h-full w-3 bg-gradient-to-r from-transparent to-card"
        />
      )}
    </button>
  );
}

// ── Milestone marker ─────────────────────────────────────────────────────────

export function MilestoneDot({
  tone,
  title,
  onClick,
  size = "md",
  className,
}: {
  tone: EvolutionTone;
  title?: string;
  onClick?: () => void;
  size?: "sm" | "md";
  className?: string;
}) {
  const meta = toneMeta(tone);
  const Icon = meta.icon;
  const interactive = Boolean(onClick);
  const dim = size === "sm" ? "size-5 [&_svg]:size-2.5" : "size-7 [&_svg]:size-3.5";
  return (
    <span
      role={interactive ? "button" : undefined}
      tabIndex={interactive ? 0 : undefined}
      onClick={onClick}
      onKeyDown={
        interactive
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onClick!();
              }
            }
          : undefined
      }
      title={title}
      className={cn(
        "inline-flex items-center justify-center rounded-full ring-4 ring-card",
        meta.dot,
        dim,
        interactive && "cursor-pointer focus-visible:outline-none focus-visible:ring-ring",
        className,
      )}
    >
      <Icon aria-hidden />
    </span>
  );
}

// ── Cluster cloud ────────────────────────────────────────────────────────────

/** A "N changes" cloud standing in for a folded run of routine events. */
export function ClusterPill({
  count,
  expanded,
  onToggle,
  className,
}: {
  count: number;
  expanded?: boolean;
  onToggle?: () => void;
  className?: string;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={expanded ?? false}
      title={`${count} routine changes`}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border border-dashed bg-muted/40 px-2 py-0.5 text-[11px] font-medium text-muted-foreground transition hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
    >
      <Layers aria-hidden className="size-3" />
      {count} changes
    </button>
  );
}

// ── Context marker ───────────────────────────────────────────────────────────

export function ContextMarkerTick({
  at,
  label,
  kind,
  scale,
  showLabel = true,
}: {
  at: string;
  label: string;
  kind: "market" | "changeset";
  scale: TimeScale;
  /** When false, draw the dot only — the label is suppressed to avoid collision
   * on a dense track and stays available via the tooltip. */
  showLabel?: boolean;
}) {
  return (
    <div
      className="absolute top-0 flex -translate-x-1/2 flex-col items-center"
      style={{ left: `${(scale.pct(at) * 100).toFixed(3)}%` }}
      title={label}
    >
      <span
        className={cn("size-1.5 rounded-full", kind === "market" ? "bg-primary" : "bg-warning")}
      />
      {showLabel && (
        <span className="mt-0.5 max-w-[6rem] truncate whitespace-nowrap text-[10px] leading-none text-muted-foreground">
          {label}
        </span>
      )}
    </div>
  );
}

// ── Lane label ───────────────────────────────────────────────────────────────

/** The leading cell of a lane: the locale and a status hint for its terms. */
export function LaneLabel({
  locale,
  market,
  statuses,
  className,
}: {
  locale: string;
  market?: string;
  statuses?: TermStatus[];
  className?: string;
}) {
  const dots = [...new Set(statuses ?? [])];
  return (
    <div className={cn("flex items-center gap-1.5", className)}>
      <span className="font-mono text-xs font-medium uppercase text-foreground">{locale}</span>
      {market && <span className="truncate text-[10px] text-muted-foreground">{market}</span>}
      <span className="ml-auto flex items-center gap-0.5">
        {dots.map((s) => (
          <span
            key={s}
            className={cn("size-1.5 rounded-full", STATUS_DOT[s])}
            title={TERM_STATUS_LABEL[s]}
          />
        ))}
      </span>
    </div>
  );
}

// ── Legend ───────────────────────────────────────────────────────────────────

const LEGEND_STATUSES: TermStatus[] = [
  "preferred",
  "approved",
  "admitted",
  "deprecated",
  "forbidden",
];

export function EvolutionLegend({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted-foreground",
        className,
      )}
    >
      {LEGEND_STATUSES.map((s) => (
        <span key={s} className="inline-flex items-center gap-1">
          <span className={cn("h-2 w-3 rounded-sm border", SPAN_FILL[s])} />
          {TERM_STATUS_LABEL[s]}
        </span>
      ))}
      <span className="inline-flex items-center gap-1">
        <GitFork aria-hidden className="size-3 text-primary" />
        rename
      </span>
      <span className="inline-flex items-center gap-1">
        <GitBranch aria-hidden className="size-3 text-primary" />
        sibling
      </span>
    </div>
  );
}

// ── Empty / frame helpers (re-exported for renderer convenience) ─────────────

export function EvolutionRow({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("relative", className)}>{children}</div>;
}

export { Badge };
