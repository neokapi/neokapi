// "What governs here" — the by-location answer (Apache-2.0).
//
// The point resolves the way a run resolves it: the finest declared binding
// wins, and a binding skipped because its window had closed is reported rather
// than quietly replaced. That is why the fallback note is rendered at the top
// and not folded into the voice row — governance changing on a date must be
// visible.

import { useMemo } from "react";
import { Badge, Separator, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { EmptyHint, LocalePill, useResource } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { ScrollText, ShieldCheck } from "lucide-react";
import { useScope } from "./ScopeProvider";
import { NoteStrip, Pane, PaneError, PaneSkeleton, ReachNote, ValidityChip } from "./atoms";
import type { GovernanceAnswer, TermHit } from "./types";

export interface GovernsPaneProps {
  /** Cap on the terms listed. */
  limit?: number;
  className?: string;
}

/**
 * One concept's terms, as one row: the term it is known by, the term in the
 * chosen language beside it, the words it rules out, and the rest of its
 * languages counted.
 */
export interface ConceptTerms {
  concept_id: string;
  /** The term the row leads with: the first preferred term outside the chosen language. */
  primary: TermHit;
  /** The term in the scope's chosen language, when one is chosen and held. */
  chosen?: TermHit;
  /** Discouraged or forbidden terms, each with the term to use instead. */
  ruled_out: { term: TermHit; instead?: string }[];
  /** The concept's other allowed terms, in other languages or as synonyms. */
  others: TermHit[];
  definition?: string;
}

/** True when a term's locale is the chosen language or a region of it. */
function inLanguage(term: TermHit, locale: string): boolean {
  const l = (term.locale ?? "").toLowerCase();
  const want = locale.toLowerCase();
  return l === want || l.startsWith(`${want}-`);
}

/**
 * Group a flat term list into one row per concept, in the order the concepts
 * first appear. `locale` is the scope's chosen language, when one is set.
 */
export function groupTerms(terms: TermHit[], locale?: string): ConceptTerms[] {
  const order: string[] = [];
  const byConcept = new Map<string, TermHit[]>();
  for (const term of terms) {
    let list = byConcept.get(term.concept_id);
    if (!list) {
      list = [];
      byConcept.set(term.concept_id, list);
      order.push(term.concept_id);
    }
    list.push(term);
  }
  return order.map((id) => {
    const list = byConcept.get(id)!;
    const allowed = list.filter((term) => !term.discouraged);
    const outside = locale ? allowed.filter((term) => !inLanguage(term, locale)) : allowed;
    const primary =
      outside.find((term) => term.status === "preferred") ??
      outside[0] ??
      allowed.find((term) => term.status === "preferred") ??
      allowed[0] ??
      list[0];
    const chosen = locale
      ? allowed.find((term) => term !== primary && inLanguage(term, locale))
      : undefined;
    const ruled_out = list
      .filter((term) => term.discouraged && term !== primary)
      .map((term) => ({
        term,
        // The allowed term in the same language, else the one the source named.
        instead:
          allowed.find((a) => a.locale === term.locale && a.status === "preferred")?.term ??
          allowed.find((a) => a.locale === term.locale)?.term ??
          term.replacement,
      }));
    const others = allowed.filter((term) => term !== primary && term !== chosen);
    return {
      concept_id: id,
      primary,
      chosen,
      ruled_out,
      others,
      definition: list.find((term) => term.definition)?.definition,
    };
  });
}

/** A term's word, struck through when it is ruled out, and its language. */
function TermWord({ term }: { term: TermHit }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        className={cn(
          "font-medium",
          term.discouraged ? "text-destructive line-through" : "text-foreground",
        )}
      >
        {term.term}
      </span>
      {term.locale && <LocalePill locale={term.locale} />}
    </span>
  );
}

function ConceptRow({ row }: { row: ConceptTerms }) {
  const { primary, chosen, ruled_out, others } = row;
  const languages = new Set(others.map((term) => term.locale ?? ""));
  return (
    <li
      className="py-1.5"
      data-testid="governs-term"
      data-concept={row.concept_id}
      data-discouraged={primary.discouraged}
    >
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <TermWord term={primary} />
        {primary.discouraged && primary.replacement && (
          <span className="text-muted-foreground">
            {t("use {replacement}", { replacement: primary.replacement })}
          </span>
        )}
        <ValidityChip state={undefined} from={primary.valid_from} to={primary.valid_to} />
        {chosen && (
          <span data-testid="governs-term-chosen" className="inline-flex items-center gap-1.5">
            <span aria-hidden className="text-muted-foreground">
              ·
            </span>
            <TermWord term={chosen} />
          </span>
        )}
        {ruled_out.map(({ term, instead }) => (
          <span
            key={`${term.term}:${term.locale ?? ""}`}
            data-testid="governs-term-discouraged"
            className="inline-flex items-center gap-1.5"
          >
            <TermWord term={term} />
            {instead && (
              <span className="text-muted-foreground">
                {t("use {replacement}", { replacement: instead })}
              </span>
            )}
            <ValidityChip state={undefined} from={term.valid_from} to={term.valid_to} />
          </span>
        ))}
        {others.length > 0 && (
          <SimpleTooltip
            content={others
              .map((term) => (term.locale ? `${term.term} (${term.locale})` : term.term))
              .join(", ")}
          >
            <span
              data-testid="governs-term-more"
              className="text-xs text-muted-foreground tabular-nums"
            >
              {languages.size === others.length
                ? t("{count, plural, one {+# language} other {+# languages}}", {
                    count: others.length,
                  })
                : t("{count, plural, one {+# term} other {+# terms}}", { count: others.length })}
            </span>
          </SimpleTooltip>
        )}
      </div>
      {row.definition && (
        <p className="mt-0.5 truncate text-xs text-muted-foreground">{row.definition}</p>
      )}
    </li>
  );
}

export function GovernsPane({ limit, className }: GovernsPaneProps) {
  const { scope, source, capabilities } = useScope();

  const { data, loading, error } = useResource<GovernanceAnswer>(
    () => source.governs({ scope, limit }),
    [
      scope.workspace,
      scope.project,
      scope.stream,
      scope.collection,
      scope.path,
      scope.locale,
      limit,
    ],
  );

  const terms = data?.terms;
  const concepts = useMemo(() => groupTerms(terms ?? [], scope.locale), [terms, scope.locale]);

  const reach =
    capabilities.governance === "resolved"
      ? t("resolved at this point")
      : t("from declared content");

  return (
    <Pane
      title={t("What governs here")}
      icon={<ShieldCheck />}
      reach={reach}
      className={className}
      data-testid="governs-pane"
    >
      {error && <PaneError error={error} />}
      {!error && loading && <PaneSkeleton />}
      {!error && !loading && data && (
        <div className="space-y-4 text-sm">
          <NoteStrip notes={data.notes} />

          <div className="flex flex-wrap items-center gap-2">
            {data.point.profile ? (
              <Badge variant="outline" data-testid="governs-profile">
                {t("profile")} <span className="ml-1 font-medium">{data.point.profile}</span>
              </Badge>
            ) : (
              <Badge variant="outline" data-testid="governs-profile">
                {t("project default")}
              </Badge>
            )}
            {data.point.channel && (
              <Badge variant="outline" data-testid="governs-channel">
                {t("channel")} <span className="ml-1 font-medium">{data.point.channel}</span>
              </Badge>
            )}
            {data.point.collection && (
              <Badge variant="secondary" className="font-normal">
                {data.point.collection}
              </Badge>
            )}
            {data.clearance && (
              <Badge variant="outline" data-testid="governs-clearance">
                {t("clearance")} <span className="ml-1 font-medium">{data.clearance}</span>
              </Badge>
            )}
          </div>

          <section data-testid="governs-voice">
            <h4 className="mb-1 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              {t("Voice")}
            </h4>
            {data.voice ? (
              <div className="space-y-2">
                <div className="flex flex-wrap items-baseline gap-2">
                  <span className="font-medium">{data.voice.name}</span>
                  {data.voice.field && (
                    <code className="font-mono text-[11px] text-muted-foreground">
                      {data.voice.field}
                    </code>
                  )}
                  {data.voice.source && (
                    <span className="text-xs text-muted-foreground">{data.voice.source}</span>
                  )}
                </div>
                {data.voice.guide && (
                  <details className="group" data-testid="governs-voice-guide">
                    <summary className="cursor-pointer text-xs text-muted-foreground hover:text-foreground">
                      {t("The guide a tool reads")}
                    </summary>
                    <pre className="mt-1.5 max-h-64 overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-[11px] whitespace-pre-wrap text-muted-foreground">
                      {data.voice.guide}
                    </pre>
                  </details>
                )}
              </div>
            ) : (
              <ReachNote>{t("No voice profile binds at this point.")}</ReachNote>
            )}
          </section>

          {data.profiles.length > 0 && (
            <>
              <Separator />
              <section data-testid="governs-windows">
                <h4 className="mb-1.5 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                  {t("Validity")}
                </h4>
                <ul className="flex flex-wrap gap-2">
                  {data.profiles.map((p) => (
                    <li key={p.name} className="flex items-center gap-1.5">
                      <span className="text-xs font-medium">{p.name}</span>
                      <ValidityChip state={p.state} from={p.valid_from} to={p.valid_to} />
                    </li>
                  ))}
                </ul>
              </section>
            </>
          )}

          {data.rules && data.rules.length > 0 && (
            <>
              <Separator />
              <section data-testid="governs-rules">
                <h4 className="mb-1.5 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                  {t("Rules")}
                </h4>
                <ul className="space-y-1">
                  {data.rules.map((rule) => (
                    <li key={rule.id} className="flex items-center gap-2">
                      {rule.gate && (
                        <Badge variant="outline" className="font-mono text-[11px] font-normal">
                          {rule.gate}
                        </Badge>
                      )}
                      <span>{rule.label}</span>
                      <ValidityChip state={rule.state} from={rule.valid_from} to={rule.valid_to} />
                    </li>
                  ))}
                </ul>
              </section>
            </>
          )}

          <Separator />
          <section data-testid="governs-terms">
            <h4 className="mb-1 flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              <ScrollText className="size-3.5" />
              {t("Terms")}
              {data.terms_total !== undefined && data.terms_total > concepts.length ? (
                <span className="font-normal normal-case">
                  {t("{shown} of {total}", { shown: concepts.length, total: data.terms_total })}
                </span>
              ) : (
                data.terms_capped && (
                  <span className="font-normal normal-case">
                    {t("first {shown}", { shown: concepts.length })}
                  </span>
                )
              )}
            </h4>
            {concepts.length > 0 ? (
              <ul className="divide-y">
                {concepts.map((row) => (
                  <ConceptRow key={row.concept_id} row={row} />
                ))}
              </ul>
            ) : (
              <EmptyHint
                title={t("No terms scoped here")}
                description={t("Vocabulary bound at a coarser point still applies.")}
              />
            )}
          </section>
        </div>
      )}
    </Pane>
  );
}
