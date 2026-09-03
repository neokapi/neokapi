import { useState } from "react";
import { ChevronRight, Info, Loader2, Lock, MapPin } from "lucide-react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  Collapsible,
  CollapsibleContent,
  CoordinateChip,
  SimpleTooltip,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewPoint, TermRule } from "../../types/api";
import { LayerCard } from "./LayerCard";

/**
 * The point rail: where this unit's file sits, and what governs it there.
 *
 * The summary line is the address itself, drawn as coordinate chips, so a
 * reviewer reads the product and channel a unit belongs to before opening
 * anything. Behind it sit the file's own path, the voice profile in force with
 * its rendered guidance, the term rules bearing on this unit's wording, the
 * governance profiles' validity windows, and the caveats the resolution
 * produced.
 */

/** A rule whose severity only reports. Everything else fails a check. */
function warnsOnly(rule: TermRule): boolean {
  const s = (rule.severity ?? "").toLowerCase();
  return s === "minor" || s === "neutral";
}

/**
 * One term rule bound at this point, drawn as context.
 *
 * The rail says what the model was told about a word, so a rule is neutral
 * whatever its severity. Painting them by severity filled the card with red
 * `cart → سلة التسوق` chips that a reviewer read as a list of defects, and most
 * of them were rules resolved from the terms store, which carry no severity at
 * all and therefore landed in the "everything else fails" branch. Severity lives
 * in the tooltip; a do-not-translate rule is marked by a lock rather than by a
 * fill. Red belongs to the Checks card, where a finding says this unit broke a
 * rule.
 */
function TermRuleChip({ rule }: { rule: TermRule }) {
  const bite = warnsOnly(rule) ? t("warns only") : t("blocks approval");
  const label = rule.do_not_translate ? `${t("do not translate")} · ${bite}` : bite;
  return (
    <SimpleTooltip content={rule.note ? `${label} · ${rule.note}` : label}>
      <span
        className="inline-flex items-center gap-1 rounded border border-border bg-muted/40 px-1 py-px font-mono text-[10px] text-foreground"
        data-slot="review-point-term"
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

export interface PointRailProps {
  point?: ReviewPoint;
  /** The model is still on its way; the rail says so rather than reading empty. */
  loading?: boolean;
}

/** How many term rules the rail draws before the rest go behind a toggle. The
 *  rules bearing on this unit's own wording lead the list, so the first few are
 *  the ones a reviewer is looking for. */
const RULES_SHOWN = 8;

export function PointRail({ point, loading }: PointRailProps) {
  const [guideOpen, setGuideOpen] = useState(false);
  const [rulesOpen, setRulesOpen] = useState(false);

  if (!point) {
    return (
      <Card data-slot="review-point">
        <CardContent className="flex items-center gap-2 p-2.5 text-xs text-muted-foreground">
          <MapPin size={12} />
          {loading ? (
            <>
              <Loader2 size={12} className="animate-spin" />
              {t("Resolving the point this unit sits at…")}
            </>
          ) : (
            t("No point resolved for this unit.")
          )}
        </CardContent>
      </Card>
    );
  }

  const coordinates = Object.entries(point.coordinates ?? {}).filter(([, v]) => v);
  const rules = point.term_rules ?? [];
  const shown = rulesOpen ? rules : rules.slice(0, RULES_SHOWN);
  const hidden = rules.length - shown.length;
  const capped = point.terms_total > rules.length;
  const profiles = point.profiles ?? [];
  const notes = point.notes ?? [];

  const summary = (
    <>
      <span className="font-medium text-foreground" translate="no" data-slot="review-point-ref">
        {point.default ? t("default point") : (point.ref ?? t("default point"))}
      </span>
      {point.collection && (
        <Badge variant="outline" className="text-[11px]">
          <span translate="no">{point.collection}</span>
        </Badge>
      )}
      {coordinates.map(([axis, value]) => (
        <CoordinateChip key={axis} axis={axis} value={value} data-slot="review-point-coordinate" />
      ))}
    </>
  );

  return (
    <LayerCard
      title={t("Point")}
      icon={<MapPin size={12} className="mt-0.5 shrink-0 text-muted-foreground" aria-hidden />}
      summary={summary}
      dataSlot="review-point"
      toggleLabel={t("What governs this unit")}
    >
      <div className="space-y-1.5">
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
              <span className="font-medium" translate="no" data-slot="review-point-voice">
                {point.voice.name}
              </span>
              {point.voice.source && (
                <span className="font-mono text-[10px] text-muted-foreground" translate="no">
                  {point.voice.source}
                </span>
              )}
              {point.voice.guide && (
                <Button
                  variant="ghost"
                  size="xs"
                  className="h-5 gap-1 px-1 text-[11px] text-muted-foreground"
                  onClick={() => setGuideOpen((v) => !v)}
                  aria-expanded={guideOpen}
                  data-slot="review-point-guide-toggle"
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
            <span className="text-muted-foreground">{t("none bound here")}</span>
          )}
        </div>

        {point.voice?.guide && (
          <Collapsible open={guideOpen}>
            <CollapsibleContent>
              <pre
                className="max-h-56 overflow-auto whitespace-pre-wrap rounded-md border bg-muted/40 p-2 text-[11px] leading-snug"
                data-slot="review-point-guide"
              >
                {point.voice.guide}
              </pre>
            </CollapsibleContent>
          </Collapsible>
        )}

        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-muted-foreground">{t("Terms")}</span>
          {rules.length === 0 ? (
            <span className="text-muted-foreground">
              {point.terms_total > 0
                ? t("{count} in force, none bearing on this wording", {
                    count: point.terms_total,
                  })
                : t("none in force here")}
            </span>
          ) : (
            <>
              {shown.map((rule, i) => (
                <TermRuleChip key={`${rule.term}-${i}`} rule={rule} />
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
                    total: point.terms_total,
                  })}
                </span>
              )}
            </>
          )}
        </div>

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
