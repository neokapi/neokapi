import { useState } from "react";
import { ChevronRight, Info, Loader2, Lock, MapPin, Tag } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "../../lib/utils";
import { findingToneBadgeClass } from "../../lib/finding-severity";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Collapsible, CollapsibleContent } from "../ui/collapsible";
import { CoordinateChip } from "../ui/coordinate-chip";
import { SimpleTooltip } from "../ui/tooltip";
import { LayerCard } from "./LayerCard";
import type { ReviewPointView, ReviewTermHitView, ReviewTermRuleView } from "./types";

/**
 * The point card: where this unit's file sits, and what governs it there.
 *
 * The summary line is the address itself, drawn as coordinate chips, so a
 * reviewer reads the product and channel a unit belongs to before opening
 * anything. Behind it sit the file's own path, the voice profile in force with
 * its rendered guidance and the unit's score against the profile's bar, the
 * term rules bearing on this unit's wording, the terms the source matches, the
 * governance profiles' validity windows, and the caveats the resolution
 * produced. A host that carries none of a row leaves that row out.
 */

/** A rule whose severity only reports. Everything else fails a check. */
function warnsOnly(rule: ReviewTermRuleView): boolean {
  const s = (rule.severity ?? "").toLowerCase();
  return s === "minor" || s === "neutral";
}

/**
 * One term rule bound at this point, drawn as context.
 *
 * The card says what the model was told about a word, so a rule is neutral
 * whatever its severity: the bite ("blocks approval" / "warns only") reads in
 * the tooltip, and a do-not-translate rule is marked by a lock rather than by
 * a fill. Red belongs to the Checks card, where a finding says this unit broke
 * a rule. See packages/ui/docs/judgement-colours.md.
 */
export function TermRuleChip({ rule, index }: { rule: ReviewTermRuleView; index?: number }) {
  const bite = warnsOnly(rule) ? t("warns only") : t("blocks approval");
  const label = rule.do_not_translate ? `${t("do not translate")} · ${bite}` : bite;
  return (
    <SimpleTooltip content={rule.note ? `${label} · ${rule.note}` : label}>
      <span
        className="inline-flex items-center gap-1 rounded border border-border bg-muted/40 px-1 py-px font-mono text-[10px] text-foreground"
        data-slot="review-point-term"
        data-testid={index === undefined ? "point-term-rule" : `point-term-rule-${index}`}
        data-severity={warnsOnly(rule) ? "warns" : "blocks"}
        data-dnt={rule.do_not_translate ? "true" : undefined}
      >
        {rule.do_not_translate && (
          <Lock size={9} className="shrink-0 text-muted-foreground" aria-hidden />
        )}
        <span translate="no">{rule.term}</span>
        {rule.replacement ? (
          <>
            <span aria-hidden>&rarr;</span>
            <span translate="no">{rule.replacement}</span>
          </>
        ) : null}
      </span>
    </SimpleTooltip>
  );
}

/** One term the source matches: the term, and the renderings the store mandates. */
export function TermHitChip({ hit }: { hit: ReviewTermHitView }) {
  return (
    <span
      className="inline-flex items-center gap-1 rounded border border-border bg-muted/40 px-1 py-px font-mono text-[10px] text-foreground"
      title={hit.domain ? t("Domain: {domain}", { domain: hit.domain }) : undefined}
      data-slot="review-point-term-hit"
    >
      <Tag size={9} className="shrink-0 text-muted-foreground" aria-hidden />
      <span translate="no">{hit.term}</span>
      {hit.renderings && hit.renderings.length > 0 && (
        <>
          <span aria-hidden>&rarr;</span>
          <span translate="no">{hit.renderings.join(", ")}</span>
        </>
      )}
    </span>
  );
}

/**
 * The unit's voice score against the bar it is held to.
 *
 * A score below the bar is a verdict on this unit, so it takes the tone a
 * failing finding takes; a score that clears the bar is context and stays
 * muted. An unscored unit reads as the bar alone.
 */
export function VoiceScoreChip({ score, bar }: { score?: number; bar: number }) {
  const below = score !== undefined && score < bar;
  return (
    <span
      className={cn(
        "rounded-full border px-2 py-0.5 text-[11px] tabular-nums",
        below
          ? findingToneBadgeClass("destructive")
          : "border-border bg-card text-muted-foreground",
      )}
      data-slot="review-point-voice-score"
      data-testid="point-voice-score"
      data-below-bar={below ? "true" : undefined}
      title={
        score === undefined
          ? t("The lowest voice score this profile accepts. This unit has not been scored.")
          : t("The unit's latest voice score against the bar its profile sets.")
      }
    >
      {score === undefined ? t("Bar {bar}", { bar }) : t("Voice {score} of {bar}", { score, bar })}
    </span>
  );
}

export interface PointCardProps {
  point?: ReviewPointView;
  /** The model is still on its way; the card says so rather than reading empty. */
  loading?: boolean;
  defaultOpen?: boolean;
  testId?: string;
  className?: string;
}

/** How many term rules the card draws before the rest go behind a toggle. The
 *  rules bearing on this unit's own wording lead the list, so the first few are
 *  the ones a reviewer is looking for. */
const RULES_SHOWN = 8;

export function PointCard({ point, loading, defaultOpen, testId, className }: PointCardProps) {
  const [guideOpen, setGuideOpen] = useState(false);
  const [rulesOpen, setRulesOpen] = useState(false);

  const icon = <MapPin size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />;

  // One LayerCard whether the point has arrived or not, so the fold state a
  // reviewer set survives the model landing: a second card for the empty
  // state would hand its own state to the loaded one.
  if (!point) {
    const waiting = loading
      ? t("Resolving the point this unit sits at…")
      : t("No point resolved for this unit.");
    return (
      <LayerCard
        title={t("Point")}
        icon={icon}
        summary={
          loading ? (
            <span className="flex items-center gap-1.5">
              <Loader2 size={12} className="animate-spin" aria-hidden />
              {waiting}
            </span>
          ) : (
            waiting
          )
        }
        dataSlot="review-point"
        testId={testId}
        toggleLabel={t("What governs this unit")}
        defaultOpen={defaultOpen}
        className={className}
      >
        <p className="text-muted-foreground" data-testid="review-point">
          {waiting}
        </p>
      </LayerCard>
    );
  }

  const coordinates = Object.entries(point.coordinates ?? {}).filter(([, v]) => v);
  const rules = point.termRules ?? [];
  const shown = rulesOpen ? rules : rules.slice(0, RULES_SHOWN);
  const hidden = rules.length - shown.length;
  const total = point.termsTotal ?? rules.length;
  const capped = total > rules.length;
  const profiles = point.profiles ?? [];
  const notes = point.notes ?? [];
  const ref = point.ref ?? (point.default ? t("default point") : undefined);
  const bar = point.voice?.bar;

  const summary = (
    <>
      {ref && (
        <span className="font-medium text-foreground" translate="no" data-slot="review-point-ref">
          {ref}
        </span>
      )}
      {point.collection && (
        <Badge variant="outline" className="text-[11px]" data-testid="point-collection">
          <span translate="no">{point.collection}</span>
        </Badge>
      )}
      {coordinates.map(([axis, value]) => (
        <CoordinateChip
          key={axis}
          axis={axis}
          value={value}
          data-slot="review-point-coordinate"
          data-testid={`point-coordinate-${axis}`}
        />
      ))}
      {!ref && !point.collection && coordinates.length === 0 && (
        <span translate="no">{point.voice?.name ?? t("No coordinates declared")}</span>
      )}
    </>
  );

  return (
    <LayerCard
      title={t("Point")}
      icon={icon}
      summary={summary}
      dataSlot="review-point"
      testId={testId}
      toggleLabel={t("What governs this unit")}
      defaultOpen={defaultOpen}
      className={className}
    >
      <div className="space-y-1.5" data-testid="review-point">
        {point.path && (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-muted-foreground">{t("File")}</span>
            <SimpleTooltip content={point.path}>
              <span className="truncate font-mono text-[10px] text-muted-foreground" translate="no">
                {point.path}
              </span>
            </SimpleTooltip>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-muted-foreground">{t("Voice")}</span>
          {point.voice ? (
            <>
              <span
                className="font-medium"
                translate="no"
                data-slot="review-point-voice"
                data-testid="point-profile-name"
              >
                {point.voice.name}
              </span>
              {point.voice.source && (
                <span className="font-mono text-[10px] text-muted-foreground" translate="no">
                  {point.voice.source}
                </span>
              )}
              {bar !== undefined && <VoiceScoreChip score={point.voice.score} bar={bar} />}
              {point.voice.guide && (
                <Button
                  variant="ghost"
                  size="xs"
                  className="h-5 gap-1 px-1 text-[11px] text-muted-foreground"
                  onClick={() => setGuideOpen((v) => !v)}
                  aria-expanded={guideOpen}
                  data-slot="review-point-guide-toggle"
                  data-testid="point-guidance-toggle"
                >
                  <ChevronRight
                    size={12}
                    className={
                      guideOpen ? "rotate-90 transition-transform" : "transition-transform"
                    }
                  />
                  {t("Guidance the model was given")}
                </Button>
              )}
            </>
          ) : (
            <span className="text-muted-foreground">
              {t("No voice profile is bound at this point.")}
            </span>
          )}
        </div>

        {point.voice?.guide && (
          <Collapsible open={guideOpen}>
            <CollapsibleContent>
              <pre
                className="max-h-56 overflow-auto whitespace-pre-wrap rounded-md border bg-muted/40 p-2 text-[11px] leading-snug"
                data-slot="review-point-guide"
                data-testid="point-guidance"
              >
                {point.voice.guide}
              </pre>
            </CollapsibleContent>
          </Collapsible>
        )}

        <div className="flex flex-wrap items-center gap-x-2 gap-y-1" data-testid="point-term-rules">
          <span className="text-muted-foreground">{t("Terms")}</span>
          {rules.length === 0 ? (
            <span className="text-muted-foreground">
              {total > 0
                ? t("{count} in force, none bearing on this wording", { count: total })
                : t("none in force here")}
            </span>
          ) : (
            <>
              {shown.map((rule, i) => (
                <TermRuleChip key={`${rule.term}-${i}`} rule={rule} index={i} />
              ))}
              {hidden > 0 && (
                <Button
                  variant="ghost"
                  size="xs"
                  className="h-5 px-1 text-[11px] text-muted-foreground"
                  onClick={() => setRulesOpen(true)}
                  data-slot="review-point-terms-more"
                >
                  {t("{count} more", { count: hidden })}
                </Button>
              )}
              {capped && (
                <span
                  className="text-[11px] text-muted-foreground"
                  data-slot="review-point-terms-total"
                >
                  {t("{shown} of {total} rules bound here", {
                    shown: rules.length,
                    total,
                  })}
                </span>
              )}
            </>
          )}
        </div>

        {point.termHits !== undefined && (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1" data-testid="point-terms">
            <span className="text-muted-foreground">{t("Matched")}</span>
            {point.termHitsLoading ? (
              <span className="text-muted-foreground">{t("Looking up terms…")}</span>
            ) : point.termHits.length === 0 ? (
              <span className="text-muted-foreground">{t("No terms matched this block.")}</span>
            ) : (
              point.termHits.map((hit, i) => <TermHitChip key={`${hit.term}-${i}`} hit={hit} />)
            )}
          </div>
        )}

        {profiles.length > 0 && (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-muted-foreground">{t("Valid")}</span>
            {profiles.map((p) => (
              <span
                key={`${p.name}-${p.valid_from ?? ""}`}
                className="rounded bg-muted px-1 text-[11px] text-muted-foreground"
                data-slot="review-point-profile"
              >
                <span translate="no">{p.name}</span> · {p.state}
                {p.valid_from || p.valid_to
                  ? ` · ${p.valid_from ?? "…"} → ${p.valid_to ?? "…"}`
                  : ""}
              </span>
            ))}
          </div>
        )}

        {notes.length > 0 && (
          <ul className="space-y-0.5" data-slot="review-point-notes">
            {notes.map((note, i) => (
              <li key={i} className="flex items-start gap-1 text-[11px] text-muted-foreground">
                <Info size={11} className="mt-0.5 shrink-0" />
                <span>{note}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </LayerCard>
  );
}
