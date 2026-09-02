import { useState } from "react";
import { ChevronRight, Info, Loader2, MapPin } from "lucide-react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  Collapsible,
  CollapsibleContent,
  SimpleTooltip,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ReviewPoint, TermRule } from "../../types/api";

/**
 * The point rail: where this unit's file sits, and what governs it there.
 *
 * A reviewer scans this, so it is one dense strip of chips rather than a form.
 * It carries the collection and the coordinates, the voice profile in force
 * with its rendered guidance behind a disclosure, the term rules bearing on
 * this unit's wording, the governance profiles' validity windows, and the
 * caveats the resolution produced.
 */

/** A rule whose severity only reports. Everything else fails a check. */
function warnsOnly(rule: TermRule): boolean {
  const s = (rule.severity ?? "").toLowerCase();
  return s === "minor" || s === "neutral";
}

function TermRuleChip({ rule }: { rule: TermRule }) {
  const tone = rule.do_not_translate
    ? "border-teal-500/40 bg-teal-500/10 text-teal-700 dark:text-teal-300"
    : warnsOnly(rule)
      ? "border-amber-500/30 bg-amber-500/5 text-amber-700 dark:text-amber-400"
      : "border-destructive/40 bg-destructive/5 text-destructive";
  const label = rule.do_not_translate
    ? t("do not translate")
    : (rule.severity ?? t("blocks approval"));
  return (
    <SimpleTooltip content={rule.note ? `${label} · ${rule.note}` : label}>
      <span
        className={`inline-flex items-center gap-1 rounded border px-1 py-px font-mono text-[10px] ${tone}`}
        data-slot="review-point-term"
      >
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

  return (
    <Card data-slot="review-point">
      <CardContent className="space-y-1.5 p-2.5 text-xs">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <MapPin size={12} className="shrink-0 text-muted-foreground" />
          <span className="font-medium" translate="no" data-slot="review-point-ref">
            {point.default ? t("default point") : (point.ref ?? t("default point"))}
          </span>
          {point.collection && (
            <Badge variant="outline" className="text-[10px]">
              <span translate="no">{point.collection}</span>
            </Badge>
          )}
          {point.path && (
            <SimpleTooltip content={point.path}>
              <span className="truncate font-mono text-[10px] text-muted-foreground" translate="no">
                {point.path}
              </span>
            </SimpleTooltip>
          )}
          {coordinates.map(([axis, value]) => (
            <span
              key={axis}
              className="rounded bg-muted px-1 text-[10px] text-muted-foreground"
              data-slot="review-point-coordinate"
            >
              <span translate="no">
                {axis}: {value}
              </span>
            </span>
          ))}
        </div>

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
                  className="h-5 px-1 text-[10px] text-muted-foreground"
                  onClick={() => setRulesOpen(true)}
                  data-slot="review-point-terms-more"
                >
                  {t("{count} more", { count: hidden })}
                </Button>
              )}
              {capped && (
                <span
                  className="text-[10px] text-muted-foreground"
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
                className="rounded bg-muted px-1 text-[10px] text-muted-foreground"
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
      </CardContent>
    </Card>
  );
}
