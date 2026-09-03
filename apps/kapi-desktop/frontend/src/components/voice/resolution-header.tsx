// How a voice came to govern a point, read as a header.
//
// The chain the resolver walked stays prominent, because a binding whose window
// closed is the one place in the app where governance changing on a date is
// visible: the superseded binding is struck through with the date that closed
// it, and the voice that governs instead is emphasized. The recipe plumbing
// that selected it (the key it is bound on, how it was selected, where it
// lives, the terms it reads, its channels and its window) collapses to one
// quiet line beneath.

import { ArrowRight, BookOpen, Radio } from "lucide-react";
import { Badge, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { VoicePoint, VoiceValidity } from "../../types/voice";

/** An RFC3339 instant as the date a reader needs. */
export function shortDate(value: string): string {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toISOString().slice(0, 10);
}

/** A validity window, coloured because a lapsed one is an alert, not a state. */
export function ValidityChip({ validity }: { validity: VoiceValidity }) {
  const range = [validity.from, validity.to]
    .filter(Boolean)
    .map((v) => shortDate(v as string))
    .join(" → ");
  return (
    <Badge
      variant="outline"
      className={cn(
        "font-normal",
        validity.state === "expired"
          ? "border-destructive/40 text-destructive"
          : validity.state === "upcoming"
            ? "border-amber-500/40 text-amber-700 dark:text-amber-500"
            : "border-emerald-500/40 text-emerald-700 dark:text-emerald-500",
      )}
      data-testid="voice-validity"
    >
      {validity.state}
      {range && <span className="ml-1 font-mono text-[11px] opacity-70">{range}</span>}
    </Badge>
  );
}

/**
 * The chain the resolver walked to arrive at this voice, kept prominent.
 *
 * A skipped rung is drawn, struck through, with the boundary that excluded it,
 * so a reader can see that governance moved on a date rather than that a
 * profile was never bound. The voice that governs instead is emphasized.
 */
export function ResolutionChain({ point }: { point: VoicePoint }) {
  const winner = point.fallback
    ? point.fallback.governing || t("project default")
    : (point.profile?.name ?? point.label);
  return (
    <div
      className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2"
      data-testid="voice-chain"
    >
      {point.fallback && (
        <>
          <span className="flex items-center gap-1.5 text-sm">
            <span className="text-muted-foreground line-through">{point.fallback.profile}</span>
            <Badge variant="outline" className="border-destructive/40 font-normal text-destructive">
              {point.fallback.expired ? t("window closed") : t("not yet in force")}
              {point.fallback.boundary && (
                <span className="ml-1 font-mono text-[11px] opacity-70">
                  {shortDate(point.fallback.boundary)}
                </span>
              )}
            </Badge>
          </span>
          <ArrowRight className="size-4 text-muted-foreground" />
        </>
      )}
      <span className="text-sm font-semibold text-foreground">{winner}</span>
    </div>
  );
}

/**
 * The recipe plumbing that selected this voice, collapsed to one quiet line:
 * the key it was bound on, how it was selected, where it lives, the terms it
 * reads, the channels it covers and its validity window.
 */
export function PlumbingLine({ point }: { point: VoicePoint }) {
  const path = point.source || point.binding?.value;
  const hasAny =
    !!point.field ||
    !!point.binding ||
    !!path ||
    !!point.termstore ||
    !!point.validity ||
    !!point.channels?.length;
  if (!hasAny) return null;
  return (
    <div
      className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground"
      data-testid="voice-plumbing"
    >
      {point.field && (
        <SimpleTooltip content={t("The recipe key this voice is bound on")}>
          <span className="inline-flex items-baseline gap-1">
            {t("bound on")} <code className="font-mono">{point.field}</code>
          </span>
        </SimpleTooltip>
      )}
      {point.binding && (
        <SimpleTooltip content={t("How the profile was selected")}>
          <code className="font-mono">{point.binding.kind}</code>
        </SimpleTooltip>
      )}
      {path && (
        <SimpleTooltip content={path}>
          <code className="inline-block max-w-[18rem] truncate align-bottom font-mono">{path}</code>
        </SimpleTooltip>
      )}
      {point.termstore && (
        <span className="inline-flex items-center gap-1">
          <BookOpen className="size-3" />
          {point.termstore}
        </span>
      )}
      {!!point.channels?.length && (
        <span className="inline-flex items-center gap-1">
          <Radio className="size-3" />
          {point.channels.join(", ")}
        </span>
      )}
      {point.validity && <ValidityChip validity={point.validity} />}
    </div>
  );
}

/** The resolution chain and the plumbing line, as one header. */
export function ResolutionHeader({ point }: { point: VoicePoint }) {
  return (
    <div className="space-y-2">
      <ResolutionChain point={point} />
      <PlumbingLine point={point} />
    </div>
  );
}
