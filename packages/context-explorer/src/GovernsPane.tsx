// "What governs here" — the by-location answer (Apache-2.0).
//
// The point resolves the way a run resolves it: the finest declared binding
// wins, and a binding skipped because its window had closed is reported rather
// than quietly replaced. That is why the fallback note is rendered at the top
// and not folded into the voice row — governance changing on a date must be
// visible.

import { Badge, Separator, cn } from "@neokapi/ui-primitives";
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

function TermRow({ term }: { term: TermHit }) {
  return (
    <li
      className="flex items-start gap-2 py-1.5"
      data-testid="governs-term"
      data-discouraged={term.discouraged}
    >
      <span
        className={cn(
          "font-medium",
          term.discouraged ? "text-destructive line-through" : "text-foreground",
        )}
      >
        {term.term}
      </span>
      {term.locale && <LocalePill locale={term.locale} />}
      {term.discouraged && term.replacement && (
        <span className="text-muted-foreground">
          {t("use {replacement}", { replacement: term.replacement })}
        </span>
      )}
      <ValidityChip state={undefined} from={term.valid_from} to={term.valid_to} />
      {term.definition && (
        <span className="min-w-0 flex-1 truncate text-muted-foreground">{term.definition}</span>
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
              {data.terms_total !== undefined && data.terms_total > data.terms.length && (
                <span className="font-normal normal-case">
                  {t("{shown} of {total}", { shown: data.terms.length, total: data.terms_total })}
                </span>
              )}
            </h4>
            {data.terms.length > 0 ? (
              <ul className="divide-y">
                {data.terms.map((term) => (
                  <TermRow
                    key={`${term.concept_id}:${term.term}:${term.locale ?? ""}`}
                    term={term}
                  />
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
